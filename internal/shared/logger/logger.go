// logger.go 提供统一日志抽象接口和异步处理框架
// 主要功能：
// - 支持多后端输出（Loki/Zerolog）
// - 异步批量处理（5秒/10条触发）
// - 线程安全的通道通信
// - 优雅关闭机制
// - 全局日志级别控制
// - 结构化日志记录
// - 监控指标收集
// - 日志采样功能
// - 上下文支持
package logger

import (
	"context"
	"easyms/internal/shared/entities"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// BackendLogger 定义日志后端接口
type BackendLogger interface {
	Log(logs []*LogEntry) error // 批量记录日志条目
}

var (
	loggerImpl     BackendLogger        // 全局日志实现
	defaultService string               // 默认服务名称
	minLogLevel    string               // 最小日志级别
	sampleRate     float64        = 1.0 // 采样率，默认1.0表示100%记录
	fallbackLogger zerolog.Logger       // 备用日志记录器
	rootLogger     *Logger              // 全局根日志记录器
)

// 全局日志通道和管理器
var (
	logPool = sync.Pool{
		New: func() interface{} {
			return &LogEntry{
				Fields: make(map[string]interface{}),
			}
		},
	}
	logChan        = make(chan *LogEntry, 1000) // 日志处理通道，缓冲区大小为1000
	closeChan      = make(chan struct{})        // 关闭通知通道
	wg             sync.WaitGroup               // worker管理器，用于等待所有日志处理完成
	logChanFull    = false                      // 标记日志通道是否已满
	logChanFullMtx sync.RWMutex                 // 保护logChanFull的读写锁
)

// Prometheus metrics
var (
	logEntriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_entries_total",
			Help: "Total number of log entries processed",
		},
		[]string{"level", "service"},
	)

	logDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_dropped_total",
			Help: "Total number of log entries dropped due to full channel",
		},
		[]string{"service"},
	)

	logLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "log_latency_seconds",
			Help: "Log processing latency in seconds",
		},
		[]string{"backend"},
	)

	logChannelUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "log_channel_usage",
			Help: "Current usage of log channel buffer",
		},
	)

	logChannelCapacity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "log_channel_capacity",
			Help: "Total capacity of log channel buffer",
		},
	)

	logSampledTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_sampled_total",
			Help: "Total number of log entries sampled out",
		},
		[]string{"level", "service"},
	)
)

// Logger 是上下文感知的日志记录器
type Logger struct {
	fields map[string]interface{}
}

func shouldLog(level string) bool {
	return strings.Compare(level, minLogLevel) >= 0
}

// shouldSample 根据采样率判断是否应该记录该日志
func shouldSample() bool {
	return rand.Float64() < sampleRate
}

// SetSampleRate 设置日志采样率 (0.0-1.0)
func SetSampleRate(rate float64) {
	if rate < 0.0 {
		rate = 0.0
	}
	if rate > 1.0 {
		rate = 1.0
	}
	sampleRate = rate
}

// Init 初始化日志系统
func Init(service string, cfg *entities.AppConfig) {
	fallbackLogger = zerolog.New(os.Stderr).With().Timestamp().Str("service", service).Str("module", "logger_fallback").Logger()
	defaultService = service
	if cfg != nil && cfg.Log != (entities.LogConfig{}) {
		minLogLevel = strings.ToLower(cfg.Log.LogLevel)
	} else {
		minLogLevel = "info"
	}

	if cfg != nil && cfg.Log != (entities.LogConfig{}) {
		switch strings.ToLower(cfg.Log.LogType) {
		case "loki":
			loggerImpl = NewLokiLogger(service, cfg.Loki)
		default:
			loggerImpl = NewZerologLogger(service, minLogLevel)
		}
	} else {
		loggerImpl = NewZerologLogger(service, minLogLevel)
	}

	rootLogger = &Logger{fields: make(map[string]interface{})}
	logChannelCapacity.Set(float64(cap(logChan)))
	wg.Add(1)
	go logProcessor()
}

func logProcessor() {
	defer wg.Done()
	var logs []*LogEntry
	ticker := time.NewTicker(5 * time.Second)
	monitorTicker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer monitorTicker.Stop()

	for {
		select {
		case entry := <-logChan:
			if entry.Context != nil {
				if traceID := entry.Context.Value("trace_id"); traceID != nil {
					entry.Fields["trace_id"] = traceID
				}
				if requestID := entry.Context.Value("request_id"); requestID != nil {
					entry.Fields["request_id"] = requestID
				}
			}

			if !shouldSample() {
				logSampledTotal.WithLabelValues(entry.Level, entry.Service).Inc()
				logPool.Put(entry)
				continue
			}

			logs = append(logs, entry)
			logChannelUsage.Set(float64(len(logChan)))

			if len(logs) >= 10 {
				flushLogs(logs)
				logs = nil
			}
		case <-ticker.C:
			if len(logs) > 0 {
				flushLogs(logs)
				logs = nil
			}
		case <-monitorTicker.C:
			usage := len(logChan)
			capacity := cap(logChan)
			usagePercent := float64(usage) / float64(capacity) * 100
			logChanFullMtx.Lock()
			logChanFull = usagePercent > 80
			logChanFullMtx.Unlock()
		case <-closeChan:
			if len(logs) > 0 {
				flushLogs(logs)
			}
			return
		}
	}
}

