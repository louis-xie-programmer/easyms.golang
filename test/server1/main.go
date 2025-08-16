// main.go 程序入口点
// 包含服务启动和初始化逻辑
package main

import (
	"easyms/pkg/config"
	"easyms/pkg/logger"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	consulapi "github.com/hashicorp/consul/api"
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

	// Consul注册
	// 获取 Consul 地址
	consulHost := "localhost:8500"
	if configStore := config.GetAppConfigStore(); configStore != nil && configStore.Consul.Host != "" {
		consulHost = configStore.Consul.Host
	}
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = consulHost
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		logger.Error(err, "Consul client error", "server1", nil)
	} else {
		registration := &consulapi.AgentServiceRegistration{
			ID:      fmt.Sprintf("server1-%d", appConfig.Server.Port),
			Name:    "server1",
			Address: appConfig.Server.Host,
			Port:    appConfig.Server.Port,
			Check: &consulapi.AgentServiceCheck{
				HTTP:     fmt.Sprintf("http://%s:%d/health", appConfig.Server.Host, appConfig.Server.Port),
				Interval: "10s",
				Timeout:  "5s",
			},
		}
		err = consulClient.Agent().ServiceRegister(registration)
		if err != nil {
			logger.Error(err, "Consul register error", "server1", nil)
		} else {
			logger.Info("Consul service registered", "server1", nil)
		}
	}

	r := gin.Default()
	r.GET("/hello", func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "Hello from server1"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	logger.Warn(fmt.Sprintf("Server1 started on port %d", appConfig.Server.Port), "server1", nil)
	err = r.Run(fmt.Sprintf(":%d", appConfig.Server.Port))
	if err != nil {
		logger.Error(err, "Gin server error", "server1", nil)
	}
	logger.Shutdown()
}
