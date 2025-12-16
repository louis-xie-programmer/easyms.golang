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
	"gopkg.in/yaml.v2"
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

	"github.com/mercari/go-circuitbreaker"
	"golang.org/x/time/rate"

	"easyms/internal/shared/discovery"
)

// ReverseProxyPool 反向代理池，用于缓存和复用ReverseProxy实例
type ReverseProxyPool struct {
	proxies map[string]*httputil.ReverseProxy
	mutex   sync.RWMutex
}

// GetProxy 获取或创建指定目标的反向代理
func (p *ReverseProxyPool) GetProxy(target string) (*httputil.ReverseProxy, error) {
	// 先尝试读锁获取已存在的代理
	p.mutex.RLock()
	proxy, exists := p.proxies[target]
	p.mutex.RUnlock()

	if exists {
		return proxy, nil
	}

	// 如果不存在，创建新的代理
	url, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy = httputil.NewSingleHostReverseProxy(url)

	// 修改 Director 修正 Host 和 URL
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// 设置正确的 Host，避免回流到网关
		req.Host = url.Host
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
	}

	// 写锁保存代理实例
	p.mutex.Lock()
	p.proxies[target] = proxy
	p.mutex.Unlock()

	return proxy, nil
}

// Gateway API网关结构体
// 包含服务发现客户端，用于获取后端服务实例
// 并实现负载均衡和熔断功能
type Gateway struct {
	sd              *discovery.ServiceDiscovery               // 服务发现客户端
	proxyPool       *ReverseProxyPool                         // 反向代理池
	httpClient      *http.Client                              // 专用HTTP客户端，带有连接池和超时设置
	useWeighted     bool                                      // 是否使用权重负载均衡
	routeRules      []*model.RouteRule                        // 路由规则
	routeMutex      sync.RWMutex                              // 路由规则读写锁
	circuitBreakers map[string]*circuitbreaker.CircuitBreaker // 熔断器映射
	cbMutex         sync.RWMutex                              // 熔断器读写锁
	rateLimiters    map[string]*rate.Limiter                  // 限流器映射
	rlMutex         sync.RWMutex                              // 限流器读写锁
	config          *model.GatewayConfig                      // 当前配置
	configMutex     sync.RWMutex                              // 配置读写锁
}

// NewGateway 创建新的API网关实例
// 通过传入的服务发现客户端初始化网关
// 参数:
//   - sd: 服务发现客户端
//
// 返回值:
//   - *Gateway: API网关实例
func NewGateway(sd *discovery.ServiceDiscovery) *Gateway {
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

	return &Gateway{
		sd: sd,
		proxyPool: &ReverseProxyPool{
			proxies: make(map[string]*httputil.ReverseProxy),
		},
		httpClient:      httpClient,
		useWeighted:     false,
		routeRules:      make([]*model.RouteRule, 0),
		circuitBreakers: make(map[string]*circuitbreaker.CircuitBreaker),
		rateLimiters:    make(map[string]*rate.Limiter),
	}
}

// UseWeightedLoadBalancer 启用权重负载均衡
func (g *Gateway) UseWeightedLoadBalancer(use bool) {
	g.useWeighted = use
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

// GetCircuitBreaker 获取服务对应的熔断器
func (g *Gateway) GetCircuitBreaker(serviceName string) *circuitbreaker.CircuitBreaker {
	g.cbMutex.RLock()
	cb, exists := g.circuitBreakers[serviceName]
	g.cbMutex.RUnlock()

	if exists {
		return cb
	}

	// 获取配置
	g.configMutex.RLock()
	var config *model.CircuitBreakerServiceConfig
	if g.config != nil && g.config.CircuitBreaker != nil {
		if serviceConfig, ok := g.config.CircuitBreaker.Services[serviceName]; ok {
			config = serviceConfig
		}
	}
	g.configMutex.RUnlock()

	// 创建熔断器配置选项
	opts := []circuitbreaker.BreakerOption{
		circuitbreaker.WithOnStateChangeHookFn(func(from, to circuitbreaker.State) {
			log.Printf("[CB][%s] 状态变更: %s -> %s\n", serviceName, from, to)
		}),
		circuitbreaker.WithCounterResetInterval(10 * time.Second),
		circuitbreaker.WithOpenTimeout(60 * time.Second),
		circuitbreaker.WithHalfOpenMaxSuccesses(4),
		circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(10, 0.4)),
	}

	// 应用自定义配置
	if config != nil {
		opts = append(opts,
			circuitbreaker.WithCounterResetInterval(time.Duration(config.CounterResetInterval)*time.Second),
			circuitbreaker.WithHalfOpenMaxSuccesses(config.HalfOpenMaxSuccesses),
			circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(
				config.FailureRateWindow,
				config.FailureRateThreshold,
			)))
	}

	// 创建熔断器
	cb = circuitbreaker.New(opts...)

	// 存储熔断器
	g.cbMutex.Lock()
	g.circuitBreakers[serviceName] = cb
	g.cbMutex.Unlock()

	return cb
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
	g.configMutex.RLock()
	rlConfig := g.config.RateLimit
	g.configMutex.RUnlock()

	if rlConfig == nil {
		return true // 没有限流配置，默认允许
	}

	ip := req.RemoteAddr
	ua := req.UserAgent()

	// IP限流
	for _, rule := range rlConfig.IPLimits {
		if rule.Net != nil && rule.Net.Contains(net.ParseIP(strings.Split(ip, ":")[0])) {
			limiter := g.GetRateLimiter("ip:"+rule.CIDR, int(rule.Rate), rule.Burst)
			if !limiter.Allow() {
				return false
			}
			break
		}
	}

	// UA限流
	for _, rule := range rlConfig.UALimits {
		if rule.Regexp != nil && rule.Regexp.MatchString(ua) {
			limiter := g.GetRateLimiter("ua:"+rule.Pattern, int(rule.Rate), rule.Burst)
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

func (g *Gateway) LoadConfig(path string) {
	cfg, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("[GW] 读取配置文件失败: %v\n", err)
	}
	var config *model.GatewayConfig
	if err := yaml.Unmarshal(cfg, &config); err != nil {
		log.Fatalf("[GW] 解析配置文件失败: %v\n", err)
	}
	g.configMutex.Lock()
	g.config = config
	g.configMutex.Unlock()
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
	}
}

