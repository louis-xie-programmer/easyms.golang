// Package plugins 包含了 API 网关的所有插件实现。
package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TracingPlugin 负责为每个请求创建和管理分布式追踪 Span。
// 它是 OpenTelemetry 集成的一部分，用于实现请求的端到端追踪。
type TracingPlugin struct {
	tracer trace.Tracer // OpenTelemetry 的 Tracer 实例
}

// NewTracingPlugin 创建一个新的 TracingPlugin 实例。
// serviceName: 当前服务的名称，用于命名 Tracer。
func NewTracingPlugin(serviceName string) *TracingPlugin {
	return &TracingPlugin{
		// 获取一个命名的 Tracer，通常使用服务名称作为 Tracer 名称。
		tracer: otel.Tracer(serviceName),
	}
}

// Name 返回插件的名称。
func (p *TracingPlugin) Name() string {
	return "tracing"
}

// Order 返回插件的执行顺序。
// -10 表示它应该在插件链中最早执行，甚至在 Metrics 插件之前。
// 这样可以确保捕获到最完整的请求生命周期，包括所有前置处理。
func (p *TracingPlugin) Order() int {
	return -10
}

// Execute 是 Tracing 插件的核心逻辑。
// 它在请求进入时启动一个 Span，在请求处理完成后结束 Span，并记录相关属性。
func (p *TracingPlugin) Execute(ctx *plugin.Context) {
	// 1. 提取上下文 (Extract)
	// 尝试从 HTTP 请求头中提取已有的追踪上下文 (Trace Context)。
	// 这允许我们将网关的 Span 链接到上游 (例如前端、其他服务) 发起的 Trace，形成完整的分布式追踪链。
	propagator := otel.GetTextMapPropagator()
	parentCtx := propagator.Extract(ctx.Request.Context(), propagation.HeaderCarrier(ctx.Request.Header))

	// 2. 启动 Span (Start)
	// 基于提取到的父上下文，创建一个新的 Span 来代表网关对该请求的处理。
	spanName := ctx.Request.Method + " " + ctx.Request.URL.Path // Span 的名称通常是 HTTP 方法和路径
	newCtx, span := p.tracer.Start(parentCtx, spanName, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End() // 确保 Span 在函数返回时结束

	// 记录一些基本的请求属性到 Span 中
	span.SetAttributes(
		attribute.String("http.method", ctx.Request.Method),
		attribute.String("http.url", ctx.Request.URL.String()),
		attribute.String("http.user_agent", ctx.Request.UserAgent()),
		attribute.String("http.client_ip", ctx.Request.RemoteAddr),
	)

	// 3. 更新请求上下文
	// 将包含新 Span 的 Context 设置回 http.Request。
	// 这样，后续的插件 (如 ProxyPlugin) 就可以访问到这个 Span，并将其传播到下游服务。
	ctx.Request = ctx.Request.WithContext(newCtx)

	// 4. 继续执行后续插件
	ctx.Next()

	// 5. 记录响应状态 (Post-processing)
	// 请求处理完成后，记录响应状态码到 Span 中。
	statusCode := ctx.StatusCode()
	span.SetAttributes(attribute.Int("http.status_code", statusCode))

	// 如果状态码是 5xx，则将 Span 标记为错误。
	if statusCode >= 500 {
		// 可以在这里记录更详细的错误信息，如果 Context 中有错误对象的话。
		span.RecordError(nil)
	}

	// 如果路由插件已设置服务名，则记录到 Span 中。
	if serviceName, ok := ctx.Get(ServiceNameKey); ok {
		span.SetAttributes(attribute.String("upstream.service", serviceName.(string)))
	}
}
