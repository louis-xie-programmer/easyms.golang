package main

import (
	"easyms/internal/services/order/internal/handles"
	"easyms/internal/services/order/internal/service"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/mq"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// App 是所有组件的容器。
type App struct {
	engine       *gin.Engine
	ds           *discovery.Discovery
	relayService *service.RelayService
	publisher    mq.Publisher
}

// InitializeApp 手动构建并返回一个完整的 App 实例。
// 这次它将正确地处理两步配置加载。
func InitializeApp(serverName string, env string) (*App, func(), error) {
	// --- 1. 引导配置加载 ---
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init app config store: %w", err)
	}

	// --- 2. 根据引导配置，创建核心依赖 ---
	discoveryClient, discoveryCleanup, err := provideDiscovery(cfgStore)
	if err != nil {
		return nil, nil, err
	}

	// --- 3. 根据引导配置，创建并加载真实的应用配置 ---
	appConfig, err := provideAppConfig(cfgStore, discoveryClient, serverName, env)
	if err != nil {
		discoveryCleanup()
		return nil, nil, err
	}

	// --- 4. 日志初始化 (必须在配置加载后) ---
	logger.Init(serverName, appConfig)

	// --- 5. 初始化其他依赖于 AppConfig 的组件 ---
	dbase, err := provideDatabase(appConfig)
	if err != nil {
		discoveryCleanup()
		return nil, nil, err
	}

	publisher, publisherCleanup, err := providePublisher(appConfig)
	if err != nil {
		discoveryCleanup()
		return nil, nil, err
	}

	// --- 6. 服务层初始化 ---
	orderService := service.NewOrderService(dbase)
	relayService := service.NewRelayService(dbase, publisher, 10*time.Second)

	// --- 7. 接口层初始化 ---
	orderHandler := handles.MakeCreateOrderEndpoint(orderService)
	engine := provideGinEngine(orderHandler)

	// --- 8. 构建 App ---
	app := &App{
		engine:       engine,
		ds:           discoveryClient,
		relayService: relayService,
		publisher:    publisher,
	}

	// --- 9. 定义清理函数 ---
	cleanup := func() {
		relayService.Stop()
		publisherCleanup()
		discoveryCleanup()
	}

	// --- 10. 启动后台服务 ---
	if err := discoveryClient.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to register service: %w", err)
	}
	relayService.Start()

	return app, cleanup, nil
}

// --- Provider 函数 ---

// provideAppConfig 是核心的改造，它实现了两步加载逻辑
func provideAppConfig(cfgStore *models.AppConfigStore, discoveryClient *discovery.Discovery, serverName, env string) (*models.AppConfig, error) {
	var provider config.AppConfigProvider
	if cfgStore.StoreType == "consul" {
		provider = config.NewConsulConfig(discoveryClient, serverName, cfgStore.Consul.KeyPath, env)
	} else {
		provider = config.NewLocalConfig(serverName, env)
	}

	if err := provider.LoadAppConfig(); err != nil {
		return nil, err
	}
	return config.GetAppConfig(), nil
}

func provideDiscovery(cfgStore *models.AppConfigStore) (*discovery.Discovery, func(), error) {
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		discoveryClient.DeRegister("order-svc")
	}
	return discoveryClient, cleanup, nil
}

func provideDatabase(cfg *models.AppConfig) (db.Database, error) {
	dbase, err := db.NewEasyDatabaseWithPool(cfg.Database.Type, fmt.Sprintf("%s://%s:%s@%s:%d/%s",
		cfg.Database.Type, cfg.Database.UserName, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database), cfg.Database)
	if err != nil {
		return nil, err
	}
	if err := dbase.AutoMigrate(&models.Order{}, &models.OrderItem{}, &models.OutboxEvent{}); err != nil {
		return nil, err
	}
	return dbase, nil
}

func providePublisher(cfg *models.AppConfig) (mq.Publisher, func(), error) {
	pub, err := mq.NewRabbitMQPublisher(cfg.RabbitMQ.URL)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		pub.Close()
	}
	return pub, cleanup, nil
}

func provideGinEngine(orderHandler gin.HandlerFunc) *gin.Engine {
	g := gin.Default()
	// 健康检查端点
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})
	g.POST("/orders", orderHandler)
	return g
}
