// gateway.go API网关模块
// 主要功能：
// 1. 反向代理功能
// 2. 负载均衡
// 3. 熔断保护
// 4. 请求转发和响应处理
package gateway

import (
	"bytes"
	"context"
	"easyms/internal/platform/gateway/internal/domain/model"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"

	"github.com/mercari/go-circuitbreaker"
	"golang.org/x/time/rate"

	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
)

// Middleware 定义了中间件类型
type Middleware func(http.Handler) http.Handler

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
	UALimits map[string]*struct {
		Rate  float64
		Burst int
	}
}

// ReverseProxyPool 反向代理池，用于缓存和复用ReverseProxy实例
type ReverseProxyPool struct {
	proxies map[string]*httputil.ReverseProxy
	mutex   sync.RWMutex
}

// GetProxy 获取或创建指定目标的反向代理
func (p *ReverseProxyPool) GetProxy(target string) (*httputil.ReverseProxy, error) {
	p.mutex.RLock()
	proxy, exists := p.proxies[target]
	p.mutex.RUnlock()

	if exists {
		return proxy, nil
	}

	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy = httputil.NewSingleHostReverseProxy(u)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
		req.URL.Host = u.Host
		req.URL.Scheme = u.Scheme
	}

	// 设置自定义错误处理器
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error(err, "Reverse proxy error", "gateway", "target", u.Host)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(fmt.Sprintf(`{"error": "upstream service unavailable", "details": "%v"}`, err)))
	}

	p.mutex.Lock()
	p.proxies[target] = proxy
	p.mutex.Unlock()

	return proxy, nil
}

// Gateway API网关结构体
type Gateway struct {
	sd              *discovery.ServiceDiscovery
	proxyPool       *ReverseProxyPool
	httpClient      *http.Client
	useWeighted     bool
	routeRules      []*model.RouteRule
	routeMutex      sync.RWMutex
	circuitBreakers map[string]*circuitbreaker.CircuitBreaker
	cbMutex         sync.RWMutex
	cbConfig        map[string]*CircuitBreakerConfig
	cbConfigMutex   sync.RWMutex
	rateLimiters    map[string]*rate.Limiter
	rlMutex         sync.RWMutex
	rlConfig        *RateLimitConfig
	rlConfigMutex   sync.RWMutex
	config          *model.GatewayConfig
	configMutex     sync.RWMutex
	authCache       *cache.Cache
	clientID        string
	clientSecret    string
	middlewares     []Middleware
}

// NewGateway 创建新的API网关实例
func NewGateway(sd *discovery.ServiceDiscovery) *Gateway {
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

	gateway := &Gateway{
		sd:              sd,
		proxyPool:       &ReverseProxyPool{proxies: make(map[string]*httputil.ReverseProxy)},
		httpClient:      httpClient,
		useWeighted:     false,
		routeRules:      make([]*model.RouteRule, 0),
		circuitBreakers: make(map[string]*circuitbreaker.CircuitBreaker),
		cbConfig:        make(map[string]*CircuitBreakerConfig),
		rateLimiters:    make(map[string]*rate.Limiter),
		rlConfig: &RateLimitConfig{
			DefaultRate:  100,
			DefaultBurst: 200,
			IPLimits: make(map[string]*struct {
				Rate  float64
				Burst int
			}),
			UALimits: make(map[string]*struct {
				Rate  float64
				Burst int
			}),
		},
		authCache:   cache.New(5*time.Minute, 10*time.Minute),
		middlewares: make([]Middleware, 0),
	}

	gateway.cbConfig["default"] = &CircuitBreakerConfig{
		CounterResetInterval: 10 * time.Second,
		HalfOpenMaxSuccesses: 4,
		FailureRateWindow:    10,
		FailureRateThreshold: 0.4,
	}

	// 注册默认中间件
	gateway.Use(gateway.RateLimitMiddleware)
	gateway.Use(gateway.AuthMiddleware)

	return gateway
}

