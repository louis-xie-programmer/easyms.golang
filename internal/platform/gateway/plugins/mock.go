// Package plugins 包含了 API 网关的所有插件实现。
package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"net/http"
)

const (
	// MockHeader 是用于触发 Mock 响应的 HTTP 请求头键名。
	// 当请求头中包含此键且值为 "true" 时，Mock 插件将拦截请求并返回预设的 Mock 响应。
	MockHeader = "X-Mock-Response"
)

// MockPlugin 提供 Mock 响应功能，主要用于开发和测试环境。
// 它可以让开发者在后端服务尚未完全就绪时，模拟接口响应，从而加速前端或客户端的开发。
type MockPlugin struct{}

// NewMockPlugin 创建并返回一个新的 MockPlugin 实例。
func NewMockPlugin() *MockPlugin {
	return &MockPlugin{}
}

// Name 返回插件的名称。
func (p *MockPlugin) Name() string {
	return "mock"
}

// Order 返回插件的执行顺序。
// 5 表示它应该在插件链中非常靠前的位置执行，
// 因为 Mock 插件通常会直接返回响应，从而短路整个请求处理流程。
func (p *MockPlugin) Order() int {
	return 5
}

// Execute 是 Mock 插件的核心逻辑。
// 它检查请求头中是否存在特定的 MockHeader。
// 如果存在且值为 "true"，则返回一个预设的 Mock 响应，并中断插件链。
func (p *MockPlugin) Execute(ctx *plugin.Context) {
	mockHeaderValue := ctx.Request.Header.Get(MockHeader)

	if mockHeaderValue != "true" {
		// 如果请求头中没有 MockHeader 或者其值不为 "true"，则继续执行下一个插件。
		ctx.Next()
		return
	}

	// 如果 MockHeader 存在且值为 "true"，则返回 Mock 响应并中断插件链。
	ctx.ResponseWriter.Header().Set("Content-Type", "application/json")
	ctx.ResponseWriter.WriteHeader(http.StatusOK)
	ctx.ResponseWriter.Write([]byte(`{"message": "这是来自网关的 Mock 响应。"}`))

	// **重要**: 这里不调用 ctx.Next()，从而有效地短路了后续的插件执行和对上游服务的请求。
}
