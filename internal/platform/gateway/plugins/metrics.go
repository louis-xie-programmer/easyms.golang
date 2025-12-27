package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

var (
	// requestDuration 记录请求处理的延迟分布
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Time taken to process request",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method", "status"},
	)

	// requestTotal 记录请求总数
	requestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total number of requests",
		},
		[]string{"service", "method", "status"},
	)
)

func init() {
	// 注册指标到 Prometheus 的默认注册表
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(requestTotal)
}

// MetricsPlugin 负责收集网关的监控指标
type MetricsPlugin struct{}

func NewMetricsPlugin() *MetricsPlugin {
	return &MetricsPlugin{}
}

func (p *MetricsPlugin) Name() string {
	return "metrics"
}

func (p *MetricsPlugin) Order() int {
	// 必须是最先执行的插件之一，以便覆盖整个请求生命周期
	return 0
}

func (p *MetricsPlugin) Execute(ctx *plugin.Context) {
	start := time.Now()

	// 继续执行后续插件
	ctx.Next()

	// 请求处理完成后，收集指标
	duration := time.Since(start).Seconds()
	statusCode := strconv.Itoa(ctx.StatusCode())
	method := ctx.Request.Method

	// 尝试获取目标服务名，如果未匹配到路由，则标记为 "unknown"
	serviceName := "unknown"
	if val, ok := ctx.Get("serviceName"); ok {
		serviceName = val.(string)
	}

	requestDuration.WithLabelValues(serviceName, method, statusCode).Observe(duration)
	requestTotal.WithLabelValues(serviceName, method, statusCode).Inc()
}
