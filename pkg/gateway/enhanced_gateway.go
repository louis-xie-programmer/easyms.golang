package gateway

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mercari/go-circuitbreaker"
	"golang.org/x/time/rate"

	"easyms/pkg/config"
	"easyms/pkg/discovery"
	"easyms/pkg/entities"
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	CounterResetInterval time.Duration
	HalfOpenMaxSuccesses int64
	FailureRateWindow    int64
	FailureRateThreshold float64
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	DefaultRate  float64
	DefaultBurst int
	IPLimits     map[string]*struct {
		Rate  float64
		Burst int
	}
	UALimits     map[string]*struct {
		Rate  float64
		Burst int
	}
}

// EnhancedGateway 增强版API网关
type EnhancedGateway struct {
	sd              *discovery.ServiceDiscovery
	proxyPool       *ReverseProxyPool
	httpClient      *http.Client
	useWeighted     bool
	routeRules      []*entities.RouteRule
	routeMutex      sync.RWMutex
	circuitBreakers map[string]*circuitbreaker.CircuitBreaker
	cbMutex         sync.RWMutex
	cbConfig        map[string]*CircuitBreakerConfig
	cbConfigMutex   sync.RWMutex
	rateLimiters    map[string]*rate.Limiter
	rlMutex         sync.RWMutex
	rlConfig        *RateLimitConfig
	rlConfigMutex   sync.RWMutex
}

// NewEnhancedGateway 创建新的增强版API网关实例
func NewEnhancedGateway(sd *discovery.ServiceDiscovery) *EnhancedGateway {
	// 创建带有连接池和超时设置的HTTP客户端
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	
	eg := &EnhancedGateway{
		sd:              sd,
		proxyPool:       &ReverseProxyPool{proxies: make(map[string]*httputil.ReverseProxy)},
		httpClient:      httpClient,
		useWeighted:     false,
		routeRules:      make([]*entities.RouteRule, 0),
		circuitBreakers: make(map[string]*circuitbreaker.CircuitBreaker),
		cbConfig:        make(map[string]*CircuitBreakerConfig),
		rateLimiters:    make(map[string]*rate.Limiter),
		rlConfig: &RateLimitConfig{
			DefaultRate:  100,
			DefaultBurst: 200,
			IPLimits:     make(map[string]*struct{ Rate float64; Burst int }),
			UALimits:     make(map[string]*struct{ Rate float64; Burst int }),
		},
	}
	
	// 初始化默认熔断器配置
	eg.cbConfig["default"] = &CircuitBreakerConfig{
		CounterResetInterval: 10 * time.Second,
		HalfOpenMaxSuccesses: 4,
		FailureRateWindow:    10,
		FailureRateThreshold: 0.4,
	}
	
	return eg
}

// AddRouteRule 添加路由规则
func (g *EnhancedGateway) AddRouteRule(rule *entities.RouteRule) {
	g.routeMutex.Lock()
	defer g.routeMutex.Unlock()
	g.routeRules = append(g.routeRules, rule)
}

// RemoveRouteRule 移除路由规则
func (g *EnhancedGateway) RemoveRouteRule(serviceName string) {
	g.routeMutex.Lock()
	defer g.routeMutex.Unlock()
	
	for i, rule := range g.routeRules {
		if rule.ServiceName == serviceName {
			g.routeRules = append(g.routeRules[:i], g.routeRules[i+1:]...)
			break
		}
	}
}

// GetRouteRules 获取所有路由规则
func (g *EnhancedGateway) GetRouteRules() []*entities.RouteRule {
	g.routeMutex.RLock()
	defer g.routeMutex.RUnlock()
	
	// 返回副本以避免外部修改
	rules := make([]*entities.RouteRule, len(g.routeRules))
	copy(rules, g.routeRules)
	return rules
}

// SetCircuitBreakerConfig 设置熔断器配置
func (g *EnhancedGateway) SetCircuitBreakerConfig(serviceName string, config *CircuitBreakerConfig) {
	g.cbConfigMutex.Lock()
	defer g.cbConfigMutex.Unlock()
	g.cbConfig[serviceName] = config
	
	// 如果已存在该服务的熔断器，重新创建
	g.cbMutex.Lock()
	defer g.cbMutex.Unlock()
	if _, exists := g.circuitBreakers[serviceName]; exists {
		delete(g.circuitBreakers, serviceName)
	}
}

