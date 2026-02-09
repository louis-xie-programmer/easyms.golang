// Package plugins contains all gateway plugins.
package plugins

import (
	"context"
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ProxyPlugin forwards requests to upstream services.
type ProxyPlugin struct {
	proxyPool      *sync.Pool
	sampleRateBits uint64
}

// Prometheus metrics
var (
	upstreamRetryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_upstream_retries_total",
			Help: "Upstream retries by service and instance",
		},
		[]string{"service", "route", "instance", "reason"},
	)
	upstreamFailureTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_upstream_failures_total",
			Help: "Upstream failures by service and instance",
		},
		[]string{"service", "route", "instance", "reason"},
	)
)

func init() {
	prometheus.MustRegister(upstreamRetryTotal)
	prometheus.MustRegister(upstreamFailureTotal)
}

type upstreamMetricsKey struct{}

type upstreamMetricsLabels struct {
	service  string
	route    string
	instance string
}

func withUpstreamLabels(ctx context.Context, service string, route string, instance string) context.Context {
	if service == "" {
		service = "unknown"
	}
	if route == "" {
		route = "unknown"
	}
	if instance == "" {
		instance = "unknown"
	}
	return context.WithValue(ctx, upstreamMetricsKey{}, upstreamMetricsLabels{
		service:  service,
		route:    route,
		instance: instance,
	})
}

func getUpstreamLabels(ctx context.Context) (string, string, string) {
	if ctx == nil {
		return "unknown", "unknown", "unknown"
	}
	if val, ok := ctx.Value(upstreamMetricsKey{}).(upstreamMetricsLabels); ok {
		if val.service == "" {
			val.service = "unknown"
		}
		if val.route == "" {
			val.route = "unknown"
		}
		if val.instance == "" {
			val.instance = "unknown"
		}
		return val.service, val.route, val.instance
	}
	return "unknown", "unknown", "unknown"
}

func retryReason(err error, resp *http.Response) string {
	if err != nil {
		return "error"
	}
	if resp != nil && resp.StatusCode >= 500 {
		return "status_5xx"
	}
	return "unknown"
}

type retryTransport struct {
	base          http.RoundTripper
	maxRetries    int
	backoff       time.Duration
	sampleRatePtr *uint64
}

func (t *retryTransport) sampleRate() float64 {
	if t.sampleRatePtr == nil {
		return 1
	}
	return math.Float64frombits(atomic.LoadUint64(t.sampleRatePtr))
}

func shouldSample(rate float64) bool {
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return false
	}
	return rand.Float64() < rate
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	if t.maxRetries <= 0 || !isIdempotentMethod(req.Method) {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			service, route, instance := getUpstreamLabels(req.Context())
			upstreamFailureTotal.WithLabelValues(service, route, instance, "error").Inc()
		}
		return resp, err
	}

	service, route, instance := getUpstreamLabels(req.Context())
	var lastResp *http.Response
	var lastErr error
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			sleep := t.backoff * time.Duration(1<<uint(attempt-1))
			if sleep > 0 {
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(sleep):
				}
			}
		}

		resp, err := t.base.RoundTrip(req)
		if err == nil && !shouldRetryStatus(resp.StatusCode) {
			return resp, nil
		}

		reason := retryReason(err, resp)
		if attempt < t.maxRetries {
			if shouldSample(t.sampleRate()) {
				upstreamRetryTotal.WithLabelValues(service, route, instance, reason).Inc()
			}
		} else if err != nil {
			upstreamFailureTotal.WithLabelValues(service, route, instance, reason).Inc()
		}

		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		lastResp = resp
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func shouldRetryStatus(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// NewProxyPlugin creates a proxy plugin with a shared transport.
func NewProxyPlugin(cfg *models.ProxyConfig) *ProxyPlugin {
	p := &ProxyPlugin{}
	if cfg == nil {
		cfg = &models.ProxyConfig{
			ConnectTimeout:        5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			MaxConnsPerHost:       0,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxRetries:            2,
			RetryBackoff:          100 * time.Millisecond,
		}
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
	}
	retryingTransport := &retryTransport{
		base:          transport,
		maxRetries:    cfg.MaxRetries,
		backoff:       cfg.RetryBackoff,
		sampleRatePtr: &p.sampleRateBits,
	}

	p.proxyPool = &sync.Pool{
		New: func() interface{} {
			return &httputil.ReverseProxy{
				Transport: retryingTransport,
			}
		},
	}
	p.UpdateUpstreamSampleRate(1)
	return p
}

func (p *ProxyPlugin) UpdateUpstreamSampleRate(sampleRate float64) {
	original := sampleRate
	if sampleRate < 0 {
		sampleRate = 0
	}
	if sampleRate > 1 {
		sampleRate = 1
	}
	if original != sampleRate {
		logger.Warn("Upstream metrics sample rate out of range, clamped", "gateway", "value", original, "clamped", sampleRate)
	}
	prev := math.Float64frombits(atomic.LoadUint64(&p.sampleRateBits))
	atomic.StoreUint64(&p.sampleRateBits, math.Float64bits(sampleRate))
	if prev != sampleRate {
		logger.Info("Upstream metrics sample rate updated", "gateway", "sample_rate", sampleRate)
	}
}

func (p *ProxyPlugin) Name() string {
	return "proxy"
}

func (p *ProxyPlugin) Order() int {
	return 100
}

func (p *ProxyPlugin) Execute(ctx *plugin.Context) {
	upstreamURLStr, ok := ctx.Get(UpstreamServiceURLKey)
	if !ok {
		err := fmt.Errorf("upstream service URL not set")
		logger.Error(err, "proxy execute failed", "proxy")
		http.Error(ctx.ResponseWriter, "upstream URL not configured", http.StatusInternalServerError)
		ctx.Set(UpstreamErrorKey, err)
		return
	}

	upstreamURL, err := url.Parse(upstreamURLStr.(string))
	if err != nil {
		logger.Error(err, "invalid upstream URL", "proxy", "url", upstreamURLStr)
		http.Error(ctx.ResponseWriter, "invalid upstream URL", http.StatusInternalServerError)
		ctx.Set(UpstreamErrorKey, err)
		return
	}

	serviceLabel := "unknown"
	if val, ok := ctx.Get(ServiceNameKey); ok {
		if name, ok := val.(string); ok && name != "" {
			serviceLabel = name
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
	ctx.Request = ctx.Request.WithContext(withUpstreamLabels(ctx.Request.Context(), serviceLabel, routeLabel, instanceLabel))

	proxy := p.proxyPool.Get().(*httputil.ReverseProxy)
	defer p.proxyPool.Put(proxy)

	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		originalQuery := req.URL.RawQuery
		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host
		req.URL.Path = upstreamURL.Path
		req.URL.RawPath = upstreamURL.RawPath
		req.URL.RawQuery = originalQuery
		req.Host = upstreamURL.Host
		req.RequestURI = ""

		clientIP, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			clientIP = req.RemoteAddr
		}
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
		} else {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
		req.Header.Set("X-Forwarded-Host", originalHost)
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)

		otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp != nil && resp.StatusCode >= 500 {
			service, route, instance := getUpstreamLabels(resp.Request.Context())
			upstreamFailureTotal.WithLabelValues(service, route, instance, "status_5xx").Inc()
		}
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error(err, "reverse proxy error", "proxy", "target", upstreamURL.Host)
		ctx.Set(UpstreamErrorKey, err)
	}

	proxy.ServeHTTP(ctx.ResponseWriter, ctx.Request)
}
