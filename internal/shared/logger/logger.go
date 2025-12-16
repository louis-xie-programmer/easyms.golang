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
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Logger 定义日志记录器接口
// 所有具体的日志实现都需要实现此接口
type Logger interface {
	Log(logs []LogEntry) error // 批量记录日志条目
}

var loggerImpl Logger        // 全局日志实现
var defaultService string    // 默认服务名称
var minLogLevel string       // 最小日志级别
var sampleRate float64 = 1.0 // 采样率，默认1.0表示100%记录

// 全局日志通道和管理器
var (
	logChan        = make(chan LogEntry, 1000) // 日志处理通道，缓冲区大小为1000
	closeChan      = make(chan struct{})       // 关闭通知通道
	wg             sync.WaitGroup              // worker管理器，用于等待所有日志处理完成
	logChanFull    = false                     // 标记日志通道是否已满
	logChanFullMtx sync.RWMutex                // 保护logChanFull的读写锁
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
// 初始化日志系统，包括设置服务名称、日志级别和日志实现
// 同时启动后台日志处理协程
// 参数:
//
//	service: 服务名称，用于标识日志来源
//	cfg:     应用配置指针，包含日志级别和类型配置
//
// 初始化流程：
// 1. 设置全局服务名称和日志级别
// 2. 根据配置创建对应的日志实现
// 3. 启动日志处理协程
func Init(service string, cfg *entities.AppConfig) {
	// 初始化全局服务名称和日志级别
	defaultService = service
	if cfg != nil && cfg.Log != (entities.LogConfig{}) {
		minLogLevel = strings.ToLower(cfg.Log.LogLevel)
	} else {
		minLogLevel = "info"
	}

	// 根据配置创建不同的日志实现
	if cfg != nil && cfg.Log != (entities.LogConfig{}) {
		switch strings.ToLower(cfg.Log.LogType) {
		case "loki":
			// 使用Loki日志系统
			loggerImpl = NewLokiLogger(service, cfg.Loki)
		default:
			// 默认使用Zerolog日志系统
			loggerImpl = NewZerologLogger(service, minLogLevel)
		}
	} else {
		// 使用默认日志实现
		loggerImpl = NewZerologLogger(service, minLogLevel)
	}

	// 初始化监控指标
	logChannelCapacity.Set(float64(cap(logChan)))

	// 启动日志处理worker协程
	wg.Add(1)
	go logProcessor()
}

// logProcessor 处理日志的异步处理器
// 功能：从通道接收日志条目，批量写入持久化存储
// 参数：无
// 返回值：无
// 协程安全：通过waitGroup同步
func logProcessor() {
	// 注册协程退出通知
	defer wg.Done()

	// 初始化日志缓冲区和定时器（5秒刷新间隔）
	var logs []LogEntry
	ticker := time.NewTicker(5 * time.Second)
	monitorTicker := time.NewTicker(30 * time.Second) // 每30秒监控一次
	defer ticker.Stop()
	defer monitorTicker.Stop()

	for {
		// 核心处理循环：
		// 1. 累积日志条目
		// 2. 定时/容量触发写入
		// 3. 处理关闭信号
		select {
		// 接收日志条目
		case entry := <-logChan:
			// 从上下文中提取额外信息
			if entry.Context != nil {
				// 提取trace_id（如果存在）
				if traceID := entry.Context.Value("trace_id"); traceID != nil {
					entry.Extra = append(entry.Extra, []string{"trace_id", fmt.Sprintf("%v", traceID)})
				}

				// 提取request_id（如果存在）
				if requestID := entry.Context.Value("request_id"); requestID != nil {
					entry.Extra = append(entry.Extra, []string{"request_id", fmt.Sprintf("%v", requestID)})
				}
			}

			// 根据采样率决定是否记录该日志
			if !shouldSample() {
				// 增加采样统计
				logSampledTotal.WithLabelValues(entry.Level, entry.Service).Inc()
				continue
			}

			logs = append(logs, entry)
			// 更新监控指标
			logChannelUsage.Set(float64(len(logChan)))

			// 达到批量容量时写入
			// 批量处理提高性能，减少I/O操作
			if len(logs) >= 10 {
				startTime := time.Now()
				err := loggerImpl.Log(logs)
				logLatency.WithLabelValues("batch").Observe(time.Since(startTime).Seconds())

				if err != nil {
					fmt.Println("Failed to log:", err)
				}

				// 增加指标统计
				for _, logEntry := range logs {
					logEntriesTotal.WithLabelValues(logEntry.Level, logEntry.Service).Inc()
				}

				logs = nil
			}
		case <-ticker.C: // 定时触发写入
			// 定时刷新确保日志及时写入，避免数据丢失
			if len(logs) > 0 {
				startTime := time.Now()
				err := loggerImpl.Log(logs)
				logLatency.WithLabelValues("timer").Observe(time.Since(startTime).Seconds())

				if err != nil {
					fmt.Println("Failed to log:", err)
				}

				// 增加指标统计
				for _, logEntry := range logs {
					logEntriesTotal.WithLabelValues(logEntry.Level, logEntry.Service).Inc()
				}

				logs = nil
			}
		case <-monitorTicker.C:
			// 定期报告日志系统状态
			usage := len(logChan)
			capacity := cap(logChan)
			usagePercent := float64(usage) / float64(capacity) * 100

			// 如果使用率超过80%，标记为已满并发出警告
			logChanFullMtx.Lock()
			if usagePercent > 80 {
				logChanFull = true
			} else {
				logChanFull = false
			}
			logChanFullMtx.Unlock()

		// 处理关闭信号
		case <-closeChan:
			// 刷写剩余日志
			if len(logs) > 0 {
				startTime := time.Now()
				err := loggerImpl.Log(logs)
				logLatency.WithLabelValues("shutdown").Observe(time.Since(startTime).Seconds())

				if err != nil {
					fmt.Println("Failed to log:", err)
				}

				// 增加指标统计
				for _, logEntry := range logs {
					logEntriesTotal.WithLabelValues(logEntry.Level, logEntry.Service).Inc()
				}
			}
			logs = nil
			return
		}
	}
}

// IsLogChanFull 检查日志通道是否接近满载
func IsLogChanFull() bool {
	logChanFullMtx.RLock()
	defer logChanFullMtx.RUnlock()
	return logChanFull
}

// Shutdown 优雅关闭日志系统
// 发送关闭信号并等待所有日志处理完成
func Shutdown() {
	close(closeChan)
	wg.Wait()
}

// Info 记录信息日志
func Info(msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "info",
		Message:   msg,
		Extra:     extra,
	}
}

