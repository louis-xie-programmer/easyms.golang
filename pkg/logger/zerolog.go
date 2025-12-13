// zerolog_logger.go 实现本地文件日志记录
// 主要特性：
// - 基于zerolog的结构化输出
// - 按天分割的日志轮转
// - lumberjack日志切割（大小/数量/时间）
// - 动态日志级别控制
// - 服务标识与模块分类
// - 异步日志写入
package logger

import (
	"easyms/pkg/entitis"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
)

// ZerologLogger 实现本地文件日志记录
// 提供日志轮转、异步写入、多级日志控制等功能
// 使用lumberjack实现日志文件的自动轮转
// 支持按天分割日志文件
type ZerologLogger struct {
	service     string             // 服务名称，用于日志标记
	logLevel    string             // 当前日志级别
	logger      zerolog.Logger     // zerolog 实例
	rotator     *lumberjack.Logger // 日志文件轮转器
	currentDay  string             // 当前日期，用于日志文件按天分割
	initialized bool               // 是否已初始化

	mu sync.Mutex // 互斥锁，用于保护日志写入
}

// getLogFilePath 生成日志文件路径
// 参数:
//
//	date - 日期字符串，用于按天分割日志
//
// 返回:
//
//	完整的日志文件路径
func getLogFilePath(date string) string {
	logDir := filepath.Join("logs", date)
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		panic("failed to create log directory: " + err.Error())
	}
	return filepath.Join(logDir, "app.log")
}

// NewZerologLogger 创建新的Zerolog日志记录器
// 参数:
//
//	service - 服务名称
//	level   - 日志级别(debug/info/warn/error)
//
// 返回:
//
//	*ZerologLogger 实例
func NewZerologLogger(service, level string) *ZerologLogger {
	z := &ZerologLogger{
		service:     service,
		logLevel:    strings.ToLower(level),
		currentDay:  time.Now().Format("2006-01-02"),
		initialized: false,
	}

	z.rotateLogger()
	return z
}

// rotateLogger 实现日志文件轮转
// 功能:
//   - 每天创建新的日志文件
//   - 配置日志轮转策略
//   - 设置日志格式和级别
func (z *ZerologLogger) rotateLogger() {
	z.mu.Lock()
	defer z.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today == z.currentDay && z.initialized {
		return
	}

	logPath := getLogFilePath(today)
	rotator := &lumberjack.Logger{
		Filename:   logPath, // 日志文件路径
		MaxSize:    10,      // 每个日志文件最大10MB
		MaxBackups: 5,       // 保留5个旧日志文件
		MaxAge:     7,       // 日志文件保留7天
		Compress:   false,   // 不压缩旧日志，节省CPU资源
	}

	zerolog.TimeFieldFormat = time.RFC3339
	newLogger := zerolog.New(rotator).With().Timestamp().Str("service", z.service).Logger()

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

// writeLog 写入日志条目
// 根据日志级别将日志条目写入相应的输出
// 参数:
//
//	entry - 日志条目
func (z *ZerologLogger) writeLog(entry entitis.LogEntry) {
	event := z.logger.With().
		Str("app", entry.Service).
		Str("module", entry.Module).
		Fields(entry.Extra).
		Logger()

	switch strings.ToUpper(entry.Level) {
	case "DEBUG":
		event.Debug().Msg(entry.Message)
	case "INFO":
		event.Info().Msg(entry.Message)
	case "WARN":
		event.Warn().Msg(entry.Message)
	case "ERROR":
		event.Error().Str("error", entry.Error).Msg(entry.Message)
	default:
		event.Info().Msg(entry.Message)
	}
}

// Log 实现Logger接口的日志记录方法
// 批量处理日志条目，根据日志级别过滤并写入日志
// 参数:
//
//	logs - 日志条目
func (l *ZerologLogger) Log(logs []entitis.LogEntry) error {
	for _, entry := range logs {
		if !shouldLog(entry.Level) {
			continue
		}
		l.writeLog(entry)
	}
	return nil
}

// Close 关闭日志记录器
// 功能:
//   - 关闭日志工作协程
//   - 等待所有日志写入完成
//   - 关闭日志文件
func (l *ZerologLogger) Close() {
	close(closeChan)
	if l.rotator != nil {
		l.rotator.Close()
	}
}
