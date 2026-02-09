// Package logger 提供统一的日志框架。
package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ZerologStdoutLogger 输出到标准输出（JSON 或 Console）。
type ZerologStdoutLogger struct {
	service  string
	logLevel string
	console  bool
	logger   zerolog.Logger
	mu       sync.Mutex
}

// NewZerologStdoutLogger 创建一个输出到标准输出的日志后端。
func NewZerologStdoutLogger(service, level string, console bool) BackendLogger {
	z := &ZerologStdoutLogger{
		service:  service,
		logLevel: strings.ToLower(level),
		console:  console,
	}
	z.resetLogger()
	return z
}

func (z *ZerologStdoutLogger) resetLogger() {
	z.mu.Lock()
	defer z.mu.Unlock()

	var writer io.Writer = os.Stdout
	if z.console {
		writer = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	}

	newLogger := zerolog.New(writer).With().Timestamp().Str("service", z.service).Logger()
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
}

// Log 实现 BackendLogger。
func (z *ZerologStdoutLogger) Log(logs []*LogEntry) error {
	for _, entry := range logs {
		if !shouldLog(entry.Level) {
			continue
		}
		z.writeLog(entry)
	}
	return nil
}

func (z *ZerologStdoutLogger) writeLog(entry *LogEntry) {
	var event *zerolog.Event
	switch entry.Level {
	case "debug":
		event = z.logger.Debug()
	case "info":
		event = z.logger.Info()
	case "warn":
		event = z.logger.Warn()
	case "error":
		event = z.logger.Error()
		if entry.Error != "" {
			event = event.Err(fmt.Errorf(entry.Error))
		}
	default:
		event = z.logger.Info()
	}
	event.Str("module", entry.Module).Fields(entry.Fields).Msg(entry.Message)
}
