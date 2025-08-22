package logger

import (
	"fmt"
	"github.com/go-kit/log"
	"time"
)

// KitLoggerAdapter 适配 go-kit 的 Logger 接口
type KitLoggerAdapter struct{}

// NewKitLoggerAdapter 返回一个 go-kit 兼容的 logger
func NewKitLoggerAdapter() log.Logger {
	return &KitLoggerAdapter{}
}

// Log 实现 go-kit 的 Logger 接口
func (a *KitLoggerAdapter) Log(keyvals ...interface{}) error {
	// 直接支持 LogEntry
	if entry, ok := keyvals[0].(LogEntry); ok {
		if entry.Service == "" {
			entry.Service = defaultService
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now()
		}

		if entry.Level == "" {
			entry.Level = "info"
		}

		if len(keyvals) > 1 {
			for i := 1; i < len(keyvals); i += 2 {
				key, ok := keyvals[i].(string)
				if !ok {
					key = fmt.Sprintf("invalid_%d", i)
				}
				val := fmt.Sprint(keyvals[i+1])

				switch key {
				case "level":
					entry.Level = val
				default:
					entry.Extra = append(entry.Extra, []string{key, val})
				}
			}
		}

		if !shouldLog(entry.Level) {
			return nil
		}

		logChan <- entry
		return nil
	}

	if len(keyvals)%2 != 0 {
		keyvals = append(keyvals, "(MISSING)")
	}

	entry := LogEntry{
		Service:   defaultService,
		Timestamp: time.Now(),
	}

	var extras [][]string

	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprintf("invalid_%d", i)
		}
		val := fmt.Sprint(keyvals[i+1])

		switch key {
		case "level":
			entry.Level = val
		case "msg", "message":
			entry.Message = val
		case "module":
			entry.Module = val
		case "error":
			entry.Error = val
		default:
			extras = append(extras, []string{key, val})
		}
	}

	if len(extras) > 0 {
		entry.Extra = extras
	}

	// 丢到全局异步日志通道
	logChan <- entry
	return nil
}