// Use 添加中间件到网关
func (g *Gateway) Use(mw Middleware) {
	g.middlewares = append(g.middlewares, mw)
}

// AuthMiddleware 认证中间件
func (g *Gateway) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 对特定路径（如健康检查）跳过认证
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error": "Missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		if _, found := g.authCache.Get(tokenStr); found {
			next.ServeHTTP(w, r)
			return
		}

		authTarget := g.sd.GetService("auth-svc")
		if authTarget == "" {
			logger.Error(nil, "auth-svc not found in service discovery", "gateway", "path", r.URL.Path)
			http.Error(w, `{"error": "authentication service unavailable"}`, http.StatusInternalServerError)
			return
		}

		verifyURL := fmt.Sprintf("http://%s/oauth2/verify", authTarget)
		req, err := http.NewRequestWithContext(r.Context(), "POST", verifyURL, nil)
		if err != nil {
			http.Error(w, `{"error": "failed to create request"}`, http.StatusInternalServerError)
			return
		}

		req.Header.Set("Authorization", "Bearer "+tokenStr)
		req.SetBasicAuth(g.clientID, g.clientSecret)

		resp, err := g.httpClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			msg := "invalid or expired token"
			if err != nil {
				logger.Warn("Auth verification failed", "gateway", "service", "auth-svc", "error", err)
			} else {
				logger.Warn("Auth verification failed", "gateway", "service", "auth-svc", "status", resp.StatusCode)
			}
			http.Error(w, fmt.Sprintf(`{"error": "%s"}`, msg), http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()

		g.authCache.Set(tokenStr, true, 5*time.Minute)
		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware 限流中间件
func (g *Gateway) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.checkRateLimit(r) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ServeHTTP 实现HTTP处理器接口
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 核心反向代理逻辑
	proxyHandler := http.HandlerFunc(g.reverseProxy)

	// 构建中间件链
	var chainedHandler http.Handler = proxyHandler
	for i := len(g.middlewares) - 1; i >= 0; i-- {
		chainedHandler = g.middlewares[i](chainedHandler)
	}

	chainedHandler.ServeHTTP(w, r)
}

// reverseProxy 是核心的反向代理处理器
func (g *Gateway) reverseProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	rule := g.matchRoute(r.URL.Path)
	var service, path string

	if rule != nil {
		service, path = g.applyRouteRule(r, rule)
	} else {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		service = parts[1]
		path = "/" + strings.Join(parts[2:], "/")
	}

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

	upstreamURL := "http://" + target + path
	log.Printf("Forwarding => %s (service: %s)", upstreamURL, service)

	proxy, err := g.proxyPool.GetProxy("http://" + target)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadGateway)
		return
	}

	proxy.Transport = g.httpClient.Transport
	proxy.FlushInterval = time.Millisecond * 100

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		url, _ := url.Parse("http://" + target)
		req.Host = url.Host
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
		if rule != nil {
			g.modifyRequest(req, rule)
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if rule != nil {
			g.modifyResponse(resp, rule)
		}
		return nil
	}

	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = io.ReadAll(r.Body)
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	breaker := g.GetCircuitBreaker(service)

	reqFunc := func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		outReq := r.Clone(ctx)
		outReq.URL, _ = url.Parse(upstreamURL)
		outReq.Host = target
		outReq.RequestURI = ""

		if rule != nil {
			g.modifyRequest(outReq, rule)
		}

		return g.httpClient.Do(outReq)
	}

	result, err := breaker.Do(context.Background(), reqFunc)
	if err != nil {
		requestDuration := time.Since(startTime)
		log.Printf("Circuit open or upstream failed for service=[%s], err=%v, duration=%v\n", service, err, requestDuration)
		http.Error(w, "service unavailable (circuit open or upstream error)", http.StatusServiceUnavailable)
		return
	}

	resp := result.(*http.Response)
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	requestDuration := time.Since(startTime)
	log.Printf("Request completed for service=[%s], status=%d, duration=%v\n", service, resp.StatusCode, requestDuration)
}

