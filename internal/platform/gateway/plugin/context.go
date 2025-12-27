package plugin

import (
	"net/http"
	"sync"
)

// Context 封装了单次请求的上下文信息，在插件链中传递。
type Context struct {
	Request        *http.Request
	ResponseWriter http.ResponseWriter

	// writerWrapper 用于捕获响应状态码
	writerWrapper *responseWriterWrapper

	// data 是一个用于在插件之间传递数据的键值存储。
	// 例如，认证插件可以将用户信息存入，供后续插件使用。
	data map[string]interface{}
	mu   sync.RWMutex

	// 插件链相关
	plugins []Plugin
	index   int
}

// responseWriterWrapper 包装 http.ResponseWriter 以捕获状态码
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// NewContext 创建一个新的 Context 实例。
func NewContext(w http.ResponseWriter, r *http.Request, plugins []Plugin) *Context {
	wrapper := &responseWriterWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // 默认为 200
	}
	return &Context{
		Request:        r,
		ResponseWriter: wrapper, // 使用包装器
		writerWrapper:  wrapper,
		data:           make(map[string]interface{}),
		plugins:        plugins,
		index:          -1,
	}
}

// Next 调用插件链中的下一个插件。
// 这是实现插件链式调用的核心。
func (c *Context) Next() {
	c.index++
	for c.index < len(c.plugins) {
		c.plugins[c.index].Execute(c)
		c.index++
	}
}

// Set 将一个键值对存入 Context 的数据存储中。
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Get 从 Context 的数据存储中获取一个值。
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists := c.data[key]
	return value, exists
}

// StatusCode 返回响应的状态码。
func (c *Context) StatusCode() int {
	return c.writerWrapper.statusCode
}
