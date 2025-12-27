// gateway.go API网关模块
//
// 经过重构，网关的核心职责简化为：
// 1. 加载和管理插件 (Plugin)。
// 2. 接收 HTTP 请求。
// 3. 创建请求上下文 (Context)。
// 4. 按照顺序执行插件链。
package gateway

import (
	"easyms/internal/platform/gateway/plugin"
	"net/http"
	"sort"
	"sync"
)

// Gateway 是 API 网关的核心结构体。
// 它实现了 http.Handler 接口。
type Gateway struct {
	plugins []plugin.Plugin
	mu      sync.RWMutex
}

// NewGateway 创建一个新的、空的 API 网关实例。
func NewGateway() *Gateway {
	return &Gateway{
		plugins: make([]plugin.Plugin, 0),
	}
}

// AddPlugin 向网关注册一个或多个插件。
// 注册后，插件会根据其 Order() 值被排序。
func (g *Gateway) AddPlugin(plugins ...plugin.Plugin) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.plugins = append(g.plugins, plugins...)

	// 根据 Order() 对插件进行排序，Order 值小的优先执行。
	sort.SliceStable(g.plugins, func(i, j int) bool {
		return g.plugins[i].Order() < g.plugins[j].Order()
	})
}

// ServeHTTP 实现 http.Handler 接口，是网关处理所有请求的入口。
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 为每个请求创建一个新的上下文
	// 注意：这里传递的是插件列表的副本，以保证在请求处理过程中插件列表不变
	g.mu.RLock()
	pluginsCopy := make([]plugin.Plugin, len(g.plugins))
	copy(pluginsCopy, g.plugins)
	g.mu.RUnlock()

	ctx := plugin.NewContext(w, r, pluginsCopy)

	// 2. 启动插件链的执行
	ctx.Next()
}

// HealthCheck 提供一个简单的健康检查端点。
func (g *Gateway) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
