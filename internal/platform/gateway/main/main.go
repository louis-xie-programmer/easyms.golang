// main.go API网关服务主程序
package main

import (
	"context"
	"easyms/internal/platform/gateway"
	"easyms/internal/platform/gateway/plugins"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/tracing" // 引入 tracing 包
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
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

	// 初始化应用配置存储
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		log.Fatalf("Failed to initialize app config store: %v", err)
	}

	// 加载配置
	var provider config.AppConfigProvider
	var cfgDiscovery *discovery.Discovery
	switch cfgStore.StoreType {
	case "consul":
		cd, err := discovery.NewDiscovery(cfgStore.Consul.Host)
		if err != nil {
			log.Fatalf("Failed to create config discovery client: %v", err)
		}
		cfgDiscovery = cd
		provider = config.NewConsulConfig(cfgDiscovery, serverName, cfgStore.Consul.KeyPath, cfgStore.Env)
	case "local":
		provider = config.NewLocalConfig(serverName, cfgStore.Env)
	default:
		log.Fatalf("Unsupported config store type: %s", cfgStore.StoreType)
	}
	if err := provider.LoadAppConfig(); err != nil {
		log.Fatalf("Failed to load application config: %v", err)
	}

	appConfig := config.GetAppConfig()
	if appConfig == nil {
		log.Fatalf("Failed to get application config")
	}

	if appConfig.Gateway == nil {
		log.Fatalf("Gateway configuration ('gateway:' section) not found in config files")
	}
	gatewayConfig := appConfig.Gateway

	// 初始化日志系统
	logger.Init(serverName, appConfig)

	// --- 初始化分布式追踪 (Tracing) ---
	if appConfig.Tracing.Enable {
		shutdown, err := tracing.InitTracerProvider(serverName, appConfig.Tracing.Endpoint)
		if err != nil {
			logger.Error(err, "Failed to initialize tracer provider", serverName)
		} else {
			// 确保在程序退出时刷新所有 Span
			defer func() {
				if err := shutdown(context.Background()); err != nil {
					logger.Error(err, "Failed to shutdown tracer provider", serverName)
				}
			}()
			logger.Info("Tracing enabled", "endpoint", appConfig.Tracing.Endpoint)
		}
	}

	// 初始化 Redis 客户端 (用于分布式限流等)
	var redisClient *db.EasyRedis
	if appConfig.Cache.Redis.Address != "" {
		redisClient, err = db.NewEasyRedis(
			&appConfig.Cache.Redis.Address,
			&appConfig.Cache.Redis.Password,
			&appConfig.Cache.Redis.DB,
		)
		if err != nil {
			logger.Error(err, "Failed to initialize Redis client", serverName)
			// 我们可以选择在这里 panic，或者允许网关在没有 Redis 的情况下运行（限流将失效或降级）
			// 这里选择记录错误，让限流插件处理 nil client
		} else {
			logger.Info("Redis client initialized", "address", appConfig.Cache.Redis.Address)
			// 确保在程序退出时关闭 Redis 连接
			defer func() {
				if err := redisClient.Close(); err != nil {
					logger.Error(err, "Failed to close Redis client", serverName)
				}
			}()
		}
	} else {
		logger.Warn("Redis configuration missing, distributed features will be disabled", serverName)
	}

	// 初始化服务发现
	sd, err := discovery.NewServiceDiscovery(appConfig.Consul.Host)
	if err != nil {
		log.Fatalf("Failed to create service discovery client: %v", err)
	}

	// 动态监听服务
	serviceSet := make(map[string]struct{})
	for _, route := range gatewayConfig.RouteRules {
		if _, exists := serviceSet[route.ServiceName]; !exists {
			logger.Info("Watching service", "target_service", route.ServiceName)
			sd.WatchService(route.ServiceName)
			serviceSet[route.ServiceName] = struct{}{}
		}
	}
	// Ensure auth service is also watched
	if _, exists := serviceSet["auth-svc"]; !exists {
		logger.Info("Watching service", "target_service", "auth-svc")
		sd.WatchService("auth-svc")
		serviceSet["auth-svc"] = struct{}{}
	}

	// --- 2. 创建插件化网关 ---
	gw := gateway.NewGateway()

	// --- 3. 根据配置初始化并注册所有插件 ---

	// Tracing 插件 (如果启用)
	var tracingPlugin *plugins.TracingPlugin
	if appConfig.Tracing.Enable {
		tracingPlugin = plugins.NewTracingPlugin(serverName)
	}

	// Auth 插件
	var authPlugin *plugins.AuthPlugin
	if gatewayConfig.Auth != nil && gatewayConfig.Auth.Enable {
		// 使用指数退避策略连接 Auth 服务
		var conn *grpc.ClientConn
		var authServiceAddr string

		// 指数退避参数
		backoff := 1 * time.Second
		maxBackoff := 30 * time.Second
		maxRetries := 10 // 增加重试次数，或者可以基于时间判断

		for i := 0; i < maxRetries; i++ {
			authServiceAddr = sd.GetService("auth-svc")
			if authServiceAddr != "" {
				// 尝试建立 gRPC 连接
				// 使用 WithBlock 确保连接成功，设置超时防止永久阻塞
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

				// 配置 Keepalive 参数以保持连接健康
				kacp := keepalive.ClientParameters{
					Time:                2 * time.Minute,  // send pings when idle
					Timeout:             10 * time.Second, // wait for ping ack
					PermitWithoutStream: false,            // avoid too many pings
				}

				c, err := grpc.DialContext(ctx, authServiceAddr,
					grpc.WithTransportCredentials(insecure.NewCredentials()),
					grpc.WithBlock(),
					grpc.WithKeepaliveParams(kacp),
				)
				cancel()

				if err == nil {
					conn = c
					logger.Info("Connected to auth service", "address", authServiceAddr)
					break
				}
				logger.Warn("Failed to connect to auth service, retrying...", "address", authServiceAddr, "error", err)
			} else {
				logger.Warn("Auth service address not found in discovery, retrying...", "attempt", i+1)
			}

			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		if conn == nil {
			logger.Error(errors.New("failed to connect to auth service after retries"), "Auth plugin disabled", serverName)
		} else {
			// 注意：这里不应该 defer conn.Close()，因为插件需要在整个生命周期内使用连接
			// 如果需要优雅关闭，应该在 main 函数结束时处理

			authConfig := plugins.AuthConfig{
				Scheme:          gatewayConfig.Auth.Scheme,
				CacheExpiration: time.Duration(gatewayConfig.Auth.CacheExpirationSeconds) * time.Second,
				SkipPaths:       gatewayConfig.Auth.SkipPaths,
			}
			authPlugin = plugins.NewAuthPlugin(conn, authConfig)
			logger.Info("Auth plugin enabled", "status", "initialized")
			defer func() {
				_ = conn.Close()
			}()
		}
	}

	metricsSampleRate := 0.1
	upstreamSampleRate := 1.0
	if gatewayConfig.Metrics != nil {
		metricsSampleRate = gatewayConfig.Metrics.SampleRate
		if gatewayConfig.Metrics.UpstreamSampleRate != nil {
			upstreamSampleRate = *gatewayConfig.Metrics.UpstreamSampleRate
		}
	}
	metricsPlugin := plugins.NewMetricsPlugin(metricsSampleRate)
	proxyPlugin := plugins.NewProxyPlugin(gatewayConfig.Proxy)
	proxyPlugin.UpdateUpstreamSampleRate(upstreamSampleRate)
	// Config hot-reload (Consul)
	var cfgWatcher config.ConfigWatcherInterface
	if cfgStore.StoreType == "consul" && cfgStore.Consul.ReloadOnChanges && cfgDiscovery != nil {
		cfgWatcher = config.NewConfigWatcher(cfgDiscovery, cfgStore.Consul.KeyPath, serverName, cfgStore.Env, func(cfg *models.AppConfig) {
			config.SetAppConfig(cfg)
			if cfg != nil && cfg.Gateway != nil && cfg.Gateway.Metrics != nil {
				metricsPlugin.UpdateSampleRate(cfg.Gateway.Metrics.SampleRate)
				if cfg.Gateway.Metrics.UpstreamSampleRate != nil {
					proxyPlugin.UpdateUpstreamSampleRate(*cfg.Gateway.Metrics.UpstreamSampleRate)
				}
			}
		})
		cfgWatcher.Start()
		defer cfgWatcher.Stop()
	}

	if lc, ok := provider.(*config.LocalConfig); ok {
		lc.SetOnChangeCallback(func(cfg *models.AppConfig) {
			if cfg != nil && cfg.Gateway != nil && cfg.Gateway.Metrics != nil {
				metricsPlugin.UpdateSampleRate(cfg.Gateway.Metrics.SampleRate)
				if cfg.Gateway.Metrics.UpstreamSampleRate != nil {
					proxyPlugin.UpdateUpstreamSampleRate(*cfg.Gateway.Metrics.UpstreamSampleRate)
				}
			}
		})
	}

	mockPlugin := plugins.NewMockPlugin()
	rateLimitPlugin := plugins.NewRateLimitPlugin(gatewayConfig.RateLimit, redisClient)
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

	// 注册插件 (注意顺序)
	if tracingPlugin != nil {
		gw.AddPlugin(tracingPlugin) // Tracing 最先执行
	}
	if authPlugin != nil {
		gw.AddPlugin(authPlugin)
	}
	gw.AddPlugin(
		metricsPlugin,
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
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("Starting plugin-based gateway server", "port", port)
		serverErrors <- server.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error(err, "Server failed to start or encountered a runtime error", serverName)
		}
	case sig := <-shutdown:
		logger.Info("Shutdown signal received", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Error(err, "Graceful shutdown failed", serverName)
		} else {
			logger.Info("Server shutdown gracefully", serverName)
		}
	}
}