func flushLogs(logs []*LogEntry) {
	if len(logs) == 0 {
		return
	}
	startTime := time.Now()
	if err := loggerImpl.Log(logs); err != nil {
		fallbackLogger.Error().Err(err).Msg("Failed to write logs to primary backend")
	}
	logLatency.WithLabelValues("batch").Observe(time.Since(startTime).Seconds())

	for _, logEntry := range logs {
		logEntriesTotal.WithLabelValues(logEntry.Level, logEntry.Service).Inc()
		// 清理 fields 以便复用
		for k := range logEntry.Fields {
			delete(logEntry.Fields, k)
		}
		logPool.Put(logEntry)
	}
}

func IsLogChanFull() bool {
	logChanFullMtx.RLock()
	defer logChanFullMtx.RUnlock()
	return logChanFull
}

func Shutdown() {
	close(closeChan)
	wg.Wait()
}

func (l *Logger) submit(level, msg, module string, err error, ctx context.Context, args ...interface{}) {
	if !shouldLog(level) {
		return
	}

	entry := logPool.Get().(*LogEntry)
	entry.Service = defaultService
	entry.Module = module
	entry.Timestamp = time.Now()
	entry.Level = level
	entry.Message = msg
	if err != nil {
		entry.Error = err.Error()
	} else {
		entry.Error = ""
	}
	entry.Context = ctx

	// 合并预设字段和单次调用字段
	for k, v := range l.fields {
		entry.Fields[k] = v
	}
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				entry.Fields[key] = args[i+1]
			}
		}
	}

	select {
	case logChan <- entry:
	default:
		logDroppedTotal.WithLabelValues(entry.Service).Inc()
		logPool.Put(entry)
	}
}

// With 返回一个带有预设字段的新 Logger
func (l *Logger) With(args ...interface{}) *Logger {
	newFields := make(map[string]interface{}, len(l.fields)+len(args)/2)
	for k, v := range l.fields {
		newFields[k] = v
	}
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				newFields[key] = args[i+1]
			}
		}
	}
	return &Logger{fields: newFields}
}

// Global functions delegating to the root logger
func With(args ...interface{}) *Logger {
	return rootLogger.With(args...)
}

func (l *Logger) Info(msg, module string, args ...interface{}) {
	l.submit("info", msg, module, nil, nil, args...)
}

func Info(msg, module string, args ...interface{}) {
	rootLogger.Info(msg, module, args...)
}

func (l *Logger) Warn(msg, module string, args ...interface{}) {
	l.submit("warn", msg, module, nil, nil, args...)
}

func Warn(msg, module string, args ...interface{}) {
	rootLogger.Warn(msg, module, args...)
}

func (l *Logger) Error(err error, msg, module string, args ...interface{}) {
	l.submit("error", msg, module, err, nil, args...)
}

func Error(err error, msg, module string, args ...interface{}) {
	rootLogger.Error(err, msg, module, args...)
}

func (l *Logger) Debug(msg, module string, args ...interface{}) {
	l.submit("debug", msg, module, nil, nil, args...)
}

func Debug(msg, module string, args ...interface{}) {
	rootLogger.Debug(msg, module, args...)
}

func (l *Logger) InfoWithContext(ctx context.Context, msg, module string, args ...interface{}) {
	l.submit("info", msg, module, nil, ctx, args...)
}

func InfoWithContext(ctx context.Context, msg, module string, args ...interface{}) {
	rootLogger.InfoWithContext(ctx, msg, module, args...)
}

func (l *Logger) WarnWithContext(ctx context.Context, msg, module string, args ...interface{}) {
	l.submit("warn", msg, module, nil, ctx, args...)
}

func WarnWithContext(ctx context.Context, msg, module string, args ...interface{}) {
	rootLogger.WarnWithContext(ctx, msg, module, args...)
}

func (l *Logger) ErrorWithContext(ctx context.Context, err error, msg, module string, args ...interface{}) {
	l.submit("error", msg, module, err, ctx, args...)
}

func ErrorWithContext(ctx context.Context, err error, msg, module string, args ...interface{}) {
	rootLogger.ErrorWithContext(ctx, err, msg, module, args...)
}

func (l *Logger) DebugWithContext(ctx context.Context, msg, module string, args ...interface{}) {
	l.submit("debug", msg, module, nil, ctx, args...)
}

func DebugWithContext(ctx context.Context, msg, module string, args ...interface{}) {
	rootLogger.DebugWithContext(ctx, msg, module, args...)
}
