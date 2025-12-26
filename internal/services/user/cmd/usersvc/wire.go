// go:build wireinject
//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"easyms/internal/services/user/internal/handles"
	"easyms/internal/services/user/internal/service"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
)

// App 是 user-svc 所有组件的容器。
type App struct {
	engine *gin.Engine
	ds     *discovery.Discovery
}

// NewApp 创建一个新的 App 实例。
func NewApp(engine *gin.Engine, ds *discovery.Discovery, cfg *models.AppConfig, inputs ConfigInputs) (*App, error) {
	// 在这里执行服务注册
	err := ds.Register(inputs.ServerName, cfg.Server.Host, cfg.Server.Port, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to register service: %w", err)
	}

	return &App{
		engine: engine,
		ds:     ds,
	}, nil
}

// ConfigInputs 用于封装传递给 wire 的简单类型参数。
type ConfigInputs struct {
	ServerName string
	Env        string
}

// HealthHandler 用于健康检查
type HealthHandler struct {
	db db.Database
}

func NewHealthHandler(db db.Database) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Check(c *gin.Context) {
	// 检查数据库连接
	sqlDB, err := h.db.GetDB().DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": "failed to get db instance"})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": "db ping failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// providerSet 集合了所有组件的构造函数。
var providerSet = wire.NewSet(
	// --- 基础组件 Providers ---
	config.InitAppConfigStore,
	provideDiscovery,
	provideAppConfig, // 依赖 ConfigInputs
	provideDatabase,
	provideGinEngine,
	NewHealthHandler,

	// --- 服务层 Providers ---
	service.NewUserService,

	// --- 接口层 Providers ---
	handles.NewUserHandler,

	// --- App Provider ---
	NewApp,
)

// --- Provider 函数的具体实现 ---

// provideAppConfig 现在依赖于 ConfigInputs 结构体，解决了多字符串参数问题。
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

// provideDiscovery 现在依赖于引导配置 cfgStore
func provideDiscovery(cfgStore *models.AppConfigStore, inputs ConfigInputs) (*discovery.Discovery, func(), error) {
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		discoveryClient.DeRegister(inputs.ServerName)
	}
	return discoveryClient, cleanup, nil
}

func provideDatabase(cfg *models.AppConfig) (db.Database, error) {
	dbase, err := db.NewEasyDatabaseWithPool(cfg.Database.Type, fmt.Sprintf("%s://%s:%s@%s:%d/%s",
		cfg.Database.Type, cfg.Database.UserName, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database), cfg.Database)
	if err != nil {
		return nil, err
	}
	// 自动迁移 User 模型
	if err := dbase.AutoMigrate(&models.User{}); err != nil {
		return nil, err
	}
	return dbase, nil
}

func provideGinEngine(userHandler *handles.UserHandler, healthHandler *HealthHandler) *gin.Engine {
	g := gin.Default()
	// 健康检查端点
	g.GET("/health", healthHandler.Check)
	// 用户相关路由
	userRoutes := g.Group("/users")
	{
		userRoutes.GET("/:id", userHandler.GetUserByID)
		userRoutes.POST("/", userHandler.CreateUser)
	}
	return g
}

// InitializeApp 是 Wire 的入口点（Injector）。
// 它的参数现在是 ConfigInputs 结构体。
func InitializeApp(inputs ConfigInputs) (*App, func(), error) {
	wire.Build(providerSet)
	return nil, nil, nil
}
