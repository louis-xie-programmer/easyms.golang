// internal/services/auth/main.go
package main

import (
	"context"
	pb "easyms/api/proto/auth"
	"easyms/internal/services/auth/internal/handles"
	"easyms/internal/services/auth/internal/middleware"
	"easyms/internal/services/auth/internal/service"
	"easyms/internal/services/auth/internal/storage"
	"easyms/internal/shared/config"
	"easyms/internal/shared/db"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/tracing"
	"errors"
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	serverName = "auth-svc"
)

type application struct {
	config         *models.AppConfig
	discovery      *discovery.Discovery
	db             db.Database
	redis          *db.EasyRedis
	tokenGranter   service.TokenGranter
	tokenService   service.TokenService
	clientSvc      service.ClientDetailsService
	userSvc        service.UserDetailsService
	tracerShutdown func(context.Context) error
}

func main() {
	app, err := initDependencies()
	if err != nil {
		log.Fatalf("Failed to initialize dependencies: %v", err)
	}

	if app.discovery != nil {
		grpcPort := app.config.Server.GrpcPort

		healthCheck := &api.AgentServiceCheck{
			TCP:                            fmt.Sprintf("%s:%d", app.config.Server.Host, grpcPort),
			Interval:                       "10s",
			Timeout:                        "5s",
			DeregisterCriticalServiceAfter: "1m",
		}

		err = app.discovery.Register(serverName, app.config.Server.Host, grpcPort, nil, healthCheck)
		if err != nil {
			logger.Error(err, "Failed to register service with consul", serverName)
			panic(err)
		}
	}

	grpcServer, httpServer := startServers(app)

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown
	logger.Info("Shutdown signal received, starting graceful shutdown...", serverName)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var shutdownErr error
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		logger.Info("Shutting down HTTP server...", serverName)
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("http server shutdown failed: %w", err))
		}
	}()

	go func() {
		defer wg.Done()
		logger.Info("Shutting down gRPC server...", serverName)
		grpcServer.GracefulStop()
	}()

	wg.Wait()

	if app.discovery != nil {
		logger.Info("Deregistering service from Consul...", serverName)
		app.discovery.DeRegister(serverName)
	}

	if app.tracerShutdown != nil {
		logger.Info("Shutting down tracer provider...", serverName)
		if err := app.tracerShutdown(shutdownCtx); err != nil {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("tracer shutdown failed: %w", err))
		}
	}

	if shutdownErr != nil {
		logger.Error(shutdownErr, "Errors during shutdown.", serverName)
	} else {
		logger.Info("Graceful shutdown complete.", serverName)
	}
}

func initDependencies() (*application, error) {
	app := &application{}
	var err error

	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		return nil, fmt.Errorf("failed to init config store: %w", err)
	}

	provider := config.NewLocalConfig(serverName, cfgStore.Env)
	if err := provider.LoadAppConfig(); err != nil {
		return nil, fmt.Errorf("failed to load local app config: %w", err)
	}
	app.config = config.GetAppConfig()
	if app.config == nil {
		return nil, errors.New("application config is not loaded")
	}

	logger.Init(serverName, app.config)

	if app.config.Tracing.Enable {
		app.tracerShutdown, err = tracing.InitTracerProvider(serverName, app.config.Tracing.Endpoint)
		if err != nil {
			logger.Error(err, "Failed to initialize tracer provider", serverName)
		}
	}

	app.discovery, err = discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	dsn := url.URL{
		Scheme: app.config.Database.Type,
		User:   url.UserPassword(app.config.Database.UserName, app.config.Database.Password),
		Host:   fmt.Sprintf("%s:%d", app.config.Database.Host, app.config.Database.Port),
		Path:   app.config.Database.Database,
	}
	app.db, err = db.NewEasyDatabaseWithPool(app.config.Database.Type, dsn.String(), app.config.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	app.redis, err = db.NewEasyRedis(&app.config.Cache.Redis.Address, &app.config.Cache.Redis.Password, &app.config.Cache.Redis.DB)
	if err != nil {
		logger.Warn("Redis connection failed, falling back to DB-only mode", "error", err)
		app.redis = nil
	}

	if app.config.OAuth2.JWTSecret == "" {
		return nil, errors.New("JWT secret is not configured")
	}
	var tokenEnhancer storage.TokenEnhancer = storage.NewJwtTokenEnhancer(app.config.OAuth2.JWTSecret)

	tokenStore := storage.NewJwtTokenStore(tokenEnhancer.(*storage.JwtTokenEnhancer), app.db, app.redis)

	app.tokenService = service.NewTokenService(tokenStore, tokenEnhancer)
	app.userSvc = service.NewPostgresUserDetailsService(app.db)
	app.clientSvc = service.NewPostgresClientDetailsService(app.db)
	app.tokenGranter = service.NewComposeTokenGranter(map[string]service.TokenGranter{
		"client_credentials": service.NewClientCredentialsTokenGranter("client_credentials", app.clientSvc, app.tokenService),
		"password":           service.NewUsernamePasswordTokenGranter("password", app.userSvc, app.tokenService),
		"refresh_token":      service.NewRefreshGranter("refresh_token", app.tokenService),
	})

	return app, nil
}

