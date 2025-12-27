package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/discovery"
	"fmt"
	"net/http"
	"strings"
)

// Route represents a routing rule for the gateway.
type Route struct {
	PathPrefix  string
	ServiceName string
	StripPrefix bool
	// ... other routing rule fields can be added here
}

// RoutingPlugin determines the upstream service for a request.
type RoutingPlugin struct {
	serviceDiscovery *discovery.ServiceDiscovery
	routes           []Route
}

// NewRoutingPlugin creates a new routing plugin.
func NewRoutingPlugin(sd *discovery.ServiceDiscovery, routes []Route) *RoutingPlugin {
	return &RoutingPlugin{
		serviceDiscovery: sd,
		routes:           routes,
	}
}

func (p *RoutingPlugin) Name() string {
	return "routing"
}

func (p *RoutingPlugin) Order() int {
	return 30
}

const (
	// UpstreamServiceURLKey is the key used to store the resolved upstream URL in the context.
	UpstreamServiceURLKey = "upstreamServiceUrl"
	// OriginalPathKey is the key for the original request path.
	OriginalPathKey = "originalPath"
)

func (p *RoutingPlugin) Execute(ctx *plugin.Context) {
	originalPath := ctx.Request.URL.Path
	ctx.Set(OriginalPathKey, originalPath)

	// 1. Match Route
	matchedRoute := p.matchRoute(originalPath)
	if matchedRoute == nil {
		http.Error(ctx.ResponseWriter, "no matching route found", http.StatusNotFound)
		return
	}

	serviceName := matchedRoute.ServiceName
	targetPath := originalPath
	if matchedRoute.StripPrefix {
		targetPath = strings.TrimPrefix(targetPath, matchedRoute.PathPrefix)
		if !strings.HasPrefix(targetPath, "/") {
			targetPath = "/" + targetPath
		}
	}

	// Store the service name in the context for other plugins (like CircuitBreaker).
	ctx.Set(ServiceNameKey, serviceName)

	// 2. Service Discovery
	upstreamHost := p.serviceDiscovery.GetService(serviceName)
	if upstreamHost == "" {
		http.Error(ctx.ResponseWriter, fmt.Sprintf("service '%s' not found or unavailable", serviceName), http.StatusServiceUnavailable)
		return
	}

	// 3. Store upstream URL in context for the proxy plugin
	upstreamURL := fmt.Sprintf("http://%s%s", upstreamHost, targetPath)
	ctx.Set(UpstreamServiceURLKey, upstreamURL)

	// 4. Continue to the next plugin
	ctx.Next()
}

func (p *RoutingPlugin) matchRoute(path string) *Route {
	// For a large number of routes, consider a more efficient structure like a radix tree.
	for i := range p.routes {
		if strings.HasPrefix(path, p.routes[i].PathPrefix) {
			return &p.routes[i]
		}
	}
	return nil
}
