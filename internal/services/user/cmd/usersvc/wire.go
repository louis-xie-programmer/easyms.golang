// go:build wireinject
//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"easyms/internal/services/user/internal/handles"
	"easyms/internal/services/user/internal/service"
	"easyms/internal/services/user/internal/storage" // Import the new storage package
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/tracing"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// App is the container for all components of the user-svc.
type App struct {
	httpServer *http.Server
	ds         *discovery.Discovery
}

// NewApp creates a new App instance.
func NewApp(httpServer *http.Server, ds *discovery.Discovery) *App {
	return &App{
		httpServer: httpServer,
		ds:         ds,
	}
}

// ConfigInputs encapsulates simple type parameters for Wire.
type ConfigInputs struct {
	ServerName string
	Env        string
}

// HealthHandler for health checks.
type HealthHandler struct {
	db db.Database
}

func NewHealthHandler(db db.Database) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Check(c *gin.Context) {
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

// providerSet aggregates providers for all components.
var providerSet = wire.NewSet(
	// --- Infrastructure Providers ---
	config.InitAppConfigStore,
	provideDiscovery,
	provideAppConfig,
	provideDatabase,
	provideHttpServer,
	provideTracer,
	NewHealthHandler,

	// --- Storage Layer Providers ---
	storage.NewUserStorage, // Add the new storage provider

	// --- Service Layer Providers ---
	service.NewUserService,

	// --- Handler Layer Providers ---
	handles.NewUserHandler,

	// --- App Provider ---
	NewApp,
)

// --- Provider Implementations ---

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

func provideDiscovery(cfgStore *models.AppConfigStore, inputs ConfigInputs) (*discovery.Discovery, func(), error) {
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		logger.Info("Deregistering service from Consul...", inputs.ServerName)
		discoveryClient.DeRegister(inputs.ServerName)
	}
	return discoveryClient, cleanup, nil
}

func provideTracer(cfg *models.AppConfig, inputs ConfigInputs) (func(), error) {
	if !cfg.Tracing.Enable {
		return func() {}, nil
	}
	shutdown, err := tracing.InitTracerProvider(inputs.ServerName, cfg.Tracing.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tracer: %w", err)
	}
	cleanup := func() {
		logger.Info("Shutting down tracer provider...", inputs.ServerName)
		if err := shutdown(context.Background()); err != nil {
			logger.Error(err, "Failed to shutdown tracer provider", inputs.ServerName)
		}
	}
	return cleanup, nil
}

func provideDatabase(cfg *models.AppConfig) (db.Database, error) {
	dsn := url.URL{
		Scheme: cfg.Database.Type,
		User:   url.UserPassword(cfg.Database.UserName, cfg.Database.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Database.Host, cfg.Database.Port),
		Path:   cfg.Database.Database,
	}
	dbase, err := db.NewEasyDatabaseWithPool(cfg.Database.Type, dsn.String(), cfg.Database)
	if err != nil {
		return nil, err
	}
	if err := dbase.AutoMigrate(&models.User{}); err != nil {
		return nil, err
	}
	return dbase, nil
}

func provideHttpServer(userHandler *handles.UserHandler, healthHandler *HealthHandler, cfg *models.AppConfig, inputs ConfigInputs) *http.Server {
	g := gin.Default()

	if cfg.Tracing.Enable {
		g.Use(otelgin.Middleware(inputs.ServerName))
	}

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

// InitializeApp is the entry point for Wire.
func InitializeApp(inputs ConfigInputs) (*App, func(), error) {
	wire.Build(providerSet)
	return nil, nil, nil
}
