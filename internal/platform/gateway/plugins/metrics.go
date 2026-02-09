// Package plugins contains all gateway plugins.
package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/logger"
	"github.com/prometheus/client_golang/prometheus"
	"math"
	"math/rand"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Gateway request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "route", "instance", "method", "status"},
	)
	requestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total gateway requests",
		},
		[]string{"service", "route", "instance", "method", "status"},
	)
)

func init() {
	rand.Seed(time.Now().UnixNano())
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(requestTotal)
}

// MetricsPlugin collects gateway request metrics.
type MetricsPlugin struct {
	sampleRateBits uint64
}

func NewMetricsPlugin(sampleRate float64) *MetricsPlugin {
	p := &MetricsPlugin{}
	p.UpdateSampleRate(sampleRate)
	return p
}

func (p *MetricsPlugin) UpdateSampleRate(sampleRate float64) {
	original := sampleRate
	if sampleRate < 0 {
		sampleRate = 0
	}
	if sampleRate > 1 {
		sampleRate = 1
	}
	if original != sampleRate {
		logger.Warn("Metrics sample rate out of range, clamped", "gateway", "value", original, "clamped", sampleRate)
	}
	prev := p.sampleRate()
	atomic.StoreUint64(&p.sampleRateBits, math.Float64bits(sampleRate))
	if prev != sampleRate {
		logger.Info("Metrics sample rate updated", "gateway", "sample_rate", sampleRate)
	}
}

func (p *MetricsPlugin) sampleRate() float64 {
	return math.Float64frombits(atomic.LoadUint64(&p.sampleRateBits))
}

func (p *MetricsPlugin) Name() string {
	return "metrics"
}

func (p *MetricsPlugin) Order() int {
	return 0
}

func (p *MetricsPlugin) Execute(ctx *plugin.Context) {
	start := time.Now()
	ctx.Next()

	duration := time.Since(start).Seconds()
	statusCode := strconv.Itoa(ctx.StatusCode())
	method := ctx.Request.Method

	serviceName := "unknown"
	if val, ok := ctx.Get(ServiceNameKey); ok {
		if name, ok := val.(string); ok && name != "" {
			serviceName = name
		}
	}
	routeLabel := "unknown"
	if val, ok := ctx.Get(RoutePathPrefixKey); ok {
		if route, ok := val.(string); ok && route != "" {
			routeLabel = route
		}
	}
	instanceLabel := "unknown"
	if val, ok := ctx.Get(UpstreamInstanceKey); ok {
		if inst, ok := val.(string); ok && inst != "" {
			instanceLabel = inst
		}
	}

	if ctx.StatusCode() >= 500 || rand.Float64() < p.sampleRate() {
		requestDuration.WithLabelValues(serviceName, routeLabel, instanceLabel, method, statusCode).Observe(duration)
	}
	requestTotal.WithLabelValues(serviceName, routeLabel, instanceLabel, method, statusCode).Inc()
}
