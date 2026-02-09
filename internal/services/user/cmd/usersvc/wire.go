//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

// Package main 是用户服务 (user-svc) 的主程序入口。
// 这个文件 (wire.go) 定义了使用 Google Wire 进行依赖注入的规则。
// Wire 会根据这里定义的 Provider 函数自动生成依赖注入的代码 (在 wire_gen.go 中)。
package main

import (
	"context"
	"easyms/internal/services/user/internal/handles"
	"easyms/internal/services/user/internal/service"
	"easyms/internal/services/user/internal/storage"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/tracing"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"net/http"
)

// App 是包含了用户服务所有核心组件的容器。
type App struct {
	httpServer *http.Server
	ds         *discovery.Discovery
}

// NewApp 创建一个新的 App 实例。
func NewApp(httpServer *http.Server, ds *discovery.Discovery) *App {
	return &App{
		httpServer: httpServer,
		ds:         ds,
	}
}

// ConfigInputs 封装了在构建依赖图时需要传入的简单类型参数。
type ConfigInputs struct {
	ServerName string
	Env        string
}

// HealthHandler 提供了健康检查的 HTTP Handler。
type HealthHandler struct {
	db db.Database
}

// NewHealthHandler 创建一个新的 HealthHandler 实例。
func NewHealthHandler(db db.Database) *HealthHandler {
	return &HealthHandler{db: db}
}

// Check 是健康检查的具体实现，它会检查数据库连接是否正常。
func (h *HealthHandler) Check(c *gin.Context) {
	sqlDB, err := h.db.GetDB().DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": "无法获取数据库实例"})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": "数据库 ping 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// providerSet 是一个 Wire Provider 集合，它告诉 Wire 如何创建各个组件。
var providerSet = wire.NewSet(
	// --- 基础设施层 Providers ---
	config.InitAppConfigStore,
	provideDiscovery,
	provideAppConfig,
	provideDatabase,
	provideHttpServer,
	provideTracer,
	NewHealthHandler,

	// --- 存储层 Providers ---
	storage.NewUserStorage,

	// --- 服务层 Providers ---
	service.NewUserService,

	// --- 处理器 (Handler) 层 Providers ---
	handles.NewUserHandler,

	// --- 应用顶层 Provider ---
	NewApp,
)

// --- Provider 函数的具体实现 ---

// provideAppConfig 负责提供 AppConfig 实例。
// 它会根据配置决定是从本地文件还是从 Consul 加载配置。
func provideAppConfig(cfgStore *models.AppConfigStore, discoveryClient *discovery.Discovery, inputs ConfigInputs) (*models.AppConfig, error) {
	var provider config.AppConfigProvider
	if cfgStore.StoreType == "consul" {
		provider = config.NewConsulConfig(discoveryClient, inputs.ServerName, cfgStore.Consul.KeyPath, inputs.Env)
	} else {
		provider = config.NewLocalConfig(inputs.ServerName, inputs.Env)
	}

	if err := provider.LoadAppConfig(); err != nil {
		return nil, err
	}
	return config.GetAppConfig(), nil
}

// provideDiscovery 负责提供服务发现客户端实例，并返回一个用于资源清理的函数。
func provideDiscovery(cfgStore *models.AppConfigStore, inputs ConfigInputs) (*discovery.Discovery, func(), error) {
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		return nil, nil, err
	}
	// 定义清理函数，用于在程序退出时注销服务
	cleanup := func() {
		logger.Info("正在从 Consul 注销服务...", inputs.ServerName)
		discoveryClient.DeRegister(inputs.ServerName)
	}
	return discoveryClient, cleanup, nil
}

// provideTracer 负责提供分布式追踪实例，并返回一个用于资源清理的函数。
func provideTracer(cfg *models.AppConfig, inputs ConfigInputs) (func(), error) {
	if !cfg.Tracing.Enable {
		return func() {}, nil // 如果未启用，返回一个空操作的清理函数
	}
	shutdown, err := tracing.InitTracerProvider(inputs.ServerName, cfg.Tracing.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("初始化 tracer 失败: %w", err)
	}
	// 定义清理函数，用于在程序退出时关闭 Tracer Provider
	cleanup := func() {
		logger.Info("正在关闭 tracer provider...", inputs.ServerName)
		if err := shutdown(context.Background()); err != nil {
			logger.Error(err, "关闭 tracer provider 失败", inputs.ServerName)
		}
	}
	return cleanup, nil
}

// provideDatabase 负责提供数据库实例，并返回一个用于资源清理的函数。
func provideDatabase(cfg *models.AppConfig) (db.Database, func(), error) {
	// 构建数据库连接 DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Database.UserName, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)

	// 创建带连接池的数据库实例
	dbase, err := db.NewEasyDatabaseWithPool(cfg.Database.Type, dsn, &cfg.Database)
	if err != nil {
		return nil, nil, err
	}
	// 自动迁移 User 表
	if err := dbase.AutoMigrate(&models.User{}); err != nil {
		return nil, nil, err
	}

	// 定义清理函数，用于在程序退出时关闭数据库连接
	cleanup := func() {
		logger.Info("正在关闭数据库连接...")
		if err := dbase.Close(); err != nil {
			logger.Error(err, "关闭数据库连接失败")
		}
	}

	return dbase, cleanup, nil
}

// provideHttpServer 负责创建和配置 Gin HTTP 服务器实例。
func provideHttpServer(userHandler *handles.UserHandler, healthHandler *HealthHandler, cfg *models.AppConfig, inputs ConfigInputs) *http.Server {
	g := gin.Default()

	// 如果启用了追踪，则添加 OpenTelemetry 中间件
	if cfg.Tracing.Enable {
		g.Use(otelgin.Middleware(inputs.ServerName))
	}

	// 注册路由
	g.GET("/health", healthHandler.Check)
	userRoutes := g.Group("/users")
	{
		userRoutes.GET("/:id", userHandler.GetUserByID)
		userRoutes.POST("/", userHandler.CreateUser)
	}

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: g,
	}
}

// InitializeApp 是 Wire 的入口点。
// Wire 会分析 providerSet，并自动生成一个函数体来填充这里的 `nil`。
// 这个函数将按照正确的顺序创建和连接所有依赖项。
func InitializeApp(inputs ConfigInputs) (*App, func(), error) {
	wire.Build(providerSet)
	return nil, nil, nil
}
