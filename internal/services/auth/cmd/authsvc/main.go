// internal/services/auth/main.go
package main

import (
	"context"
	pb "easyms/api/proto/auth"
	"easyms/internal/services/auth/internal/handles"
	"easyms/internal/services/auth/internal/middleware"
	"easyms/internal/services/auth/internal/service"
	"easyms/internal/services/auth/internal/storage"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"fmt"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// main 认证服务主函数
func main() {
	// 认证服务名称和端口
	serverName := "auth-svc"

	var err error

	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil || cfgStore == nil {
		// 在日志系统初始化前，只能用fmt
		fmt.Printf("Failed to initialize app config store: %v\n", err)
		panic(err)
	}
	// 按需初始化Discovery客户端
	// 添加检查确保cfgStore不为空再访问其属性
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		fmt.Printf("Failed to create consul client: %v\n", err)
		panic(err)
	}

	var provider config.AppConfigProvider

	// 加载应用配置
	if cfgStore.StoreType == "consul" {
		// 使用Consul配置提供者
		provider = config.NewConsulConfig(discoveryClient, serverName, cfgStore.Consul.KeyPath, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			fmt.Printf("Failed to load app config from consul: %v\n", err)
			panic(err)
		}
		// 动态监听配置文件并更新服务
		watch := config.NewConfigWatcher(discoveryClient, cfgStore.Consul.KeyPath, serverName, cfgStore.Env, provider.OnChange())
		go watch.Start()
	} else {
		// 使用本地配置提供者
		// 从本地配置文件加载配置
		provider = config.NewLocalConfig(serverName, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			fmt.Printf("Failed to load local app config: %v\n", err)
			panic(err)
		}
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		panic("Application config is not loaded")
	}

	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	if discoveryClient != nil {
		err = discoveryClient.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil)
		if err != nil {
			logger.Error(err, "Failed to register service with consul", serverName)
			panic(err)
		}

		// 延迟注销服务
		// 确保服务在退出时从Consul中注销
		defer discoveryClient.DeRegister(serverName)
	}

	// 检查appConfig是否为空
	appConfig = config.GetAppConfig()

	watch := config.NewConfigWatcher(discoveryClient, cfgStore.Consul.KeyPath, serverName, cfgStore.Env, provider.OnChange())
	go watch.Start() // 配置热更新机制

	// 初始化认证服务组件
	// 初始化各种认证服务相关的组件
	var tokenService service.TokenService
	var tokenGranter service.TokenGranter
	var tokenEnhancer storage.TokenEnhancer
	// var tokenStore storage.TokenStore
	var userDetailsService service.UserDetailsService
	var clientDetailsService service.ClientDetailsService

	// 初始化JWT令牌增强器
	if appConfig.OAuth2.JWTSecret == "" {
		panic("JWT secret is not configured")
	}
	tokenEnhancer = storage.NewJwtTokenEnhancer(appConfig.OAuth2.JWTSecret)

	// 初始化数据库连接
	// 根据配置连接到数据库
	// 添加重试机制以避免连接冲突
	dbase, err := db.NewEasyDatabaseWithPool(appConfig.Database.Type, fmt.Sprintf("%s://%s:%s@%s:%d/%s",
		appConfig.Database.Type,
		appConfig.Database.UserName,
		appConfig.Database.Password,
		appConfig.Database.Host,
		appConfig.Database.Port,
		appConfig.Database.Database), appConfig.Database)

	if err != nil {
		logger.Error(err, "Failed to connect to database", serverName)
		panic(err)
	}

	// 尝试连接Redis
	logger.Info(
		"Attempting to connect to Redis",
		serverName,
		"address", appConfig.Cache.Redis.Address,
		"db", appConfig.Cache.Redis.DB,
	)
	redisClient, err := db.NewEasyRedis(
		&appConfig.Cache.Redis.Address,
		&appConfig.Cache.Redis.Password,
		&appConfig.Cache.Redis.DB,
	)

	// 创建token存储，支持Redis降级到内存
	var tokenStore storage.TokenStore

	if err == nil {
		// Redis连接成功
		logger.Info("Successfully connected to Redis, using Redis for token store.", serverName)
		tokenStore = storage.NewJwtTokenStore(
			tokenEnhancer.(*storage.JwtTokenEnhancer),
			dbase,
			redisClient,
		)
	} else {
		// Redis连接失败 - 降级到仅使用数据库的模式
		logger.Warn(
			"Redis connection failed, falling back to DB-only token store",
			serverName,
			"error", err.Error(),
			"address", appConfig.Cache.Redis.Address,
		)
		tokenStore = storage.NewJwtTokenStore(tokenEnhancer.(*storage.JwtTokenEnhancer), dbase, nil)
	}
	tokenService = service.NewTokenService(tokenStore, tokenEnhancer)

	// 初始化用户详情服务
	userDetailsService = service.NewPostgresUserDetailsService(dbase)

	// 初始化客户端详情服务
	clientDetailsService = service.NewPostgresClientDetailsService(dbase)

	// 初始化令牌授予器
	tokenGranter = service.NewComposeTokenGranter(map[string]service.TokenGranter{
		"client_credentials": service.NewClientCredentialsTokenGranter("client_credentials", clientDetailsService, tokenService),
		"password":           service.NewUsernamePasswordTokenGranter("password", userDetailsService, tokenService),
		"refresh_token":      service.NewRefreshGranter("refresh_token", tokenService),
	})

	// 启动 gRPC 服务器 (在一个新的 goroutine 中)
	grpcPort := appConfig.Server.Port + 10000
	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
		if err != nil {
			logger.Error(err, "Failed to listen for gRPC", serverName)
			panic(err)
		}

		s := grpc.NewServer()
		// 创建并注册 gRPC 服务实现
		pb.RegisterAuthServiceServer(s, service.NewGrpcServer(tokenGranter, tokenService, clientDetailsService))

		logger.Info("gRPC server listening", serverName, "address", lis.Addr().String())
		if err := s.Serve(lis); err != nil {
			logger.Error(err, "Failed to serve gRPC", serverName)
		}
	}()

	// 启动 HTTP 服务
	// 使用Gin框架启动HTTP服务
	g := gin.Default()

	// 启动 gRPC-Gateway 反向代理
	go func() {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mux := runtime.NewServeMux()
		opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		// 注册 gRPC-Gateway 处理器
		err := pb.RegisterAuthServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts)
		if err != nil {
			logger.Error(err, "Failed to register gRPC-Gateway", serverName)
			return
		}

		// 将 gRPC-Gateway 的 mux 作为 Gin 的一个路由
		// 注意：这里使用 Any 匹配所有 /v1/ 开头的请求
		g.Any("/v1/*any", gin.WrapH(mux))
		logger.Info("gRPC-Gateway initialized", serverName, "path", "/v1/*")
	}()

	// 初始化健康检查组件
	healthChecker := service.NewHealthCheckerService(dbase)
	// 健康检查端点
	// 提供深度健康检查接口，验证关键依赖状态
	g.GET("/health", func(c *gin.Context) {
		status := healthChecker.CheckHealth()
		c.JSON(200, status)
	})

	// 配置管理端点 (仅在Consul配置存储时启用)
	if cfgStore.StoreType == "consul" {
		configHandler := config.NewConfigHandler(discoveryClient, provider, serverName, cfgStore.Env)
		configHandler.RegisterConfigRoutes(g)
	}

	// 第1步. 通过ClientId,ClientSecret 来获取客户端默认的授权令牌
	g.POST("/oauth2/token", handles.MakeTokenEndpoint(tokenGranter, clientDetailsService))

	// 默认的客户端授权令牌刷新接口，默认用户信息存储在令牌中，未登录的用户直接使用默认令牌访问，当令牌快速过期时，可以通过此接口刷新令牌
	g.POST("/oauth2/client-refresh", middleware.MakeSimpleClientMiddleware(tokenService), handles.RefreshTokenEndpoint(tokenService))

	// 2. 面向客户端的用户登录接口，这里主要是通过默认令牌以及用户名和密码进行认证，其中令牌中客户端信息存储在客户端默认的令牌中从中间件中获取，用户名和密码通过json传值，生成用户访问令牌，
	g.POST("/login", middleware.MakeSimpleClientMiddleware(tokenService), handles.LoginEndPoint(userDetailsService, tokenService))

	// 3. 通过用户授权令牌来获取刷新令牌，这里主要是通过用户授权令牌进行认证，生成新的访问令牌和刷新令牌，用户信息和客户端等信息存储在令牌中，从中间件中获取
	g.POST("/oauth2/refresh", middleware.MakeAuthorityAuthorizationMiddleware(tokenService), handles.RefreshTokenEndpoint(tokenService))

	g.POST("/oauth2/verify", handles.VerifyTokenEndpoint(tokenService))

	g.POST("/client/register", handles.RegisterClientEndPoint(clientDetailsService))

	g.POST("/user/register", middleware.MakeSimpleClientMiddleware(tokenService), handles.RegisterUserEndPoint(userDetailsService, []string{"admin"}))

	// admin 接口，需要管理员权限才能访问（测试案例）
	g.POST("/admin",
		middleware.MakeAuthorityAuthorizationMiddleware(tokenService),
		middleware.MakeScopeHandler("admin"),
		func(ctx *gin.Context) {
			ctx.JSON(200, gin.H{
				"message": "Hello Admin!",
			})
		},
	)

	// 启动 HTTP 服务
	// 监听指定端口提供服务
	logger.Info("Starting server", serverName, "port", appConfig.Server.Port)
	if err := g.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName)
	}
}
