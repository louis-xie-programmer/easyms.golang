package main

import (
	"easyms/internal/shared/config"
	"easyms/internal/shared/logger"
	"fmt"
)

func main() {
	serverName := "user-svc"
	env := "dev" // 假设环境为 dev

	// 创建并传入 ConfigInputs
	inputs := ConfigInputs{
		ServerName: serverName,
		Env:        env,
	}

	// 使用 Wire 进行依赖注入和应用初始化
	app, cleanup, err := InitializeApp(inputs)
	if err != nil {
		// 在这个阶段，日志系统还未初始化，所以只能用 fmt
		fmt.Printf("Failed to initialize app: %v\n", err)
		panic(err)
	}
	// 使用 defer 来确保在 main 函数退出时执行清理操作
	defer cleanup()

	// 获取应用配置
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		panic("Application config is not loaded")
	}

	// 在所有依赖都成功构建后，再初始化日志系统
	logger.Init(serverName, appConfig)

	// 启动 HTTP 服务
	logger.Info("Starting server", serverName, "port", appConfig.Server.Port)
	if err := app.engine.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName)
	}
}