// GetCircuitBreaker 获取服务对应的熔断器
func (g *EnhancedGateway) GetCircuitBreaker(serviceName string) *circuitbreaker.CircuitBreaker {
	g.cbMutex.RLock()
	cb, exists := g.circuitBreakers[serviceName]
	g.cbMutex.RUnlock()
	
	if exists {
		return cb
	}
	
	// 获取配置
	g.cbConfigMutex.RLock()
	config, configExists := g.cbConfig[serviceName]
	if !configExists {
		config = g.cbConfig["default"]
	}
	g.cbConfigMutex.RUnlock()
	
	// 创建新的熔断器
	cb = circuitbreaker.New(
		circuitbreaker.WithOnStateChangeHookFn(func(from, to circuitbreaker.State) {
			log.Printf("[CB][%s] 状态变更: %s -> %s\n", serviceName, from, to)
		}),
		circuitbreaker.WithCounterResetInterval(config.CounterResetInterval),
		circuitbreaker.WithOpenTimeout(60 * time.Second),
		circuitbreaker.WithHalfOpenMaxSuccesses(config.HalfOpenMaxSuccesses),
		circuitbreaker.WithTripFunc(
			circuitbreaker.NewTripFuncFailureRate(config.FailureRateWindow, config.FailureRateThreshold),
		),
	)
	
	// 存储熔断器
	g.cbMutex.Lock()
	g.circuitBreakers[serviceName] = cb
	g.cbMutex.Unlock()
	
	return cb
}

// SetRateLimitConfig 设置限流配置
func (g *EnhancedGateway) SetRateLimitConfig(config *RateLimitConfig) {
	g.rlConfigMutex.Lock()
	defer g.rlConfigMutex.Unlock()
	g.rlConfig = config
}

// GetRateLimiter 获取限流器
func (g *EnhancedGateway) GetRateLimiter(key string, rateLimit, burst int) *rate.Limiter {
	g.rlMutex.RLock()
	limiter, exists := g.rateLimiters[key]
	g.rlMutex.RUnlock()
	
	if exists {
		return limiter
	}
	
	// 创建新的限流器
	limiter = rate.NewLimiter(rate.Limit(rateLimit), burst)
	
	// 存储限流器
	g.rlMutex.Lock()
	g.rateLimiters[key] = limiter
	g.rlMutex.Unlock()
	
	return limiter
}

// matchRoute 匹配路由规则
func (g *EnhancedGateway) matchRoute(path string) *entities.RouteRule {
	g.routeMutex.RLock()
	defer g.routeMutex.RUnlock()
	
	for _, rule := range g.routeRules {
		if strings.HasPrefix(path, rule.PathPrefix) {
			return rule
		}
	}
	
	return nil
}

// applyRouteRule 应用路由规则
func (g *EnhancedGateway) applyRouteRule(req *http.Request, rule *entities.RouteRule) (string, string) {
	service := rule.ServiceName
	path := req.URL.Path
	
	// 去除前缀
	if rule.StripPrefix {
		path = strings.TrimPrefix(path, rule.PathPrefix)
		if path == "" {
			path = "/"
		}
	}
	
	// 路径重写
	if rule.PathRewrite != "" {
		// 注意：这里简化处理，实际应该预编译正则表达式
		// 在生产环境中应该在规则加载时就编译好正则表达式
		re, err := url.ParseRequestURI(rule.PathRewrite)
		if err == nil && re != nil {
			path = re.Path
		}
	}
	
	return service, path
}

// modifyRequest 修改请求
func (g *EnhancedGateway) modifyRequest(req *http.Request, rule *entities.RouteRule) {
	// 添加请求头
	for key, value := range rule.AddHeaders {
		req.Header.Set(key, value)
	}
	
	// 移除请求头
	for _, key := range rule.RemoveHeaders {
		req.Header.Del(key)
	}
}

// modifyResponse 修改响应
func (g *EnhancedGateway) modifyResponse(resp *http.Response, rule *entities.RouteRule) {
	// 这里可以添加响应修改逻辑
	// 例如添加响应头、修改状态码等
}

// checkRateLimit 检查限流
func (g *EnhancedGateway) checkRateLimit(req *http.Request) bool {
	ip := req.RemoteAddr
	ua := req.UserAgent()
	
	g.rlConfigMutex.RLock()
	defer g.rlConfigMutex.RUnlock()
	
	// IP限流
	for cidr, limit := range g.rlConfig.IPLimits {
		// 简化的IP匹配逻辑，实际应该使用net.ParseCIDR等
		if strings.HasPrefix(ip, cidr) {
			limiter := g.GetRateLimiter("ip:"+cidr, int(limit.Rate), limit.Burst)
			if !limiter.Allow() {
				return false
			}
			break
		}
	}
	
	// UA限流
	for pattern, limit := range g.rlConfig.UALimits {
		if strings.Contains(ua, pattern) {
			limiter := g.GetRateLimiter("ua:"+pattern, int(limit.Rate), limit.Burst)
			if !limiter.Allow() {
				return false
			}
			break
		}
	}
	
	// 默认限流
	defaultLimiter := g.GetRateLimiter("default", int(g.rlConfig.DefaultRate), g.rlConfig.DefaultBurst)
	return defaultLimiter.Allow()
}

