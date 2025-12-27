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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"

	"github.com/gin-gonic/gin"
)

// App is the container for all components.
type App struct {
	httpServer   *http.Server
	ds           *discovery.Discovery
	relayService *outbox_relay.RelayService
	publisher    mq.Publisher
}

// InitializeApp manually builds and returns a complete App instance.
func InitializeApp(serverName string, env string) (*App, func(), error) {
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init app config store: %w", err)
	}

	discoveryClient, discoveryCleanup, err := provideDiscovery(cfgStore, serverName)
	if err != nil {
		return nil, nil, err
	}

	appConfig, err := provideAppConfig(cfgStore, discoveryClient, serverName, env)
	if err != nil {
		discoveryCleanup()
		return nil, nil, err
	}

	logger.Init(serverName, appConfig)

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

	orderServiceLogger := logger.With("component", "order_service")
	orderService := service.NewOrderService(dbase, orderServiceLogger)

	// Pass the outbox config to the relay service
	relayService := outbox_relay.NewRelayService(dbase, publisher, appConfig.Outbox)

	orderHandler := handles.MakeCreateOrderEndpoint(orderService)
	httpServer := provideHttpServer(orderHandler, appConfig)

	app := &App{
		httpServer:   httpServer,
		ds:           discoveryClient,
		relayService: relayService,
		publisher:    publisher,
	}

	cleanup := func() {
		relayService.Stop()
		publisherCleanup()
		discoveryCleanup()
	}

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

func provideDiscovery(cfgStore *models.AppConfigStore, serverName string) (*discovery.Discovery, func(), error) {
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		logger.Info("Deregistering service from Consul...", serverName)
		discoveryClient.DeRegister(serverName)
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

func provideHttpServer(orderHandler gin.HandlerFunc, cfg *models.AppConfig) *http.Server {
	g := gin.Default()
	g.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	// Expose metrics endpoint
	g.GET("/metrics", gin.WrapH(promhttp.Handler()))
	g.POST("/orders", orderHandler)

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: g,
	}
}
