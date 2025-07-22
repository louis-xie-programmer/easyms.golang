// main.go 程序入口点
// 包含服务启动和初始化逻辑
package main

import (
	"easyms/pkg/config"
	"easyms/pkg/logger"
	"fmt"
	"os"
)

func main() {
	// 获取环境变量
	env := os.Getenv("ENV")
	if env == "" {
		env = "dev"
	}

	err := config.LoadServiceConfig("server1", env)
	if err != nil {
		logger.Error(err, "Failed to load service config", "server1", nil)
		//log.Fatalf("[FATAL] Failed to load service config: %v", err)
	}

	appConfig := config.GetAppConfig()

	logger.Init("server1", appConfig)

	print(appConfig)

	logger.Warn(fmt.Sprintf("Server started with config: %s", "server1"), "server1", nil)
	logger.Warn(fmt.Sprintf("Server emd with config: %s", "server1"), "server1", nil)

	select {}

	logger.Shutdown()
}
