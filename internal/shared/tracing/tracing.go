package tracing

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"net/url"
	"strings"
)

// InitTracerProvider initializes and registers the OpenTelemetry Tracer Provider.
func InitTracerProvider(serviceName, endpoint string) (func(context.Context) error, error) {
	endpoint = normalizeEndpoint(endpoint)
	// Create a new OTLP HTTP exporter.
	// The exporter will connect to the given endpoint and by default append "/v1/traces".
	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(endpoint),
	)
	if err != nil {
		return nil, err
	}

	// Create a new resource representing this service.
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create a new TracerProvider with the batch span processor.
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	// Set the global TracerProvider.
	otel.SetTracerProvider(tp)

	// Set the global TextMapPropagator to use W3C Trace Context.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Return the shutdown function to be called on service exit.
	return tp.Shutdown, nil
}

func normalizeEndpoint(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return trimmed
	}
	if strings.Contains(trimmed, "://") {
		if u, err := url.Parse(trimmed); err == nil {
			if u.Host != "" {
				return u.Host
			}
		}
	}
	if strings.Contains(trimmed, "/") {
		if u, err := url.Parse("http://" + trimmed); err == nil {
			if u.Host != "" {
				return u.Host
			}
		}
		if idx := strings.Index(trimmed, "/"); idx > 0 {
			return trimmed[:idx]
		}
	}
	return trimmed
}
