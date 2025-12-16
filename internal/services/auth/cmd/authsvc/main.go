// internal/services/auth/main.go
package main

import (
	"easyms/internal/services/auth/internal/handles"
	"easyms/internal/services/auth/internal/middleware"
	"easyms/internal/services/auth/internal/service"
	"easyms/internal/services/auth/internal/storage"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/entities"
	"easyms/internal/shared/logger"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// main 认证服务主函数
func main() {
	// 认证服务名称和端口
	serverName := "auth-svc"

	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		logger.Error(err, "Failed to initialize app config store", serverName, nil)
	}

	// 按需初始化Discovery客户端
	var discoveryClient *discovery.Discovery
	// Consul 不仅仅是配置中心，还是服务发现组件,所以单独创建
	if cfgStore.Consul != (entities.ConsulConfig{}) {
		discoveryClient, err = discovery.NewDiscovery(cfgStore.Consul.Host)
		if err != nil {
			logger.Error(err, "Failed to create consul client", serverName, nil)
		}
	}

	var provider config.AppConfigProvider

	// 加载应用配置
	if cfgStore.StoreType == "consul" {
		// 使用Consul配置提供者
		provider = config.NewConsulConfig(discoveryClient, serverName, cfgStore.Consul.KeyPath, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			logger.Error(err, "Failed to load app config", serverName, nil)
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
			logger.Error(err, "Failed to load local app config", serverName, nil)
			panic(err)
		}
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()

	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	// 初始化认证服务组件
	// 初始化各种认证服务相关的组件
	var tokenService service.TokenService
	var tokenGranter service.TokenGranter
	var tokenEnhancer storage.TokenEnhancer
	// var tokenStore storage.TokenStore
	var userDetailsService service.UserDetailsService
	var clientDetailsService service.ClientDetailsService

	// 初始化JWT令牌增强器
	tokenEnhancer = storage.NewJwtTokenEnhancer("secret")

	// 初始化数据库连接
	// 根据配置连接到数据库
	// 添加重试机制以避免连接冲突
	var dbase db.Database
	for i := 0; i < 3; i++ {
		dbase, err = db.NewEasyDatabaseWithPool(appConfig.Database.Type,
			fmt.Sprintf("%s://%s:%s@%s:%d/%s", appConfig.Database.Type,
				appConfig.Database.UserName,
				appConfig.Database.Password,
				appConfig.Database.Host,
				appConfig.Database.Port,
				appConfig.Database.Database),
			appConfig.Database)
		if err == nil {
			break
		}
		time.Sleep(time.Second * time.Duration(i+1))
	}

	if err != nil {
		logger.Error(err, "Failed to connect to database", serverName, nil)
		panic(err)
	}

	// 通过依赖注入创建服务
	if appConfig.Database.Type != "postgres" {
		panic("unsupported database type: " + appConfig.Database.Type)
	}
	tokenStore := storage.NewJwtTokenStore(tokenEnhancer.(*storage.JwtTokenEnhancer), dbase)
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

	if discoveryClient != nil {
		// 注册服务到Consul
		// 将当前服务注册到Consul服务注册中心
		// 添加短暂延迟以避免服务注册冲突
		time.Sleep(time.Millisecond * 200)
		err = discoveryClient.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil)
		if err != nil {
			panic(err)
		}

		// 延迟注销服务
		// 确保服务在退出时从Consul中注销
		defer discoveryClient.DeRegister(serverName)
	}

	// 启动 HTTP 服务
	// 使用Gin框架启动HTTP服务
	g := gin.Default()

	// 健康检查端点
	// 提供健康检查接口，供Consul等监控系统使用
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})

	// 配置管理端点 (仅在Consul配置存储时启用)
	if cfgStore.StoreType == "consul" {
		configHandler := config.NewConfigHandler(discoveryClient, provider, serverName, cfgStore.Env)
		configHandler.RegisterConfigRoutes(g)
	}

	// 第1步. 通过ClientId,ClientSecret 来获取客户端默认的授权令牌，注意默认用户直接存储在数据库中，通过客户端Id和ClientSecret进行认证，同时从数据库中查询默认用户信息，最终生成访问令牌
	g.POST("/oauth2/token", handles.MakeTokenEndpoint(tokenGranter, clientDetailsService))

	// 默认的客户端授权令牌刷新接口，默认用户信息存储在令牌中，未登录的用户直接使用默认令牌访问，当令牌快速过期时，可以通过此接口刷新令牌
	g.POST("/oauth2/client-refresh", middleware.MakeSimpleClientMiddleware(tokenService), handles.RefreshTokenEndpoint(tokenService))

	// 2. 面向客户端的用户登录接口，这里主要是通过默认令牌以及用户名和密码进行认证，其中令牌中客户端信息存储在客户端默认的令牌中从中间件中获取，用户名和密码通过json传值，生成用户访问令牌，
	g.POST("/login", middleware.MakeSimpleClientMiddleware(tokenService), handles.LoginEndPoint(userDetailsService, tokenService))

	// 3. 通过用户授权令牌来获取刷新令牌，这里主要是通过用户授权令牌进行认证，生成新的访问令牌和刷新令牌，用户信息和客户端等信息存储在令牌中，从中间件中获取
	g.POST("/oauth2/refresh", middleware.MakeAuthorityAuthorizationMiddleware(tokenService), handles.RefreshTokenEndpoint(tokenService))

	// 启动 HTTP 服务
	// 监听指定端口提供服务
	fmt.Printf("Starting %s on port %d\n", serverName, appConfig.Server.Port)
	if err := g.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName, nil)
	}
}
