// Package logger 提供了统一的日志框架。
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/natefinch/lumberjack" // 引入 lumberjack 用于日志切割和轮转
	"github.com/rs/zerolog"
)

// ZerologLogger 是 BackendLogger 接口的一个实现，它使用 zerolog 库将日志写入本地文件。
// 它支持日志文件的按天轮转和基于大小的切割。
type ZerologLogger struct {
	service     string
	logLevel    string
	logger      zerolog.Logger
	rotator     *lumberjack.Logger // lumberjack 实例，用于管理日志文件
	currentDay  string             // 当前日期字符串，用于判断是否需要按天轮转
	initialized bool
	mu          sync.Mutex
}

// NewZerologLogger 创建一个新的 ZerologLogger 实例。
// service: 服务名称，将作为日志的固定字段。
// level: 初始的最低日志级别。
func NewZerologLogger(service, level string) BackendLogger {
	z := &ZerologLogger{
		service:     service,
		logLevel:    strings.ToLower(level),
		currentDay:  time.Now().Format("2006-01-02"),
		initialized: false,
	}
	z.rotateLogger() // 初始化时立即创建或打开当天的日志文件
	return z
}

// rotateLogger 负责日志文件的轮转。
// 它会在每天第一次写入日志时，或者在日志级别变更时被调用，
// 以创建新的日志文件或重新配置 logger 实例。
func (z *ZerologLogger) rotateLogger() {
	z.mu.Lock()
	defer z.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	// 如果日期未变且已初始化，则无需任何操作
	if today == z.currentDay && z.initialized {
		return
	}

	// 配置 lumberjack 用于日志切割
	logPath := getLogFilePath(today)
	rotator := &lumberjack.Logger{
		Filename:   logPath, // 日志文件路径
		MaxSize:    10,      // 每个日志文件的最大尺寸 (MB)
		MaxBackups: 5,       // 保留的旧日志文件最大数量
		MaxAge:     7,       // 旧日志文件最长保留天数
		Compress:   false,   // 是否压缩旧日志文件
	}

	zerolog.TimeFieldFormat = time.RFC3339 // 设置时间戳格式
	// 创建一个新的 zerolog 实例，输出到 rotator
	newLogger := zerolog.New(rotator).With().Timestamp().Str("service", z.service).Logger()

	// 根据配置的日志级别设置 zerolog 的级别
	switch z.logLevel {
	case "debug":
		newLogger = newLogger.Level(zerolog.DebugLevel)
	case "info":
		newLogger = newLogger.Level(zerolog.InfoLevel)
	case "warn":
		newLogger = newLogger.Level(zerolog.WarnLevel)
	case "error":
		newLogger = newLogger.Level(zerolog.ErrorLevel)
	default:
		newLogger = newLogger.Level(zerolog.InfoLevel)
	}

	z.logger = newLogger
	z.rotator = rotator
	z.currentDay = today
	z.initialized = true
}

// getLogFilePath 根据日期生成日志文件的完整路径。
// 日志文件会存储在 "logs/{YYYY-MM-DD}/app.log" 中。
func getLogFilePath(date string) string {
	logDir := filepath.Join("logs", date)
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		// 如果创建日志目录失败，这是一个严重问题，直接 panic
		panic("创建日志目录失败: " + err.Error())
	}
	return filepath.Join(logDir, "app.log")
}

// Log 实现了 BackendLogger 接口的 Log 方法。
// 它接收一批日志条目，并逐条写入文件。
func (l *ZerologLogger) Log(logs []*LogEntry) error {
	l.rotateLogger() // 每次写入前都检查是否需要轮转日志文件
	for _, entry := range logs {
		// 再次检查日志级别，因为全局级别可能在运行时发生变化
		if !shouldLog(entry.Level) {
			continue
		}
		l.writeLog(entry)
	}
	return nil
}

// writeLog 将单个 LogEntry 转换为 zerolog 的事件并写入。
func (l *ZerologLogger) writeLog(entry *LogEntry) {
	var event *zerolog.Event
	switch entry.Level {
	case "debug":
		event = l.logger.Debug()
	case "info":
		event = l.logger.Info()
	case "warn":
		event = l.logger.Warn()
	case "error":
		event = l.logger.Error()
		if entry.Error != "" {
			event = event.Err(fmt.Errorf(entry.Error))
		}
	default:
		event = l.logger.Info()
	}

	// 添加模块信息、所有自定义字段，并最终写入日志消息
	event.Str("module", entry.Module).Fields(entry.Fields).Msg(entry.Message)
}

// UpdateLogLevel 动态更新日志记录器的级别。
func (l *ZerologLogger) UpdateLogLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logLevel = strings.ToLower(level)
	l.rotateLogger() // 重新初始化 logger 以应用新的级别
}

// Close 关闭底层的 lumberjack 日志轮转器。
func (l *ZerologLogger) Close() {
	if l.rotator != nil {
		l.rotator.Close()
	}
}
