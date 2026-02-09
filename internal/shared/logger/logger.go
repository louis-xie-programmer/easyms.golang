// Package logger 提供了一个高性能、结构化、异步的日志框架。
//
// 核心特性:
// - **统一接口**: 定义了 BackendLogger 接口，支持将日志发送到不同的后端 (如 Loki, Zerolog)。
// - **异步处理**: 日志条目被发送到一个缓冲通道，由一个独立的 goroutine 批量处理，避免阻塞业务逻辑。
// - **批量发送**: 日志会累积成批次 (默认10条或每5秒)，然后一次性发送到后端，以提高吞吐量。
// - **优雅关闭**: 提供 Shutdown 函数，确保在程序退出前所有缓冲区的日志都被处理和发送。
// - **结构化日志**: 所有日志都是结构化的，支持键值对形式的字段。
// - **上下文感知**: 自动从 context.Context 中提取 "trace_id" 和 "request_id" 并添加到日志中。
// - **日志采样**: 支持设置采样率，以减少在高流量情况下的日志量。
// - **监控集成**: 内置 Prometheus 指标，用于监控日志处理的速率、延迟、丢弃率等。
// - **降级策略**: 当日志通道满时，会丢弃非错误级别的日志，并使用备用 logger 记录错误，保证系统的稳定性。
package logger

import (
	"context"
	"easyms/internal/shared/models"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// BackendLogger 定义了日志后端需要实现的接口。
type BackendLogger interface {
	// Log 负责将一批日志条目发送到最终目的地 (如控制台、文件、Loki)。
	Log(logs []*LogEntry) error
}

// 全局变量
var (
	loggerImpl     BackendLogger  // 当前使用的日志后端实现
	defaultService string         // 日志中记录的默认服务名称
	minLogLevel    string         // 全局最低日志级别，低于此级别的日志将被忽略
	sampleRate     float64 = 1.0  // 日志采样率 (0.0 到 1.0)，默认为 1.0 (100% 记录)
	fallbackLogger zerolog.Logger // 备用日志记录器，在主日志系统失败时使用
	rootLogger     *Logger        // 全局根日志记录器实例
	isInitialized  bool           // 标记日志系统是否已初始化
	initMux        sync.Mutex     // 用于保护 isInitialized 的互斥锁，确保 Init 只执行一次
)

// 全局日志通道和相关的管理变量
var (
	// logPool 使用 sync.Pool 来复用 LogEntry 对象，减少内存分配和 GC 压力。
	logPool = sync.Pool{
		New: func() interface{} {
			return &LogEntry{
				Fields: make(map[string]interface{}),
			}
		},
	}
	logChan        = make(chan *LogEntry, 1000) // 异步日志处理通道，缓冲区大小为 1000
	closeChan      = make(chan struct{})        // 用于通知 logProcessor goroutine 停止的通道
	wg             sync.WaitGroup               // 用于等待 logProcessor 完成所有日志处理
	logChanFull    bool                         // 标记日志通道是否已满
	logChanFullMtx sync.RWMutex                 // 用于保护 logChanFull 的读写锁
)

// Prometheus 指标定义
var (
	// logEntriesTotal 按级别和服务统计已处理的日志总数。
	logEntriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_entries_total",
			Help: "按级别和服务划分的已处理日志总数。",
		},
		[]string{"level", "service"},
	)
	// logDroppedTotal 按服务统计因通道满而被丢弃的日志总数。
	logDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_dropped_total",
			Help: "因日志通道满而被丢弃的日志总数。",
		},
		[]string{"service"},
	)
	// logLatency 按后端类型统计日志批处理的延迟。
	logLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "log_latency_seconds",
			Help: "日志批处理延迟（秒）。",
		},
		[]string{"backend"},
	)
	// logChannelUsage 实时记录日志通道的当前使用量。
	logChannelUsage = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "log_channel_usage",
			Help: "日志通道缓冲区当前使用量。",
		},
	)
	// logChannelCapacity 记录日志通道的总容量。
	logChannelCapacity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "log_channel_capacity",
			Help: "日志通道缓冲区的总容量。",
		},
	)
	// logSampledTotal 按级别和服务统计被采样丢弃的日志总数。
	logSampledTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "log_sampled_total",
			Help: "被采样策略丢弃的日志总数。",
		},
		[]string{"level", "service"},
	)
)