// ServeHTTP 实现HTTP处理器接口
// 处理所有进入网关的HTTP请求
// 实现反向代理、负载均衡和熔断保护功能
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		// 1. 解析URL路径，提取服务名称
		// 路径格式: /{service_name}/{real_path}
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		// 获取目标服务名称
		// 第一个部分是服务名称
		service = parts[1]

		// 构造上游服务的真实路径
		// 去掉服务名称部分，保留实际路径
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

	// 构造上游服务的完整URL
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

	// 2. 读取并重构请求体（避免二次读取内容为空）
	// 由于请求体只能读取一次，需要将其内容读取到内存中
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = io.ReadAll(r.Body)
	}
	// 重置请求体，允许后续读取
	// 使用NopCloser包装Reader以满足io.ReadCloser接口
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// 3. 获取对应服务的熔断器
	// 每个服务都有独立的熔断器，用于故障隔离
	breaker := g.GetCircuitBreaker(service)

	// 4. 将上游调用封装成函数，用于熔断器执行
	// 熔断器通过执行此函数来监控服务状态
	reqFunc := func() (interface{}, error) {
		// 设置3秒上游调用超时
		// 防止因某个服务响应慢而阻塞整个网关
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		// 构建新的上游请求（带请求体）
		// 使用原始请求的方法、URL和请求体
		req, err := http.NewRequestWithContext(ctx, r.Method, upstreamURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}

		// 复制请求头
		// 保留原始请求的所有头部信息
		req.Header = r.Header.Clone()

		// 应用路由规则中的请求修改
		if rule != nil {
			g.modifyRequest(req, rule)
		}

		// 执行上游请求
		// 使用优化的HTTP客户端发起请求
		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		return resp, nil
	}

	// 5. 通过熔断器执行请求
	// 如果服务处于熔断状态或请求失败，熔断器会返回错误
	result, err := breaker.Do(context.Background(), reqFunc)
	if err != nil {
		requestDuration := time.Since(startTime)
		log.Printf("Circuit open or upstream failed for service=[%s], err=%v, duration=%v\n", service, err, requestDuration)
		// 回退处理：可以自定义为缓存、默认值等
		// 当前实现为返回503错误
		http.Error(w, "service unavailable (circuit open or upstream error)", http.StatusServiceUnavailable)
		return
	}

	// 6. 成功处理 - 返回上游响应
	// 断言结果为HTTP响应类型
	resp := result.(*http.Response)
	defer resp.Body.Close()

	// 复制响应头
	// 将上游服务的响应头复制到网关响应中
	for k, v := range resp.Header {
		w.Header()[k] = v
	}

	// 设置响应状态码
	// 使用上游服务返回的状态码
	w.WriteHeader(resp.StatusCode)

	// 复制响应体
	// 将上游服务的响应体复制到网关响应中
	io.Copy(w, resp.Body)

	requestDuration := time.Since(startTime)
	log.Printf("Request completed for service=[%s], status=%d, duration=%v\n", service, resp.StatusCode, requestDuration)
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
