// logger.go 提供统一日志抽象接口和异步处理框架

package logger

import (
	"fmt"
	"github.com/louis-xie-programmer/easyms/pkg/config"
	"strings"
	"sync"
	"time"
)

// Logger 定义日志记录器接口
type Logger interface {
	Log(logs []LogEntry) error
}

var loggerImpl Logger
var defaultService string
var minLogLevel string

// 全局日志通道和管理器
var (
	logChan   = make(chan LogEntry, 1000) // 日志处理通道
	closeChan = make(chan struct{})       // 关闭通知通道
	wg        sync.WaitGroup              // worker管理器
)

func shouldLog(level string) bool {
	return strings.Compare(level, minLogLevel) >= 0
}

// Init 初始化日志系统
// 参数:
//
//	service: 服务名称，用于标识日志来源
//	cfg:     应用配置指针，包含日志级别和类型配置
//
// 初始化流程：
// 1. 设置全局服务名称和日志级别
// 2. 根据配置创建对应的日志实现
// 3. 启动日志处理协程
func Init(service string, cfg *config.AppConfig) {
	// 初始化全局服务名称和日志级别
	defaultService = service
	minLogLevel = strings.ToLower(cfg.Log["log_level"])

	// 根据配置创建不同的日志实现
	switch strings.ToLower(minLogLevel) {
	case "loki":
		// 使用Loki日志系统
		loggerImpl = NewLokiLogger(service, *cfg)
	default:
		// 默认使用Zerolog日志系统
		loggerImpl = NewZerologLogger(service, minLogLevel)
	}

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
	defer ticker.Stop()

	for {
		// 核心处理循环：
		// 1. 累积日志条目
		// 2. 定时/容量触发写入
		// 3. 处理关闭信号
		select {
		// 接收日志条目
		case entry := <-logChan:
			logs = append(logs, entry)
			// 达到批量容量时写入
			if len(logs) >= 10 {
				err := loggerImpl.Log(logs)
				if err != nil {
					fmt.Println("Failed to log:", err)
				}
				logs = nil
			}

		// 处理关闭信号
		case <-closeChan:
			// 刷写剩余日志
			if len(logs) > 0 {
				err := loggerImpl.Log(logs)
				if err != nil {
					fmt.Println("Failed to log:", err)
				}
			}
			logs = nil
			return
		}
	}
}

// Shutdown 优雅关闭日志系统
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