// Logger 是一个上下文感知的日志记录器，可以包含预设的结构化字段。
type Logger struct {
	fields map[string]interface{}
}

// shouldLog 根据全局最低日志级别判断是否应该记录该级别的日志。
var logLevelOrder = map[string]int{
	"debug": 1,
	"info":  2,
	"warn":  3,
	"error": 4,
	"fatal": 5,
	"panic": 6,
}

func normalizeLogLevel(level string) string {
	level = strings.ToLower(level)
	if _, ok := logLevelOrder[level]; ok {
		return level
	}
	return "info"
}

func logLevelValue(level string) int {
	return logLevelOrder[normalizeLogLevel(level)]
}

func shouldLog(level string) bool {
	return logLevelValue(level) >= logLevelValue(minLogLevel)
}

// shouldSample 根据全局采样率判断是否应该记录本次日志。
func shouldSample() bool {
	return rand.Float64() < sampleRate
}

// SetSampleRate 设置全局日志采样率 (0.0 到 1.0 之间)。
func SetSampleRate(rate float64) {
	if rate < 0.0 {
		rate = 0.0
	}
	if rate > 1.0 {
		rate = 1.0
	}
	sampleRate = rate
}

// Init 初始化日志系统。这是一个必须在应用启动时调用的函数。
// 它会设置日志后端、启动后台处理 goroutine，并确保只执行一次。
func Init(service string, cfg *models.AppConfig) {
	initMux.Lock()
	defer initMux.Unlock()

	if isInitialized {
		return
	}

	// 初始化一个备用 logger，用于在主日志系统出现问题时记录关键错误。
	fallbackLogger = zerolog.New(os.Stderr).With().Timestamp().Str("service", service).Str("module", "logger_fallback").Logger()
	defaultService = service
	rand.Seed(time.Now().UnixNano())

	// 根据配置设置最低日志级别
	if cfg != nil && cfg.Log.LogLevel != "" {
		minLogLevel = normalizeLogLevel(cfg.Log.LogLevel)
	} else {
		minLogLevel = "info" // 默认级别
	}

	// 根据配置选择并初始化日志后端 (Loki 或 Zerolog)
	if cfg != nil && cfg.Log.LogType != "" {
		switch strings.ToLower(cfg.Log.LogType) {
		case "loki":
			loggerImpl = NewLokiLogger(service, cfg.Loki)
		case "console":
			loggerImpl = NewZerologStdoutLogger(service, minLogLevel, true)
		case "json":
			loggerImpl = NewZerologStdoutLogger(service, minLogLevel, false)
		case "local", "file", "zerolog":
			loggerImpl = NewZerologLogger(service, minLogLevel)
		default:
			loggerImpl = NewZerologLogger(service, minLogLevel)
		}
	} else {
		loggerImpl = NewZerologLogger(service, minLogLevel) // 默认后端
	}

	rootLogger = &Logger{fields: make(map[string]interface{})}
	logChannelCapacity.Set(float64(cap(logChan)))
	wg.Add(1)
	go logProcessor() // 启动后台日志处理 goroutine

	isInitialized = true
}

// IsInitialized 返回日志系统是否已经初始化。
func IsInitialized() bool {
	initMux.Lock()
	defer initMux.Unlock()
	return isInitialized
}

