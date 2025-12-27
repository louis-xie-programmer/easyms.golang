package main

import (
	"context"
	"easyms/internal/shared/config"
	"easyms/internal/shared/logger"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	serverName := "user-svc"
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	inputs := ConfigInputs{
		ServerName: serverName,
		Env:        env,
	}

	// 使用 Wire 进行依赖注入和应用初始化
	app, cleanup, err := InitializeApp(inputs)
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}
	defer cleanup()

	// 初始化日志系统
	appConfig := config.GetAppConfig()
	logger.Init(serverName, appConfig)

	// 注册服务到 Consul
	if app.ds != nil {
		err = app.ds.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil, nil) // Using default health check for now
		if err != nil {
			logger.Error(err, "Failed to register service with consul", serverName)
			panic(err)
		}
	}

	// --- 启动并管理服务器生命周期 ---
	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("Starting server", "address", app.httpServer.Addr)
		serverErrors <- app.httpServer.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error(err, "Server failed to start or encountered a runtime error", serverName)
		}
	case sig := <-shutdown:
		logger.Info("Shutdown signal received", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := app.httpServer.Shutdown(ctx); err != nil {
			logger.Error(err, "Graceful shutdown failed", serverName)
		} else {
			logger.Info("Server shutdown gracefully", serverName)
		}
	}
}
