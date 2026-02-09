// Package logger 提供了统一的日志框架。
package logger

import (
	"context"
	"time"
)

// LogEntry 定义了单个结构化日志条目的标准格式。
// 这个结构体在整个日志框架中传递，并最终由日志后端进行序列化和输出。
type LogEntry struct {
	Service   string                 `json:"service"`   // 产生日志的服务名称
	Module    string                 `json:"module"`    // 产生日志的具体模块或包名
	Timestamp time.Time              `json:"timestamp"` // 日志产生的时间戳
	Level     string                 `json:"level"`     // 日志级别 (例如 "info", "error")
	Message   string                 `json:"message"`   // 日志的主要消息文本
	Error     string                 `json:"error,omitempty"` // 错误信息字符串，如果存在的话 (omitempty 表示为空时在 JSON 中省略)
	Fields    map[string]interface{} `json:"fields"`    // 其他自定义的结构化字段，以键值对形式存在
	Context   context.Context        `json:"-"`         // Go 的上下文对象，用于在日志处理链中传递请求范围的数据 (如 trace_id)，不参与 JSON 序列化
}
