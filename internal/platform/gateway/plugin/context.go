// Package plugin 定义了 API 网关插件化架构的核心接口和数据结构。
package plugin

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// Context 封装了单次 HTTP 请求的所有上下文信息，并在插件链中传递。
// 它是插件之间共享数据和控制流程的核心机制。
type Context struct {
	Request        *http.Request      // 当前的 HTTP 请求对象
	ResponseWriter http.ResponseWriter // 当前的 HTTP 响应写入器

	writerWrapper *responseWriterWrapper // 包装 ResponseWriter，用于捕获响应状态码

	data map[string]interface{} // 用于在插件之间传递数据的键值存储
	mu   sync.RWMutex           // 保护 data 字段的并发访问

	plugins []Plugin // 当前请求需要执行的插件链
	index   int      // 当前正在执行的插件在 plugins 列表中的索引
}

// responseWriterWrapper 包装了标准的 http.ResponseWriter，
// 目的是为了捕获响应的状态码，以便在插件链中进行记录或判断。
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode  int  // 捕获到的 HTTP 状态码
	wroteHeader bool // 标记是否已经写入了响应头
}

// WriteHeader 实现了 http.ResponseWriter 接口的 WriteHeader 方法。
// 它会捕获状态码，并确保只写入一次响应头。
func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return // 已经写入过响应头，忽略后续调用
	}
	w.statusCode = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write 实现了 http.ResponseWriter 接口的 Write 方法。
// 它会在写入响应体之前，确保响应头已经被写入 (如果尚未写入，则默认状态码为 200 OK)。
func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK) // 如果没有显式设置状态码，则默认为 200 OK
	}
	return w.ResponseWriter.Write(b)
}

// Flush 兼容流式响应（SSE/Streaming）。
func (w *responseWriterWrapper) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack 兼容 WebSocket 等协议升级。
func (w *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

// Push 兼容 HTTP/2 Server Push。
func (w *responseWriterWrapper) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

// NewContext 创建一个新的 Context 实例。
// w: 原始的 http.ResponseWriter。
// r: 原始的 *http.Request。
// plugins: 当前请求需要执行的插件列表。
func NewContext(w http.ResponseWriter, r *http.Request, plugins []Plugin) *Context {
	wrapper := &responseWriterWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // 默认状态码为 200 OK
	}
	return &Context{
		Request:        r,
		ResponseWriter: wrapper, // 将原始 ResponseWriter 替换为我们的包装器
		writerWrapper:  wrapper,
		data:           make(map[string]interface{}), // 初始化数据存储
		plugins:        plugins,
		index:          -1, // 初始索引为 -1，表示 Next() 第一次调用时会从第 0 个插件开始
	}
}

// Next 调用插件链中的下一个插件。
// 这是实现插件链式调用的核心机制。
// 每个插件在执行完自己的逻辑后，如果希望请求继续被后续插件处理，就必须调用 ctx.Next()。
// 如果插件不调用 Next()，则插件链会在此中断。
func (c *Context) Next() {
	c.index++
	if c.index < len(c.plugins) {
		c.plugins[c.index].Execute(c)
	}
}

// Set 将一个键值对存入 Context 的数据存储中。
// 插件可以使用此方法将数据传递给后续插件。
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Get 从 Context 的数据存储中获取一个值。
// 插件可以使用此方法获取前序插件设置的数据。
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists := c.data[key]
	return value, exists
}

// StatusCode 返回当前响应的状态码。
// 这个状态码是由 responseWriterWrapper 捕获的。
func (c *Context) StatusCode() int {
	return c.writerWrapper.statusCode
}
