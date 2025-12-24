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

func main() {
	serverName := "order-svc"

	// --- 配置加载 ---
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize app config store: %v", err))
	}
	provider := config.NewLocalConfig(serverName, cfgStore.Env)
	if err := provider.LoadAppConfig(); err != nil {
		panic(fmt.Sprintf("Failed to load local app config: %v", err))
	}
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		panic("Application config is not loaded")
	}

	// --- 日志初始化 ---
	logger.Init(serverName, appConfig)

	// --- 服务发现 ---
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName)
		panic(err)
	}
	if err := discoveryClient.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil); err != nil {
		logger.Error(err, "Failed to register service with consul", serverName)
		panic(err)
	}
	defer discoveryClient.DeRegister(serverName)

	// --- 数据库连接 ---
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

	// --- 自动迁移数据库表 ---
	logger.Info("Auto-migrating database tables...", serverName)
	if err := dbase.AutoMigrate(&model.Order{}, &model.OrderItem{}, &model.OutboxEvent{}); err != nil {
		logger.Error(err, "Failed to auto-migrate tables", serverName)
		panic(err)
	}
	logger.Info("Database migration completed.", serverName)

	// --- 消息队列 Publisher 初始化 ---
	publisher, err := mq.NewRabbitMQPublisher(appConfig.RabbitMQ.URL)
	if err != nil {
		logger.Error(err, "Failed to create RabbitMQ publisher", serverName)
		panic(err)
	}
	defer publisher.Close()

	// --- 依赖注入 ---
	orderService := service.NewOrderService(dbase) // OrderService 不再直接依赖 publisher

	// --- 启动 Outbox Relay 服务 ---
	relayService := service.NewRelayService(dbase, publisher, 10*time.Second) // 每10秒轮询一次
	relayService.Start()
	defer relayService.Stop()

	// --- HTTP 服务启动 ---
	g := gin.Default()
	g.POST("/orders", handles.MakeCreateOrderEndpoint(orderService))

	logger.Info("Starting server", serverName, "port", appConfig.Server.Port)
	if err := g.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName)
	}
}
