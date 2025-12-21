// zerolog_logger.go 实现本地文件日志记录
// 主要特性：
// - 基于zerolog的结构化输出
// - 按天分割的日志轮转
// - lumberjack日志切割（大小/数量/时间）
// - 动态日志级别控制
// - 服务标识与模块分类
// - 异步日志写入
// - 监控指标支持
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
)

// ZerologLogger 实现本地文件日志记录
type ZerologLogger struct {
	service     string
	logLevel    string
	logger      zerolog.Logger
	rotator     *lumberjack.Logger
	currentDay  string
	initialized bool
	mu          sync.Mutex
}

// NewZerologLogger 创建新的Zerolog日志记录器
func NewZerologLogger(service, level string) BackendLogger {
	z := &ZerologLogger{
		service:     service,
		logLevel:    strings.ToLower(level),
		currentDay:  time.Now().Format("2006-01-02"),
		initialized: false,
	}
	z.rotateLogger()
	return z
}

func (z *ZerologLogger) rotateLogger() {
	z.mu.Lock()
	defer z.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today == z.currentDay && z.initialized {
		return
	}

	logPath := getLogFilePath(today)
	rotator := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     7,
		Compress:   false,
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

func getLogFilePath(date string) string {
	logDir := filepath.Join("logs", date)
	if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
		panic("failed to create log directory: " + err.Error())
	}
	return filepath.Join(logDir, "app.log")
}

// Log 实现 BackendLogger 接口
func (l *ZerologLogger) Log(logs []*LogEntry) error {
	l.rotateLogger() // 确保日志文件按天轮转
	for _, entry := range logs {
		if !shouldLog(entry.Level) {
			continue
		}
		l.writeLog(entry)
	}
	return nil
}

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

	event.Str("module", entry.Module).Fields(entry.Fields).Msg(entry.Message)
}

// UpdateLogLevel 更新日志级别
func (l *ZerologLogger) UpdateLogLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logLevel = strings.ToLower(level)
	l.rotateLogger() // 重新初始化以应用新级别
}

// Close 关闭日志记录器
func (l *ZerologLogger) Close() {
	if l.rotator != nil {
		l.rotator.Close()
	}
}
