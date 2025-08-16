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

	err := config.LoadServiceConfig("server2", env)
	if err != nil {
		logger.Error(err, "Failed to load service config", "server2", nil)
	}
	appConfig := config.GetAppConfig()
	logger.Init("server2", appConfig)

	// Consul注册
	port := 10003
	consulHost := "localhost:8500"
	if configStore := config.GetAppConfigStore(); configStore != nil && configStore.Consul.Host != "" {
		consulHost = configStore.Consul.Host
	}
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = consulHost
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		logger.Error(err, "Consul client error", "server2", nil)
	} else {
		registration := &consulapi.AgentServiceRegistration{
			ID:      fmt.Sprintf("server2-%d", port),
			Name:    "server2",
			Address: appConfig.Server.Host,
			Port:    port,
			Check: &consulapi.AgentServiceCheck{
				HTTP:     fmt.Sprintf("http://%s:%d/health", appConfig.Server.Host, port),
				Interval: "10s",
				Timeout:  "5s",
			},
		}
		err = consulClient.Agent().ServiceRegister(registration)
		if err != nil {
			logger.Error(err, "Consul register error", "server2", nil)
		} else {
			logger.Info("Consul service registered", "server2", nil)
		}
	}

	r := gin.Default()
	r.GET("/hello", func(c *gin.Context) {
		c.JSON(200, gin.H{"msg": "Hello from server2"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	logger.Warn(fmt.Sprintf("Server2 started on port %d", port), "server2", nil)
	err = r.Run(fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Error(err, "Gin server error", "server2", nil)
	}
	logger.Shutdown()

}
