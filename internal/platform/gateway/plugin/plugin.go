package plugin

// Plugin 是所有网关插件必须实现的接口。
type Plugin interface {
	// Name 返回插件的名称，应保证唯一。
	Name() string

	// Order 返回插件的执行顺序，数字越小越先执行。
	// 建议插件的顺序间隔为10，方便后续插入新插件。
	//
	// 推荐顺序:
	// 0-9:   Mock, 缓存等直接返回响应的插件
	// 10-19: 认证 (Auth)
	// 20-29: 限流 (RateLimit)
	// 30-39: 路由 (Routing)
	// 40-49: 熔断 (CircuitBreaker)
	// 50-89: 请求/响应转换 (Transformation)
	// 90-100: 代理/转发 (Proxy)
	Order() int

	// Execute 执行插件的核心逻辑。
	// 插件通过调用 ctx.Next() 来将控制权传递给链中的下一个插件。
	// 如果插件不调用 ctx.Next()，则请求处理链会在此中断。
	Execute(ctx *Context)
}
