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
	serverName := "order-svc"
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	// 1. 使用手动 DI 初始化所有组件
	app, cleanup, err := InitializeApp(serverName, env)
	if err != nil {
		// 日志系统可能还未初始化
		log.Fatalf("Failed to initialize app: %v", err)
	}
	// 确保所有清理操作在 main 退出时执行
	defer cleanup()

	// 获取已加载的配置
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		panic("Application config is not loaded")
	}

	// 2. 启动后台服务和进行服务注册
	if app.ds != nil {
		if err := app.ds.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil, nil); err != nil {
			logger.Error(err, "Failed to register service", serverName)
			panic(err)
		}
	}
	app.relayService.Start()
	logger.Info("Outbox relay service started", serverName)

	// 3. 启动并管理 HTTP 服务器生命周期
	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("Starting HTTP server", serverName, "address", app.httpServer.Addr)
		serverErrors <- app.httpServer.ListenAndServe()
	}()

	// 4. 等待关闭信号
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

		// 优雅地关闭 HTTP 服务器
		if err := app.httpServer.Shutdown(ctx); err != nil {
			logger.Error(err, "Graceful shutdown failed", serverName)
		} else {
			logger.Info("Server shutdown gracefully", serverName)
		}
	}
}
