package tracing

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitTracerProvider 初始化并注册 OpenTelemetry Tracer Provider。
// serviceName: 在 Jaeger UI 中显示的服务名。
// endpoint: Jaeger OTLP HTTP Collector 的地址 (例如: http://localhost:14268/api/traces)。
// 返回一个 shutdown 函数，用于在服务关闭时调用，以确保所有追踪数据都被发送。
func InitTracerProvider(serviceName, endpoint string) (func(context.Context) error, error) {
	// 创建一个通过 HTTP 将追踪数据发送到 Jaeger 的 Exporter。
	// WithInsecure 选项禁用了客户端的 TLS 验证，在开发环境中很方便。
	exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}

	// 创建一个 Resource，它代表了产生遥测数据的实体（即当前服务）。
	// ServiceNameKey 是一个标准键，APM 系统会用它来识别服务。
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// 创建一个 TracerProvider。
	// 使用 BatchSpanProcessor 批量异步发送数据，这在生产环境中是推荐的做法。
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	// 将我们创建的 TracerProvider 设置为全局实例。
	// 这样，任何地方通过 otel.Tracer() 获取的 tracer 都将使用这个配置。
	otel.SetTracerProvider(tp)

	// 设置全局的 TextMapPropagator。
	// 这是实现跨服务上下文传播的关键。
	// W3C Trace Context (traceparent, tracestate) 是目前最广泛接受的标准。
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// 返回 TracerProvider 的 Shutdown 方法，以便在服务优雅退出时调用。
	return tp.Shutdown, nil
}
