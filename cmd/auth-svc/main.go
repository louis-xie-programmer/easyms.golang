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
	"easyms/cmd/auth-svc/model"
	"easyms/cmd/auth-svc/service"
	"easyms/cmd/auth-svc/storage"
	"easyms/pkg/config"
	"easyms/pkg/db"
	"easyms/pkg/discovery"
	"easyms/pkg/logger"
	"fmt"
	"github.com/gin-gonic/gin"
	"os"
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
	
	// 注册服务到Consul
	// 将当前服务注册到Consul服务注册中心
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
	dbase, err := db.NewEasyDatabaseWithPool(appConfig.Database.Type,
		fmt.Sprintf("%s://%s:%s@%s:%d/%s", appConfig.Database.Type,
			appConfig.Database.UserName,
			appConfig.Database.Password,
			appConfig.Database.Host,
			appConfig.Database.Port,
			appConfig.Database.Database),
		appConfig.Database)
	if err != nil {
		logger.Error(err, "Failed to connect to database", serverName, nil)
		panic(err)
	}

	// 初始化令牌存储器
	// 使用JWT令牌存储器
	tokenStore = storage.NewJwtTokenStore(dbase)

	// 初始化令牌服务
	tokenService = service.NewTokenService(tokenStore, tokenEnhancer)

	// 初始化用户详情服务
	userDetailsService = service.NewUserDetailService(dbase.(*db.EasyDatabase))

	// 创建测试用户和客户端(并保存到数据库)
	createTestUsers(dbase)
	createTestClients()

	// 初始化客户端详情服务
	clientDetailsService = service.NewPostgresClientDetailsService(dbase.(*db.EasyDatabase))

	// 初始化令牌授予器
	tokenGranter = service.NewComposeTokenGranter(map[string]service.TokenGranter{
		"password":      service.NewUsernamePasswordTokenGranter("password", userDetailsService, tokenService),
		"refresh_token": service.NewRefreshGranter("refresh_token", userDetailsService, tokenService),
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

	// OAuth2相关路由组
	oauth2_router := g.Group("/oauth2")
	// 添加客户端授权中间件
	oauth2_router.Use(handles.MakeClientAuthorizationMiddleware(clientDetailsService))
	// 令牌端点
	oauth2_router.POST("/token", handles.MakeTokenEndpoint(tokenGranter))
	// 检查令牌端点
	oauth2_router.POST("/check_token", handles.CheckTokenEndPoint(tokenService))
	// 登录端点
	oauth2_router.POST("/login", handles.LoginEndPoint(userDetailsService, tokenService))

	// API V1路由组
	v1_router := g.Group("/api/v1")
	// 添加客户端授权中间件
	v1_router.Use(handles.MakeClientAuthorizationMiddleware(clientDetailsService))

	// 启动HTTP服务
	// 监听指定端口提供HTTP服务
	g.Run(fmt.Sprintf(":%d", appConfig.Server.Port))
}

// createTestUsers 创建测试用户
// 创建默认测试用户并保存到数据库
func createTestUsers(dbase db.Database) map[string]*model.UserDetails {
	users := map[string]*model.UserDetails{
		"user1": {
			Username: "user1",
			Password: "password1",
			Authorities: []string{"USER"},
		},
		"admin": {
			Username: "admin",
			Password: "password2",
			Authorities: []string{"USER", "ADMIN"},
		},
	}

	// 为用户生成密码哈希
	for _, user := range users {
		err := user.HashPassword()
		if err != nil {
			logger.Error(err, "Failed to hash password", serverName, [][]string{{"event", "hash_password"}})
			os.Exit(-1)
		}
		user.Password = "" // 清除明文密码

		// 自动迁移用户表结构
		err = dbase.AutoMigrate(user)
		if err != nil {
			logger.Error(err, "Failed to auto migrate user", serverName, [][]string{{"event", "auto_migrate"}})
		}
		
		// 插入用户数据到数据库
		err = dbase.Insert(user)
		if err != nil {
			logger.Error(err, "Failed to insert user", serverName, [][]string{{"event", "insert_user"}})
		}
	}

	return users
}

// createTestClients 创建测试客户端
// 创建默认测试客户端
func createTestClients() map[string]*model.ClientDetails {
	return map[string]*model.ClientDetails{
		"client1": {
			ClientId:                    "client1",             // 客户端ID
			ClientSecret:                "client1_secret",      // 客户端密钥
			AccessTokenValiditySeconds:  3600,                 // 访问令牌有效期（秒）
			RefreshTokenValiditySeconds: 7200,                 // 刷新令牌有效期（秒）
			RegisteredRedirectUri:       "http://localhost:3000/callback", // 注册重定向URI
			AuthorizedGrantTypes:        []string{"password", "refresh_token"}, // 授权类型
		},
	}
}