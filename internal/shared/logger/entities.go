package logger

import (
	"context"
	"time"
)

// LogEntry 定义日志条目结构
type LogEntry struct {
	Service   string          `json:"service"`   // 服务名称
	Module    string          `json:"module"`    // 模块名称
	Timestamp time.Time       `json:"timestamp"` // 时间戳
	Level     string          `json:"level"`     // 日志级别
	Message   string          `json:"message"`   // 日志消息
	Error     string          `json:"error"`     // 错误信息
	Extra     [][]string      `json:"extra"`     // 额外字段
	Context   context.Context `json:"-"`         // 上下文信息（不序列化）
}
