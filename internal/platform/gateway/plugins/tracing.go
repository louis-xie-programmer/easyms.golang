package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TracingPlugin 负责为每个请求创建和管理分布式追踪 Span。
type TracingPlugin struct {
	tracer trace.Tracer
}

func NewTracingPlugin(serviceName string) *TracingPlugin {
	return &TracingPlugin{
		// 获取一个命名的 Tracer
		tracer: otel.Tracer(serviceName),
	}
}

func (p *TracingPlugin) Name() string {
	return "tracing"
}

func (p *TracingPlugin) Order() int {
	// 必须是最先执行的插件之一（甚至在 Metrics 之前），以便捕获最完整的请求生命周期。
	return -10
}

func (p *TracingPlugin) Execute(ctx *plugin.Context) {
	// 1. 提取上下文 (Extract)
	// 尝试从 HTTP 请求头中提取已有的追踪上下文 (Trace Context)。
	// 这允许我们将网关的 Span 链接到上游（例如前端）发起的 Trace。
	propagator := otel.GetTextMapPropagator()
	parentCtx := propagator.Extract(ctx.Request.Context(), propagation.HeaderCarrier(ctx.Request.Header))

	// 2. 启动 Span (Start)
	// 创建一个新的 Span 来代表网关对该请求的处理。
	spanName := ctx.Request.Method + " " + ctx.Request.URL.Path
	newCtx, span := p.tracer.Start(parentCtx, spanName, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	// 记录一些基本的请求属性
	span.SetAttributes(
		attribute.String("http.method", ctx.Request.Method),
		attribute.String("http.url", ctx.Request.URL.String()),
		attribute.String("http.user_agent", ctx.Request.UserAgent()),
		attribute.String("http.client_ip", ctx.Request.RemoteAddr),
	)

	// 3. 更新请求上下文
	// 将包含新 Span 的 Context 设置回 http.Request，
	// 这样后续的插件（如 ProxyPlugin）就可以访问到这个 Span。
	ctx.Request = ctx.Request.WithContext(newCtx)

	// 4. 继续执行后续插件
	ctx.Next()

	// 5. 记录响应状态 (Post-processing)
	// 请求处理完成后，记录响应状态码和服务名。
	statusCode := ctx.StatusCode()
	span.SetAttributes(attribute.Int("http.status_code", statusCode))

	if statusCode >= 500 {
		span.RecordError(nil) // 可以改进：如果 Context 中有错误对象，应该传进来
	}

	if serviceName, ok := ctx.Get("serviceName"); ok {
		span.SetAttributes(attribute.String("upstream.service", serviceName.(string)))
	}
}
