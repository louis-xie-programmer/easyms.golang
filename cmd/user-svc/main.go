// main.go 用户服务主程序
// 主要功能：
// 1. 初始化服务配置
// 2. 启动服务发现客户端
// 3. 注册服务到Consul
// 4. 启动HTTP服务
package main

import (
	"easyms/pkg/config"
	"easyms/pkg/discovery"
	"easyms/pkg/logger"
	"fmt"
	"github.com/gin-gonic/gin"
)

// main 用户服务入口函数
// 初始化配置、服务发现客户端，注册服务并启动HTTP服务
func main() {
	// 获取环境变量
	// 用户服务名称
	serverName := "user-svc"

	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		fmt.Println("Failed to initialize app config store")
		logger.Error(err, "Failed to initialize app config store", serverName, nil)
	}

	// 初始化 Consul 服务发现客户端
	// 连接到Consul服务注册与发现中心
	d, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName, nil)
	}

	// 加载服务配置
	// 根据配置类型（本地或Consul）加载服务配置
	err = config.LoadServiceConfig(d, cfgStore.Consul.KeyPath, cfgStore.StoreType, serverName, cfgStore.Env)
	if err != nil {
		fmt.Printf("Failed to load service config %s: %v\n", cfgStore, err)
		logger.Error(err, "Failed to load service config", serverName, nil)
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()
	
	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	// 注册服务到Consul
	// 将当前服务注册到Consul服务注册中心
	err = d.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil)
	if err != nil {
		panic(err)
	}
	
	// 延迟注销服务
	// 确保服务在退出时从Consul中注销
	defer d.DeRegister(serverName)

	// 启动 HTTP 服务
	// 使用Gin框架启动HTTP服务
	g := gin.Default()
	
	// 健康检查端点
	// 提供健康检查接口，供Consul等监控系统使用
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})
	
	// 用户服务接口
	// 提供用户相关信息的接口
	g.GET("/api/user/:id", func(c *gin.Context) {
		c.String(200, "user id: "+c.Param("id"))
	})

	// 启动HTTP服务
	fmt.Printf("Starting %s on port %d\n", serverName, appConfig.Server.Port)
	if err = g.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName, nil)
	}
}