// Package gateway 实现了 API 网关的核心逻辑。
//
// 网关采用插件化架构，其核心职责简化为：
// 1. 注册、加载和管理一系列插件 (Plugin)。
// 2. 作为 http.Handler 接收所有传入的 HTTP 请求。
// 3. 为每个请求创建一个独立的插件执行上下文 (plugin.Context)。
// 4. 按照预设的顺序，在一个链条中依次执行所有插件。
package gateway

import (
	"easyms/internal/platform/gateway/plugin"
	"net/http"
	"sort"
	"sync"
)

// Gateway 是 API 网关的核心结构体。
// 它实现了 http.Handler 接口，因此可以被用作一个标准的 HTTP 服务器处理器。
// 它维护一个插件列表，并负责按顺序执行它们。
type Gateway struct {
	plugins []plugin.Plugin // 存储所有已注册的插件
	mu      sync.RWMutex    // 用于保护插件列表的并发读写
}

// NewGateway 创建一个新的、空的 API 网关实例。
func NewGateway() *Gateway {
	return &Gateway{
		plugins: make([]plugin.Plugin, 0),
	}
}

// AddPlugin 向网关注册一个或多个插件。
// 每次添加插件后，都会根据插件的 Order() 方法返回值对整个插件列表进行重新排序，
// 确保插件总是按照正确的优先级执行。
func (g *Gateway) AddPlugin(plugins ...plugin.Plugin) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.plugins = append(g.plugins, plugins...)

	// 根据 Order() 对插件进行稳定排序，Order 值小的插件会优先执行。
	sort.SliceStable(g.plugins, func(i, j int) bool {
		return g.plugins[i].Order() < g.plugins[j].Order()
	})
}

// ServeHTTP 实现了 http.Handler 接口，是网关处理所有请求的唯一入口。
// 对于每个请求，它会创建一个新的插件执行上下文，并启动插件链的执行。
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 为每个请求创建一个新的上下文。
	// 为了线程安全，这里传递的是插件列表的一个副本。
	// 这可以防止在请求处理过程中，因其他 goroutine 调用 AddPlugin 而导致插件列表被修改。
	g.mu.RLock()
	pluginsCopy := make([]plugin.Plugin, len(g.plugins))
	copy(pluginsCopy, g.plugins)
	g.mu.RUnlock()

	ctx := plugin.NewContext(w, r, pluginsCopy)

	// 2. 调用 ctx.Next() 来启动插件链的执行。
	// 第一个插件将被调用，然后它可以通过调用自己的 ctx.Next() 将控制权传递给下一个插件。
	ctx.Next()
}

// HealthCheck 提供一个简单的健康检查端点。
// 它可以被服务发现系统 (如 Consul) 用来监控网关实例的健康状况。
func (g *Gateway) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