// Error 记录错误日志
func Error(err error, msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "error",
		Message:   msg,
		Error:     err.Error(),
		Extra:     extra,
	}
}

// Warn 记录警告日志
func Warn(msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "warn",
		Message:   msg,
		Extra:     extra,
	}
}

// Debug 记录调试日志
func Debug(msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "debug",
		Message:   msg,
		Extra:     extra,
	}
}

// InfoWithContext 带上下文的信息日志
func InfoWithContext(ctx context.Context, msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "info",
		Message:   msg,
		Extra:     extra,
		Context:   ctx,
	}
}

// ErrorWithContext 带上下文的错误日志
func ErrorWithContext(ctx context.Context, err error, msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "error",
		Message:   msg,
		Error:     err.Error(),
		Extra:     extra,
		Context:   ctx,
	}
}

// WarnWithContext 带上下文的警告日志
func WarnWithContext(ctx context.Context, msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "warn",
		Message:   msg,
		Extra:     extra,
		Context:   ctx,
	}
}

// DebugWithContext 带上下文的调试日志
func DebugWithContext(ctx context.Context, msg, module string, extra [][]string) {
	logChan <- LogEntry{
		Service:   defaultService,
		Module:    module,
		Timestamp: time.Now(),
		Level:     "debug",
		Message:   msg,
		Extra:     extra,
		Context:   ctx,
	}
}