// ServeHTTP 实现HTTP处理器接口
func (g *EnhancedGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 健康检查端点
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}
	
	// 限流检查
	if !g.checkRateLimit(r) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	
	startTime := time.Now()
	
	// 匹配路由规则
	rule := g.matchRoute(r.URL.Path)
	var service, path string
	
	if rule != nil {
		// 应用路由规则
		service, path = g.applyRouteRule(r, rule)
	} else {
		// 使用默认路由逻辑
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		
		service = parts[1]
		path = "/" + strings.Join(parts[2:], "/")
	}
	
	// 通过服务发现获取服务实例地址
	var target string
	if g.useWeighted {
		target = g.sd.GetWeightedService(service)
	} else {
		target = g.sd.GetService(service)
	}
	
	if target == "" {
		http.Error(w, "service not found", http.StatusServiceUnavailable)
		return
	}
	
	// 构造上游URL
	upstreamURL := "http://" + target + path
	
	// 记录转发日志
	log.Printf("Forwarding => %s (service: %s)", upstreamURL, service)
	
	// 获取反向代理实例
	proxy, err := g.proxyPool.GetProxy("http://" + target)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadGateway)
		return
	}
	
	// 设置自定义传输器和超时
	proxy.Transport = g.httpClient.Transport
	proxy.FlushInterval = time.Millisecond * 100
	
	// 修改Director以支持请求修改
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		
		// 设置正确的Host
		url, _ := url.Parse("http://" + target)
		req.Host = url.Host
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		
		// 应用路由规则中的请求修改
		if rule != nil {
			g.modifyRequest(req, rule)
		}
	}
	
	// 修改ModifyResponse以支持响应修改
	proxy.ModifyResponse = func(resp *http.Response) error {
		if rule != nil {
			g.modifyResponse(resp, rule)
		}
		return nil
	}
	
	// 获取对应服务的熔断器
	breaker := g.GetCircuitBreaker(service)
	
	// 将上游调用封装成函数，用于熔断器执行
	reqFunc := func() (interface{}, error) {
		// 设置上游调用超时
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		
		// 创建一个新的请求
		outReq := r.Clone(ctx)
		outReq.URL, _ = url.Parse(upstreamURL)
		outReq.Host = target
		outReq.RequestURI = ""
		
		// 执行上游请求
		resp, err := g.httpClient.Do(outReq)
		if err != nil {
			return nil, err
		}
		
		return resp, nil
	}
	
	// 通过熔断器执行请求
	result, err := breaker.Do(context.Background(), reqFunc)
	if err != nil {
		requestDuration := time.Since(startTime)
		log.Printf("Circuit open or upstream failed for service=[%s], err=%v, duration=%v\n", service, err, requestDuration)
		http.Error(w, "service unavailable (circuit open or upstream error)", http.StatusServiceUnavailable)
		return
	}
	
	// 成功处理 - 返回上游响应
	resp := result.(*http.Response)
	defer resp.Body.Close()
	
	// 复制响应头和响应体
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	
	requestDuration := time.Since(startTime)
	log.Printf("Request completed for service=[%s], status=%d, duration=%v\n", service, resp.StatusCode, requestDuration)
}

// LoadConfigFromAppConfig 从应用配置加载网关配置
func (g *EnhancedGateway) LoadConfigFromAppConfig() {
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		return
	}
	
	// 加载熔断器配置
	if appConfig.CircuitBreaker != nil {
		for serviceName, cbConfig := range appConfig.CircuitBreaker.Services {
			g.SetCircuitBreakerConfig(serviceName, &CircuitBreakerConfig{
				CounterResetInterval: time.Duration(cbConfig.CounterResetInterval) * time.Second,
				HalfOpenMaxSuccesses: cbConfig.HalfOpenMaxSuccesses,
				FailureRateWindow:    cbConfig.FailureRateWindow,
				FailureRateThreshold: cbConfig.FailureRateThreshold,
			})
		}
	}
	
	// 加载限流配置
	if appConfig.RateLimit != nil {
		rlConfig := &RateLimitConfig{
			DefaultRate:  appConfig.RateLimit.DefaultRate,
			DefaultBurst: appConfig.RateLimit.DefaultBurst,
			IPLimits:     make(map[string]*struct{ Rate float64; Burst int }),
			UALimits:     make(map[string]*struct{ Rate float64; Burst int }),
		}
		
		// IP限制
		for _, ipRule := range appConfig.RateLimit.IPLimits {
			rlConfig.IPLimits[ipRule.CIDR] = &struct {
				Rate  float64
				Burst int
			}{
				Rate:  ipRule.Rate,
				Burst: ipRule.Burst,
			}
		}
		
		// UA限制
		for _, uaRule := range appConfig.RateLimit.UALimits {
			rlConfig.UALimits[uaRule.Pattern] = &struct {
				Rate  float64
				Burst int
			}{
				Rate:  uaRule.Rate,
				Burst: uaRule.Burst,
			}
		}
		
		g.SetRateLimitConfig(rlConfig)
	}
	
	// 加载路由规则
	if appConfig.Gateway != nil {
		g.routeMutex.Lock()
		g.routeRules = appConfig.Gateway.RouteRules
		g.routeMutex.Unlock()
	}
}

// UseWeightedLoadBalancer 启用权重负载均衡
func (g *EnhancedGateway) UseWeightedLoadBalancer(use bool) {
	g.useWeighted = use
}