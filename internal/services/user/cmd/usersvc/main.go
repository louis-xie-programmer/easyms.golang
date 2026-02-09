// Package main 是用户服务 (user-svc) 的主程序入口。
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
		env = "dev" // 如果未设置 APP_ENV 环境变量，则默认为 "dev"
	}

	// 准备传递给 Wire 的配置输入
	inputs := ConfigInputs{
		ServerName: serverName,
		Env:        env,
	}

	// 使用 Wire 初始化应用的所有依赖项。
	// InitializeApp 函数由 Wire 自动生成 (在 wire_gen.go 中)。
	// 它会返回一个包含所有已初始化组件的 App 实例，以及一个用于资源清理的 cleanup 函数。
	app, cleanup, err := InitializeApp(inputs)
	if err != nil {
		log.Fatalf("初始化应用失败: %v", err)
	}
	defer cleanup() // 确保在 main 函数退出时执行所有清理操作 (如关闭数据库连接、注销服务等)

	// 初始化日志系统
	appConfig := config.GetAppConfig()
	logger.Init(serverName, appConfig)

	// 将服务注册到 Consul
	if app.ds != nil {
		err = app.ds.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil, nil)
		if err != nil {
			// 如果服务注册失败，这是一个严重错误，服务无法正常工作，因此直接 panic。
			logger.Error(err, "将服务注册到 Consul 失败", serverName)
			panic(err)
		}
	}

	// --- 启动并管理服务器的生命周期 ---
	serverErrors := make(chan error, 1)

	// 在一个 goroutine 中启动 HTTP 服务器
	go func() {
		logger.Info("启动服务器", "address", app.httpServer.Addr)
		serverErrors <- app.httpServer.ListenAndServe()
	}()

	// 监听操作系统的中断信号 (SIGINT, SIGTERM)
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞主 goroutine，直到接收到服务器错误或关闭信号
	select {
	case err := <-serverErrors:
		// 如果服务器启动失败或运行时出错
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error(err, "服务器启动失败或遇到运行时错误", serverName)
		}
	case sig := <-shutdown:
		// 如果接收到关闭信号
		logger.Info("接收到关闭信号", "signal", sig)

		// 创建一个带有超时的上下文，用于优雅关闭
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 执行 HTTP 服务器的优雅关闭
		if err := app.httpServer.Shutdown(ctx); err != nil {
			logger.Error(err, "服务器优雅关闭失败", serverName)
		} else {
			logger.Info("服务器已优雅关闭", serverName)
		}
	}
}
