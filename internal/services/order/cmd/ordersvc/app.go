package main

import (
	"easyms/internal/platform/outbox_relay"
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

// App is the container for all components.
type App struct {
	engine       *gin.Engine
	ds           *discovery.Discovery
	relayService *outbox_relay.RelayService
	publisher    mq.Publisher
}

// InitializeApp manually builds and returns a complete App instance.
func InitializeApp(serverName string, env string) (*App, func(), error) {
	// --- 1. Bootstrap Config Loading ---
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init app config store: %w", err)
	}

	// --- 2. Create Core Dependencies based on Bootstrap Config ---
	discoveryClient, discoveryCleanup, err := provideDiscovery(cfgStore)
	if err != nil {
		return nil, nil, err
	}

	// --- 3. Create and Load Real App Config based on Bootstrap Config ---
	appConfig, err := provideAppConfig(cfgStore, discoveryClient, serverName, env)
	if err != nil {
		discoveryCleanup()
		return nil, nil, err
	}

	// --- 4. Logger Initialization (must be after config loading) ---
	logger.Init(serverName, appConfig)

	// --- 5. Initialize Other Dependencies that rely on AppConfig ---
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

	// --- 6. Service Layer Initialization ---
	orderService := service.NewOrderService(dbase)
	relayService := outbox_relay.NewRelayService(dbase, publisher, 10*time.Second)

	// --- 7. Interface Layer Initialization ---
	orderHandler := handles.MakeCreateOrderEndpoint(orderService)
	engine := provideGinEngine(orderHandler)

	// --- 8. Build App ---
	app := &App{
		engine:       engine,
		ds:           discoveryClient,
		relayService: relayService,
		publisher:    publisher,
	}

	// --- 9. Define Cleanup Function ---
	cleanup := func() {
		relayService.Stop()
		publisherCleanup()
		discoveryCleanup()
	}

	// --- 10. Start Background Services ---
	if err := discoveryClient.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to register service: %w", err)
	}
	relayService.Start()

	return app, cleanup, nil
}

// --- Provider Functions ---

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
	// Health check endpoint
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})
	g.POST("/orders", orderHandler)
	return g
}