func startServers(app *application) (*grpc.Server, *http.Server) {
	httpPort := app.config.Server.Port
	grpcPort := app.config.Server.GrpcPort

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		logger.Error(err, "Failed to listen for gRPC", serverName)
		panic(err)
	}

	var grpcServer *grpc.Server
	if app.config.Tracing.Enable {
		grpcServer = grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	} else {
		grpcServer = grpc.NewServer()
	}

	pb.RegisterAuthServiceServer(grpcServer, service.NewGrpcServer(app.tokenGranter, app.tokenService, app.clientSvc))

	go func() {
		logger.Info("gRPC server listening", "address", lis.Addr().String())
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error(err, "gRPC server failed to serve", serverName)
		}
	}()

	g := gin.Default()
	if app.config.Tracing.Enable {
		g.Use(otelgin.Middleware(serverName))
	}

	go func() {
		ctx := context.Background()
		mux := runtime.NewServeMux()
		opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		if app.config.Tracing.Enable {
			opts = append(opts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
		}
		err := pb.RegisterAuthServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts)
		if err != nil {
			logger.Error(err, "Failed to register gRPC-Gateway", serverName)
			return
		}
		g.Any("/v1/*any", gin.WrapH(mux))
	}()

	if app.config.Server.Host != "prod" {
		g.StaticFile("/swagger.json", "./api/proto/auth/auth.swagger.json")
		g.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/swagger.json")))
	}

	healthChecker := service.NewHealthCheckerService(app.db)
	g.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, healthChecker.CheckHealth())
	})

	g.POST("/oauth2/token", handles.MakeTokenEndpoint(app.tokenGranter, app.clientSvc))
	g.POST("/oauth2/client-refresh", middleware.MakeSimpleClientMiddleware(app.tokenService), handles.RefreshTokenEndpoint(app.tokenService))
	g.POST("/login", middleware.MakeSimpleClientMiddleware(app.tokenService), handles.LoginEndPoint(app.userSvc, app.tokenService))
	g.POST("/oauth2/refresh", middleware.MakeAuthorityAuthorizationMiddleware(app.tokenService), handles.RefreshTokenEndpoint(app.tokenService))
	g.POST("/oauth2/verify", handles.VerifyTokenEndpoint(app.tokenService))
	g.POST("/client/register", handles.RegisterClientEndPoint(app.clientSvc))
	g.POST("/user/register", middleware.MakeSimpleClientMiddleware(app.tokenService), handles.RegisterUserEndPoint(app.userSvc, []string{"admin"}))
	g.POST("/admin", middleware.MakeAuthorityAuthorizationMiddleware(app.tokenService), middleware.MakeScopeHandler("admin"), func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"message": "Hello Admin!"})
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", httpPort),
		Handler: g,
	}

	go func() {
		logger.Info("HTTP server listening", "address", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(err, "HTTP server failed to serve", serverName)
		}
	}()

	return grpcServer, httpServer
}
