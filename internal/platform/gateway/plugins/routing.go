// Package plugins contains all gateway plugins.
package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/discovery"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type Route struct {
	PathPrefix  string
	ServiceName string
	StripPrefix bool
}

type RoutingPlugin struct {
	serviceDiscovery *discovery.ServiceDiscovery
	routes           []Route
}

func NewRoutingPlugin(sd *discovery.ServiceDiscovery, routes []Route) *RoutingPlugin {
	sort.SliceStable(routes, func(i, j int) bool {
		return len(routes[i].PathPrefix) > len(routes[j].PathPrefix)
	})
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
	UpstreamServiceURLKey = "upstreamServiceUrl"
	OriginalPathKey       = "originalPath"
	ServiceNameKey        = "serviceName"
	RoutePathPrefixKey    = "routePathPrefix"
	UpstreamInstanceKey   = "upstreamInstance"
)

func (p *RoutingPlugin) Execute(ctx *plugin.Context) {
	originalPath := ctx.Request.URL.Path
	ctx.Set(OriginalPathKey, originalPath)

	matchedRoute := p.matchRoute(originalPath)
	if matchedRoute == nil {
		http.Error(ctx.ResponseWriter, "route not found", http.StatusNotFound)
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

	ctx.Set(ServiceNameKey, serviceName)
	ctx.Set(RoutePathPrefixKey, matchedRoute.PathPrefix)

	upstreamHost := p.serviceDiscovery.GetService(serviceName)
	if upstreamHost == "" {
		http.Error(ctx.ResponseWriter, fmt.Sprintf("service '%s' unavailable", serviceName), http.StatusServiceUnavailable)
		return
	}

	upstreamURL := fmt.Sprintf("http://%s%s", upstreamHost, targetPath)
	ctx.Set(UpstreamServiceURLKey, upstreamURL)
	ctx.Set(UpstreamInstanceKey, upstreamHost)

	ctx.Next()
}

func (p *RoutingPlugin) matchRoute(path string) *Route {
	for i := range p.routes {
		if strings.HasPrefix(path, p.routes[i].PathPrefix) {
			return &p.routes[i]
		}
	}
	return nil
}
