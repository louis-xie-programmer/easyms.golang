package main

import (
	"easyms/internal/shared/config"
	"easyms/internal/shared/logger"
	"fmt"
)

func main() {
	serverName := "order-svc"

	// 手动进行依赖注入和应用初始化
	app, cleanup, err := InitializeApp(serverName, "dev") // 假设环境为 dev
	if err != nil {
		// 如果日志系统已经初始化，就用日志记录；否则，用 fmt
		if logger.IsInitialized() {
			logger.Error(err, "Failed to initialize app", serverName)
		} else {
			fmt.Printf("Failed to initialize app: %v\n", err)
		}
		panic(err)
	}
	// 使用 defer 来确保在 main 函数退出时执行清理操作
	defer cleanup()

	// 获取应用配置
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		// 理论上 InitializeApp 成功后这里不会是 nil，但作为防御性编程保留
		panic("Application config is not loaded")
	}

	// 启动 HTTP 服务
	logger.Info("Starting server", serverName, "port", appConfig.Server.Port)
	if err := app.engine.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName)
	}
}