// logProcessor 是后台运行的核心日志处理 goroutine。
// 它从 logChan 中消费日志，将它们分批，并定时发送到后端。
func logProcessor() {
	defer wg.Done()
	var logs []*LogEntry
	ticker := time.NewTicker(5 * time.Second)       // 每5秒强制刷一次日志
	monitorTicker := time.NewTicker(30 * time.Second) // 每30秒监控一次通道使用率
	defer ticker.Stop()
	defer monitorTicker.Stop()

	for {
		select {
		case entry := <-logChan:
			// 自动从上下文中注入 trace_id 和 request_id
			if entry.Context != nil {
				if traceID := entry.Context.Value("trace_id"); traceID != nil {
					entry.Fields["trace_id"] = traceID
				}
				if requestID := entry.Context.Value("request_id"); requestID != nil {
					entry.Fields["request_id"] = requestID
				}
			}

			// 执行采样
			if !shouldSample() {
				logSampledTotal.WithLabelValues(entry.Level, entry.Service).Inc()
				logPool.Put(entry) // 回收被采样的日志条目
				continue
			}

			logs = append(logs, entry)
			logChannelUsage.Set(float64(len(logChan)))

			// 当日志批次达到10条时，立即发送
			if len(logs) >= 10 {
				flushLogs(logs)
				logs = nil // 重置批次
			}
		case <-ticker.C:
			// 定时器触发，发送当前批次中积累的日志
			if len(logs) > 0 {
				flushLogs(logs)
				logs = nil
			}
		case <-monitorTicker.C:
			// 定期监控通道使用率，如果超过80%，则设置 logChanFull 标志
			usage := len(logChan)
			capacity := cap(logChan)
			usagePercent := float64(usage) / float64(capacity) * 100
			logChanFullMtx.Lock()
			logChanFull = usagePercent > 80
			logChanFullMtx.Unlock()
		case <-closeChan:
			// 收到关闭信号，发送最后一批日志后退出
			if len(logs) > 0 {
				flushLogs(logs)
			}
			return
		}
	}
}

// flushLogs 将一批日志发送到后端，并更新监控指标。
func flushLogs(logs []*LogEntry) {
	if len(logs) == 0 {
		return
	}
	startTime := time.Now()
	if err := loggerImpl.Log(logs); err != nil {
		// 如果主后端失败，使用备用 logger 记录错误
		fallbackLogger.Error().Err(err).Msg("向主日志后端写入日志失败")
	}
	logLatency.WithLabelValues("batch").Observe(time.Since(startTime).Seconds())

	// 回收 LogEntry 对象到对象池
	for _, logEntry := range logs {
		logEntriesTotal.WithLabelValues(logEntry.Level, logEntry.Service).Inc()
		// 清理 fields 以便复用
		for k := range logEntry.Fields {
			delete(logEntry.Fields, k)
		}
		logPool.Put(logEntry)
	}
}

// IsLogChanFull 返回日志通道是否已满的标志。
func IsLogChanFull() bool {
	logChanFullMtx.RLock()
	defer logChanFullMtx.RUnlock()
	return logChanFull
}

// Shutdown 优雅地关闭日志系统，确保所有缓冲的日志都被处理。
func Shutdown() {
	close(closeChan)
	wg.Wait()
}

// submit 是所有日志记录函数的内部实现。
// 它从对象池获取一个 LogEntry，填充数据，然后尝试发送到日志通道。
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

	// 合并 Logger 实例的预设字段和本次调用的字段
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
		// 成功发送到通道
	default:
		// 通道已满，执行降级策略
		logDroppedTotal.WithLabelValues(entry.Service).Inc()
		// 只允许错误级别的日志通过备用 logger 同步写入
		if level == "error" {
			fallbackLogger.Error().Err(err).Str("module", module).Msg(msg)
		}
		logPool.Put(entry) // 回收被丢弃的日志条目
	}
}

// With 返回一个新的 Logger 实例，该实例继承了当前 logger 的所有字段，并添加了新的预设字段。
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

// --- 全局日志函数 ---
// 这些函数代理到 rootLogger，提供了方便的全局调用方式。

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
