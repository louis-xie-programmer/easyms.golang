// main.go API网关服务主程序
package main

import (
	"context"
	"easyms/internal/platform/gateway"
	"easyms/internal/platform/gateway/plugins"
	"easyms/internal/shared/config"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// --- 1. 初始化核心依赖 ---
	serverName := "gateway"
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev" // Fallback to "dev" if not set
	}

	// 初始化应用配置存储 (e.g., read configs/app.yaml to find Consul)
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		log.Fatalf("Failed to initialize app config store: %v", err)
	}

	// 根据存储类型 (local/consul) 加载配置
	provider := config.NewLocalConfig(serverName, cfgStore.Env)
	if err := provider.LoadAppConfig(); err != nil {
		log.Fatalf("Failed to load application config: %v", err)
	}

	// 获取加载并合并后的全局配置
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		log.Fatalf("Failed to get application config")
	}

	// 检查网关特定配置是否存在
	if appConfig.Gateway == nil {
		log.Fatalf("Gateway configuration ('gateway:' section) not found in config files")
	}
	gatewayConfig := appConfig.Gateway

	// 初始化日志系统 (使用共享配置)
	logger.Init(serverName, appConfig)

	// 初始化服务发现 (使用共享配置)
	sd, err := discovery.NewServiceDiscovery(appConfig.Consul.Host)
	if err != nil {
		log.Fatalf("Failed to create service discovery client: %v", err)
	}

	// 根据路由规则动态监听所有需要的服务
	serviceSet := make(map[string]struct{})
	for _, route := range gatewayConfig.RouteRules {
		if _, exists := serviceSet[route.ServiceName]; !exists {
			logger.Info("Watching service", "service", route.ServiceName)
			sd.WatchService(route.ServiceName)
			serviceSet[route.ServiceName] = struct{}{}
		}
	}

	// --- 2. 创建插件化网关 ---
	gw := gateway.NewGateway()

	// --- 3. 根据配置初始化并注册所有插件 ---
	metricsPlugin := plugins.NewMetricsPlugin() // 新增 Metrics 插件
	mockPlugin := plugins.NewMockPlugin()
	rateLimitPlugin := plugins.NewRateLimitPlugin(gatewayConfig.RateLimit)
	circuitBreakerPlugin := plugins.NewCircuitBreakerPlugin(gatewayConfig.CircuitBreaker)

	routes := make([]plugins.Route, len(gatewayConfig.RouteRules))
	for i, r := range gatewayConfig.RouteRules {
		routes[i] = plugins.Route{
			PathPrefix:  r.PathPrefix,
			ServiceName: r.ServiceName,
			StripPrefix: r.StripPrefix,
		}
	}
	routingPlugin := plugins.NewRoutingPlugin(sd, routes)
	proxyPlugin := plugins.NewProxyPlugin(gatewayConfig.Proxy)

	gw.AddPlugin(
		metricsPlugin, // 必须最先注册，以测量整个请求链路
		mockPlugin,
		rateLimitPlugin,
		routingPlugin,
		circuitBreakerPlugin,
		proxyPlugin,
	)

	// --- 4. 启动辅助服务 (Metrics) ---
	go startMetricsServer(9090)

	// --- 5. 启动并管理网关服务生命周期 ---
	startServer(gw, appConfig, serverName)
}

func startMetricsServer(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	addr := fmt.Sprintf(":%d", port)
	logger.Info("Starting metrics server", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error(err, "Failed to start metrics server", "metrics")
	}
}

func startServer(gw *gateway.Gateway, appConfig *models.AppConfig, serverName string) {
	port := appConfig.Server.Port
	if port == 0 {
		port = 10000 // Default port
	}

	mux := http.NewServeMux()
	mux.Handle("/", gw)
	mux.HandleFunc("/health", gw.HealthCheck)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// 创建一个通道来接收优雅停机的错误
	serverErrors := make(chan error, 1)

	// 在一个单独的 goroutine 中启动服务
	go func() {
		logger.Info("Starting plugin-based gateway server", "port", port)
		serverErrors <- server.ListenAndServe()
	}()

	// 创建一个通道来监听操作系统的信号
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞主 goroutine，直到接收到错误或停机信号
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error(err, "Server failed to start or encountered a runtime error", serverName)
		}
	case sig := <-shutdown:
		logger.Info("Shutdown signal received", "signal", sig)

		// 创建一个有超时期限的上下文，用于通知服务器在指定时间内完成现有请求
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 调用 Shutdown() 来优雅地关闭服务器
		if err := server.Shutdown(ctx); err != nil {
			logger.Error(err, "Graceful shutdown failed", serverName)
		} else {
			logger.Info("Server shutdown gracefully", serverName)
		}
	}
}
