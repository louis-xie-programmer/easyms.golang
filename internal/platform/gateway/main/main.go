// main.go API网关服务主程序
package main

import (
	"easyms/internal/platform/gateway"
	"easyms/internal/platform/gateway/plugins"
	"easyms/internal/shared/config"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"fmt"
	"log"
	"net/http"
)

func main() {
	// --- 1. 初始化核心依赖 ---
	serverName := "gateway"
	port := 10000

	// 初始化应用配置存储
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		log.Fatalf("Failed to initialize app config store: %v", err)
	}

	// 初始化服务发现
	sd, err := discovery.NewServiceDiscovery(cfgStore.Consul.Host)
	if err != nil {
		log.Fatalf("Failed to create service discovery client: %v", err)
	}
	sd.WatchService("user-svc")
	sd.WatchService("auth-svc")
	sd.WatchService("order-svc")

	// 加载应用配置 (简化版，实际应处理本地和远程)
	provider := config.NewLocalConfig(serverName, cfgStore.Env)
	if err := provider.LoadAppConfig(); err != nil {
		log.Fatalf("Failed to load app config: %v", err)
	}
	appConfig := config.GetAppConfig()

	// 初始化日志系统
	logger.Init(serverName, appConfig)

	// --- 2. 创建插件化网关 ---
	gw := gateway.NewGateway()

	// --- 3. 初始化并注册所有插件 ---

	// Mock Plugin (新添加)
	mockPlugin := plugins.NewMockPlugin()

	// Auth Plugin
	// 注意：这里的地址应该是通过服务发现动态获取的
	//authSvcAddr := "localhost:10001" // 暂时硬编码
	//authConn, err := grpc.Dial(authSvcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	//if err != nil {
	//	logger.Error(err, "Failed to connect to auth-svc via gRPC", serverName)
	//}
	//authPlugin := plugins.NewAuthPlugin(authConn)

	// RateLimit Plugin (使用默认配置)
	rateLimitPlugin := plugins.NewRateLimitPlugin(nil)

	// Routing Plugin (可以从配置文件加载路由规则)
	routes := []plugins.Route{
		{PathPrefix: "/api/orders", ServiceName: "order-svc", StripPrefix: true},
		{PathPrefix: "/api/users", ServiceName: "user-svc", StripPrefix: true},
	}
	routingPlugin := plugins.NewRoutingPlugin(sd, routes)

	// CircuitBreaker Plugin
	circuitBreakerPlugin := plugins.NewCircuitBreakerPlugin()

	// Proxy Plugin (链的末端)
	proxyPlugin := plugins.NewProxyPlugin()

	// 注册所有插件
	gw.AddPlugin(
		mockPlugin, // 注册 Mock 插件
		//authPlugin,
		rateLimitPlugin,
		routingPlugin,
		circuitBreakerPlugin,
		proxyPlugin,
	)

	// --- 4. 启动网关服务 ---
	mux := http.NewServeMux()
	mux.Handle("/", gw)
	mux.HandleFunc("/health", gw.HealthCheck)

	logger.Info("Starting plugin-based gateway server", serverName, "port", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux); err != nil {
		logger.Error(err, "Failed to start gateway server", serverName)
	}
}
