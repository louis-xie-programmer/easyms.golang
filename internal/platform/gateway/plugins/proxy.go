package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// ProxyPlugin is the final plugin in the chain that forwards the request to the upstream service.
type ProxyPlugin struct {
	proxyPool *sync.Pool
}

// NewProxyPlugin creates a new proxy plugin with a configured transport.
func NewProxyPlugin(cfg *models.ProxyConfig) *ProxyPlugin {
	// Provide default transport settings if no config is given.
	if cfg == nil {
		cfg = &models.ProxyConfig{
			ConnectTimeout:        5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
		}
	}

	// Create a custom transport with configured timeouts and connection pooling.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.ConnectTimeout,
			KeepAlive: 30 * time.Second, // Default keep-alive
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second, // Default TLS handshake timeout
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
	}

	return &ProxyPlugin{
		proxyPool: &sync.Pool{
			New: func() interface{} {
				// Create a new ReverseProxy for each request, but reuse the transport.
				return &httputil.ReverseProxy{
					Transport: transport,
				}
			},
		},
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
		err := fmt.Errorf("upstream URL not found in context")
		logger.Error(err, "proxy plugin execution failed", "proxy")
		http.Error(ctx.ResponseWriter, "Internal Server Error: upstream URL not set", http.StatusInternalServerError)
		ctx.Set(UpstreamErrorKey, err) // Set error for circuit breaker
		return
	}

	upstreamURL, err := url.Parse(upstreamURLStr.(string))
	if err != nil {
		logger.Error(err, "failed to parse upstream URL", "proxy", "url", upstreamURLStr)
		http.Error(ctx.ResponseWriter, "Internal Server Error: invalid upstream URL", http.StatusInternalServerError)
		ctx.Set(UpstreamErrorKey, err) // Set error for circuit breaker
		return
	}

	proxy := p.proxyPool.Get().(*httputil.ReverseProxy)
	defer p.proxyPool.Put(proxy)

	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host
		req.URL.Path = upstreamURL.Path
		req.Host = upstreamURL.Host
		req.RequestURI = ""

		// Inject Trace Context into the downstream request headers.
		// This allows the upstream service to extract the trace ID and continue the trace.
		otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error(err, "reverse proxy error", "proxy", "target", upstreamURL.Host)
		ctx.Set(UpstreamErrorKey, err)
	}

	proxy.ServeHTTP(ctx.ResponseWriter, ctx.Request)
}
