// entry.go 定义日志条目数据结构

package logger

import "time"

// LogEntry 定义标准化日志条目
type LogEntry struct {
	Service   string     `json:"service,omitempty"` // 服务名称
	Module    string     `json:"module,omitempty"`  // 模块名称(如果时网站则为url)
	Level     string     `json:"level"`             // 日志级别
	Message   string     `json:"message"`           // 日志内容
	Error     string     `json:"error,omitempty"`   // 错误信息
	Timestamp time.Time  `json:"timestamp"`         // 时间戳
	Extra     [][]string `json:"extra,omitempty"`   // 额外的信息
}
