package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/logger"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// ProxyPlugin is the final plugin in the chain that forwards the request to the upstream service.
type ProxyPlugin struct {
	proxyPool *sync.Pool
}

// NewProxyPlugin creates a new proxy plugin.
func NewProxyPlugin() *ProxyPlugin {
	return &ProxyPlugin{
		proxyPool: &sync.Pool{
			New: func() interface{} {
				return &httputil.ReverseProxy{}
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
		// This should not happen if the routing plugin is configured correctly.
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
		// Clear the RequestURI to avoid issues with some servers
		req.RequestURI = ""
	}

	// Set a custom error handler to capture upstream errors.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error(err, "reverse proxy error", "proxy", "target", upstreamURL.Host)
		// Do not write the header here. Instead, set the error in the context
		// so the circuit breaker plugin can handle the response.
		ctx.Set(UpstreamErrorKey, err)
	}

	// The actual forwarding happens here.
	proxy.ServeHTTP(ctx.ResponseWriter, ctx.Request)
}