// ... (其他方法保持不变) ...
// UseWeightedLoadBalancer, AddRouteRule, RemoveRouteRule, GetRouteRules, SetCircuitBreakerConfig,
// matchRoute, applyRouteRule, modifyRequest, modifyResponse, GetCircuitBreaker, SetRateLimitConfig,
// GetRateLimiter, checkRateLimit, LoadConfig, UpdateConfig, EnhancedHealthCheck

// UseWeightedLoadBalancer 启用权重负载均衡
func (g *Gateway) UseWeightedLoadBalancer(use bool) {
	g.useWeighted = use
}

// AddRouteRule 添加路由规则
func (g *Gateway) AddRouteRule(rule *model.RouteRule) {
	g.routeMutex.Lock()
	defer g.routeMutex.Unlock()
	g.routeRules = append(g.routeRules, rule)
}

// RemoveRouteRule 移除路由规则
func (g *Gateway) RemoveRouteRule(serviceName string) {
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
func (g *Gateway) GetRouteRules() []*model.RouteRule {
	g.routeMutex.RLock()
	defer g.routeMutex.RUnlock()

	// 返回副本以避免外部修改
	rules := make([]*model.RouteRule, len(g.routeRules))
	copy(rules, g.routeRules)
	return rules
}

// SetCircuitBreakerConfig 设置熔断器配置
func (g *Gateway) SetCircuitBreakerConfig(serviceName string, config *CircuitBreakerConfig) {
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

// matchRoute 匹配路由规则
func (g *Gateway) matchRoute(path string) *model.RouteRule {
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
func (g *Gateway) applyRouteRule(req *http.Request, rule *model.RouteRule) (string, string) {
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
		// 编译正则表达式
		if re, err := regexp.Compile(rule.PathRewrite); err == nil {
			path = re.ReplaceAllString(path, rule.RewriteTarget)
		}
	}

	return service, path
}

// modifyRequest 修改请求
func (g *Gateway) modifyRequest(req *http.Request, rule *model.RouteRule) {
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
func (g *Gateway) modifyResponse(resp *http.Response, rule *model.RouteRule) {
	// 这里可以添加响应修改逻辑
	// 例如添加响应头、修改状态码等
}

// GetCircuitBreaker 获取服务对应的熔断器
func (g *Gateway) GetCircuitBreaker(serviceName string) *circuitbreaker.CircuitBreaker {
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

	// 创建熔断器配置选项
	opts := []circuitbreaker.BreakerOption{
		circuitbreaker.WithOnStateChangeHookFn(func(from, to circuitbreaker.State) {
			log.Printf("[CB][%s] 状态变更: %s -> %s\n", serviceName, from, to)
		}),
		circuitbreaker.WithCounterResetInterval(config.CounterResetInterval),
		circuitbreaker.WithOpenTimeout(60 * time.Second),
		circuitbreaker.WithHalfOpenMaxSuccesses(config.HalfOpenMaxSuccesses),
		circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(config.FailureRateWindow, config.FailureRateThreshold)),
	}

	// 创建熔断器
	cb = circuitbreaker.New(opts...)

	// 存储熔断器
	g.cbMutex.Lock()
	g.circuitBreakers[serviceName] = cb
	g.cbMutex.Unlock()

	return cb
}

// SetRateLimitConfig 设置限流配置
func (g *Gateway) SetRateLimitConfig(config *RateLimitConfig) {
	g.rlConfigMutex.Lock()
	defer g.rlConfigMutex.Unlock()
	g.rlConfig = config
}

// GetRateLimiter 获取限流器
func (g *Gateway) GetRateLimiter(key string, rateLimit, burst int) *rate.Limiter {
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

// checkRateLimit 检查限流
func (g *Gateway) checkRateLimit(req *http.Request) bool {
	// 获取配置
	g.rlConfigMutex.RLock()
	rlConfig := g.rlConfig
	g.rlConfigMutex.RUnlock()

	if rlConfig == nil {
		return true // 没有限流配置，默认允许
	}

	ip := req.RemoteAddr
	ua := req.UserAgent()

	// IP限流
	for cidr, limit := range rlConfig.IPLimits {
		if strings.HasPrefix(ip, cidr) {
			limiter := g.GetRateLimiter("ip:"+cidr, int(limit.Rate), limit.Burst)
			if !limiter.Allow() {
				return false
			}
			break
		}
	}

	// UA限流
	for pattern, limit := range rlConfig.UALimits {
		if strings.Contains(ua, pattern) {
			limiter := g.GetRateLimiter("ua:"+pattern, int(limit.Rate), limit.Burst)
			if !limiter.Allow() {
				return false
			}
			break
		}
	}

	// 默认限流
	if rlConfig.DefaultRate > 0 && rlConfig.DefaultBurst > 0 {
		defaultLimiter := g.GetRateLimiter("default", int(rlConfig.DefaultRate), rlConfig.DefaultBurst)
		return defaultLimiter.Allow()
	}

	return true
}

// LoadConfig 从应用配置加载网关配置
func (g *Gateway) LoadConfig(path string) {
	cfg, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("[GW] 读取配置文件失败: %v\n", err)
	}
	var config *model.GatewayConfig
	if err := yaml.Unmarshal(cfg, &config); err != nil {
		log.Fatalf("[GW] 解析配置文件失败: %v\n", err)
	}
	g.UpdateConfig(config)
}

// UpdateConfig 更新配置
func (g *Gateway) UpdateConfig(config *model.GatewayConfig) {
	g.configMutex.Lock()
	g.config = config
	g.configMutex.Unlock()

	// 更新路由规则
	if config != nil {
		g.routeMutex.Lock()
		g.routeRules = config.RouteRules
		g.routeMutex.Unlock()

		// 更新熔断器配置
		if config.CircuitBreaker != nil {
			for serviceName, cbConfig := range config.CircuitBreaker.Services {
				g.SetCircuitBreakerConfig(serviceName, &CircuitBreakerConfig{
					CounterResetInterval: time.Duration(cbConfig.CounterResetInterval) * time.Second,
					HalfOpenMaxSuccesses: cbConfig.HalfOpenMaxSuccesses,
					FailureRateWindow:    cbConfig.FailureRateWindow,
					FailureRateThreshold: cbConfig.FailureRateThreshold,
				})
			}
		}

		// 更新限流配置
		if config.RateLimit != nil {
			rlConfig := &RateLimitConfig{
				DefaultRate:  config.RateLimit.DefaultRate,
				DefaultBurst: config.RateLimit.DefaultBurst,
				IPLimits: make(map[string]*struct {
					Rate  float64
					Burst int
				}),
				UALimits: make(map[string]*struct {
					Rate  float64
					Burst int
				}),
			}

			// IP限制
			for _, ipRule := range config.RateLimit.IPLimits {
				rlConfig.IPLimits[ipRule.CIDR] = &struct {
					Rate  float64
					Burst int
				}{
					Rate:  ipRule.Rate,
					Burst: ipRule.Burst,
				}
			}

			// UA限制
			for _, uaRule := range config.RateLimit.UALimits {
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

		// 更新认证配置
		if config.Auth != nil {
			g.clientID = config.Auth.ClientID
			g.clientSecret = config.Auth.ClientSecret
		}
	}
}

// EnhancedHealthCheck 增强的健康检查
// 不仅检查网关本身，还可以检查后端服务的健康状况
func (g *Gateway) EnhancedHealthCheck(w http.ResponseWriter, r *http.Request) {
	// 简单的健康检查
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		return
	}

	// 更详细的健康检查，包括后端服务状态
	if r.URL.Path == "/health/detail" {
		status := make(map[string]interface{})
		status["gateway"] = "OK"

		// 可以在这里添加更多关于后端服务健康状态的检查
		// 例如检查各服务的连接数、错误率等

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// 这里应该将status序列化为JSON并写入响应
		// 为简洁起见，我们只返回简单的文本
		w.Write([]byte(`{"gateway": "OK"}`))
		return
	}
}
