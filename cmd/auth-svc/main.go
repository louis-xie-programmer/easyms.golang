// main.go 认证服务主程序
// 主要功能：
// 1. 初始化服务配置
// 2. 启动服务发现客户端
// 3. 注册服务到Consul
// 4. 初始化认证服务组件
// 5. 启动HTTP服务
package main

import (
	"easyms/cmd/auth-svc/handles"
	"easyms/cmd/auth-svc/service"
	"easyms/cmd/auth-svc/storage"
	"easyms/pkg/config"
	"easyms/pkg/db"
	"easyms/pkg/discovery"
	"easyms/pkg/logger"
	"fmt"
	"github.com/gin-gonic/gin"
	"time"
)

func init() {
	// 初始化配置
}

// 认证服务名称
var serverName = "auth-svc"

// main 认证服务入口函数
// 初始化配置、服务发现客户端，注册服务，初始化认证组件并启动HTTP服务
func main() {
	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		fmt.Println("Failed to initialize app config store")
		logger.Error(err, "Failed to initialize app config store", serverName, nil)
	}

	// 初始化 Consul 服务发现客户端
	// 连接到Consul服务注册与发现中心
	d, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName, nil)
	}

	fmt.Printf("Initializing %v\n", cfgStore)
	// 加载服务配置
	// 根据配置类型（本地或Consul）加载服务配置
	err = config.LoadServiceConfig(d, cfgStore.Consul.KeyPath, cfgStore.StoreType, serverName, cfgStore.Env)
	if err != nil {
		fmt.Printf("Failed to load service config %s: %v\n", cfgStore, err)
		logger.Error(err, "Failed to load service config", serverName, nil)
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()

	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	fmt.Printf("Starting %s on %s port: %d\n", serverName, appConfig.Server.Host, appConfig.Server.Port)

	// 注册服务到Consul
	// 将当前服务注册到Consul服务注册中心
	// 添加短暂延迟以避免服务注册冲突
	time.Sleep(time.Millisecond * 100)
	err = d.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil)
	if err != nil {
		fmt.Printf("Failed to register service %v", appConfig)
		logger.Error(err, "Failed to register service", serverName, nil)
		//panic(err)
	}

	// 延迟注销服务
	// 确保服务在退出时从Consul中注销
	defer d.DeRegister(serverName)

	// 初始化认证服务组件
	// 初始化各种认证服务相关的组件
	var tokenService service.TokenService
	var tokenGranter service.TokenGranter
	var tokenEnhancer storage.TokenEnhancer
	var tokenStore storage.TokenStore
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

	// 初始化令牌存储器
	// 使用JWT令牌存储器
	tokenStore = storage.NewJwtTokenStore(tokenEnhancer.(*storage.JwtTokenEnhancer), dbase.(*db.EasyDatabase))

	// 初始化令牌服务
	tokenService = service.NewTokenService(tokenStore, tokenEnhancer)

	// 初始化用户详情服务
	userDetailsService = service.NewPostgresUserDetailsService(dbase.(*db.EasyDatabase))

	// 初始化客户端详情服务
	clientDetailsService = service.NewPostgresClientDetailsService(dbase.(*db.EasyDatabase))

	// 初始化令牌授予器
	tokenGranter = service.NewComposeTokenGranter(map[string]service.TokenGranter{
		"client_credentials": service.NewClientCredentialsTokenGranter("client_credentials", clientDetailsService, tokenService),
		"password":           service.NewUsernamePasswordTokenGranter("password", userDetailsService, tokenService),
		"refresh_token":      service.NewRefreshGranter("refresh_token", tokenService),
	})

	// 启动 HTTP 服务
	// 使用Gin框架启动HTTP服务
	g := gin.Default()

	// 健康检查端点
	// 提供健康检查接口，供Consul等监控系统使用
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})

	// 初始化配置管理接口
	configHandler := config.NewConfigHandler(d, serverName, cfgStore.Env)
	configHandler.RegisterConfigRoutes(g)

	// 客户端认证路由组 - 用于获取客户端令牌
	client_auth_r := g.Group("/c-auth")
	client_auth_r.POST("/token", handles.MakeClientAuthorizationMiddleware(clientDetailsService), handles.MakeTokenEndpoint(tokenGranter))
	// 客户端刷新令牌端点 - 用于刷新客户端令牌
	client_auth_r.POST("/refresh", handles.MakeClientOnlyAuthorizationMiddleware(tokenService), handles.RefreshClientTokenEndpoint(tokenService))

	// OAuth2相关路由组
	oauth2_router := g.Group("/oauth2")
	// 登录端点 - 通过客户端认证后获取用户令牌
	oauth2_router.POST("/login", handles.MakeClientOnlyAuthorizationMiddleware(tokenService), handles.LoginEndPoint(userDetailsService, tokenService))
	// 用户令牌刷新端点 - 用于刷新用户令牌
	oauth2_router.POST("/refresh", handles.MakeAuthorityAuthorizationMiddleware(tokenService), handles.RefreshTokenEndpoint(tokenService))

	// API V1路由组
	v1_router := g.Group("/api/v1")
	// 添加用户权限验证中间件
	v1_router.Use(handles.MakeAuthorityAuthorizationMiddleware(tokenService))

	v1_router.POST("/user/:id", func(c *gin.Context) {
		c.String(200, "user id: "+c.Param("id"))
	})

	// 仅为客户端凭证授权开放的资源组
	client_only_router := v1_router.Group("/client-resources")
	// 添加客户端专用授权中间件
	client_only_router.Use(handles.MakeClientOnlyAuthorizationMiddleware(tokenService))

	client_only_router.POST("/resource", func(c *gin.Context) {
		c.String(200, "this resource is only accessible by client credentials grant")
	})

	// 启动HTTP服务
	// 监听指定端口提供HTTP服务
	g.Run(fmt.Sprintf(":%d", appConfig.Server.Port))
}