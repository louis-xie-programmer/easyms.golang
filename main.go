package main

import (
	"context"
	"easyms/pkg/config"
	"easyms/pkg/logger"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/mercari/go-circuitbreaker"

	"easyms/middleware"
	stdlog "log"

	"github.com/gin-gonic/gin"
	"github.com/go-kit/kit/endpoint"
	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/sd"
	"github.com/go-kit/kit/sd/consul"
	"github.com/go-kit/kit/sd/lb"
)

// JWT认证中间件

func main() {
	// 获取环境变量
	env := "prod"

	err := config.LoadServiceConfig("gateway", env)
	if err != nil {
		logger.Error(err, "Failed to load service config", "gateway", nil)
		//log.Fatalf("[FATAL] Failed to load service config: %v", err)
	}

	appConfig := config.GetAppConfig()
	logger.Init("gateway", appConfig)

	// 初始化 Consul 客户端（统一配置）
	consulClient, err := config.CreateConsulClient()
	if err != nil {
		logger.Error(err, "Consul client error", "gateway", nil)
		return
	}
	client := consul.NewClient(consulClient)

	// 服务发现（支持多服务）
	services := []string{"server1", "server2"}
	instancers := make(map[string]sd.Instancer)
	balancers := make(map[string]lb.Balancer)
	kitLogger := kitlog.NewLogfmtLogger(kitlog.NewSyncWriter(stdlog.Writer()))
	// 按实例缓存熔断器（每个后端独立熔断），使用 sync.Map 简化并发
	var instanceCBs sync.Map
	getCBForInstance := func(instance string) *circuitbreaker.CircuitBreaker {
		if cb, ok := instanceCBs.Load(instance); ok {
			return cb.(*circuitbreaker.CircuitBreaker)
		}
		// 可根据服务名或实例名自定义熔断参数
		cb := middleware.NewCircuitBreakerWithConfig(middleware.CircuitBreakerConfig{
			CounterResetInterval: 10 * time.Second,
			HalfOpenMaxSuccesses: 4,
			FailureRateWindow:    10,
			FailureRateThreshold: 0.4,
			Name:                 instance,
		})
		actual, _ := instanceCBs.LoadOrStore(instance, cb)
		return actual.(*circuitbreaker.CircuitBreaker)
	}

	for _, svc := range services {
		inst := consul.NewInstancer(client, kitLogger, svc, nil, true)
		factory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
			cb := getCBForInstance(instance)

			ep := func(ctx context.Context, request interface{}) (response interface{}, err error) {
				var targetUrl string
				if !startsWithHttp(instance) {
					targetUrl = "http://" + instance
				} else {
					targetUrl = instance
				}
				path := "/"
				if ginCtx, ok := ctx.Value("ginContext").(*gin.Context); ok {
					if a := ginCtx.Param("action"); a != "" {
						path = a
					}
				}
				targetUrl += path

				req, err := http.NewRequestWithContext(ctx, "GET", targetUrl, nil)
				if err != nil {
					return nil, err
				}
				client := &http.Client{Timeout: 5 * time.Second}
				// --- 熔断器 Ready 检查 ---
				if !cb.Ready() {
					return nil, fmt.Errorf("circuit breaker open for %s", instance)
				}
				defer func() { err = cb.Done(ctx, err) }()

				resp, err := client.Do(req)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				if resp.StatusCode >= 400 {
					return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
				}
				return resp.StatusCode, nil
			}
			return ep, nil, nil
		}
		endpointer := sd.NewEndpointer(inst, factory, kitLogger)
		balancers[svc] = lb.NewRoundRobin(endpointer)
		instancers[svc] = inst
	}

	// Gin 路由
	r := gin.Default()
	//r.Use(middleware.AuthMiddleware())

	// === 限流器集成（基于 appConfig 实时同步） ===
	limiterManager, _ := middleware.NewLimiterManager(&middleware.ConsulRateLimitConfig{}, 100, 200)
	limiterManager.SyncFromAppConfig() // 启动时同步一次
	r.Use(limiterManager.Middleware())

	// 启动后台 goroutine 定时同步配置（如有 Consul 热更新，建议10s~30s同步一次）
	go func() {
		for {
			limiterManager.SyncFromAppConfig()
			time.Sleep(10 * time.Second)
		}
	}()

	// API 路由转发（多服务）
	r.Any("/api/:service/*action", func(c *gin.Context) {
		service := c.Param("service")
		balancer, ok := balancers[service]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
			return
		}
		endpoint, err := balancer.Endpoint()
		if err != nil {
			logger.Error(err, "No backend available", "gateway", nil)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No backend available"})
			return
		}
		// 传递 gin.Context 作为 context value，便于下游 endpoint 获取 action
		ctx := context.WithValue(c.Request.Context(), "ginContext", c)
		result, err := endpoint(ctx, nil)
		if err != nil {
			logger.Error(err, "Service unavailable", "gateway", nil)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service unavailable"})
			return
		}
		logger.Info(fmt.Sprintf("API gateway proxy: %s result: %v", service, result), "gateway", nil)
		c.JSON(http.StatusOK, gin.H{"result": result})
	})

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 启动网关，端口可从配置读取
	port := 10001
	logger.Warn(fmt.Sprintf("Gateway started on port %d", port), "gateway", nil)
	err = r.Run(fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Error(err, "Gin gateway error", "gateway", nil)
	}
	logger.Shutdown()
}

func startsWithHttp(addr string) bool {
	return len(addr) >= 4 && (addr[:4] == "http" || (len(addr) >= 5 && addr[:5] == "https"))
}
