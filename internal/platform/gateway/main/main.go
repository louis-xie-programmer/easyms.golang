// main.go API网关服务主程序
// 主要功能：
// 1. 初始化服务配置
// 2. 启动服务发现客户端
// 3. 监听目标服务
// 4. 启动HTTP服务
// 5. 提供反向代理和负载均衡功能
package main

import (
	"easyms/internal/platform/gateway"
	"easyms/internal/shared/config"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"fmt"
	"net/http"
)

// main API网关服务入口函数
func main() {
	// 获取环境变量
	// 网关服务名称和端口
	serverName := "gateway"
	port := 10000

	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		logger.Error(err, "Failed to initialize app config store", serverName)
	}

	// 初始化 Consul 服务发现客户端
	// 连接到Consul服务注册与发现中心
	sd, err := discovery.NewServiceDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName)
	}

	// 监听多个服务的变化
	// 启动goroutine监听user-svc和auth-svc服务实例变化
	sd.WatchService("user-svc")
	sd.WatchService("auth-svc")
	sd.WatchService("order-svc") // 新增对订单服务的监听

	// 创建 Discovery 客户端用于配置加载
	discoveryClient, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul discovery client", serverName)
	}

	var provider config.AppConfigProvider

	// 加载服务配置
	// 根据配置类型（本地或Consul）加载服务配置
	// 加载应用配置
	if cfgStore.StoreType == "consul" {
		// 使用Consul配置提供者
		provider = config.NewConsulConfig(discoveryClient, serverName, cfgStore.Consul.KeyPath, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			logger.Error(err, "Failed to load app config", serverName)
			panic(err)
		}
		// 动态监听配置文件并更新服务
		watch := config.NewConfigWatcher(discoveryClient, cfgStore.Consul.KeyPath, serverName, cfgStore.Env, provider.OnChange())
		go watch.Start()
	} else {
		// 使用本地配置提供者
		// 从本地配置文件加载配置
		provider = config.NewLocalConfig(serverName, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			logger.Error(err, "Failed to load local app config", serverName)
			panic(err)
		}
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()

	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	// 创建API网关实例
	// 初始化网关，传入服务发现客户端
	gw := gateway.NewGateway(sd)

	// 更新网关配置
	gw.LoadConfig("./internal/platform/gateway/internal/configs/gateway.yaml")

	// 启动HTTP服务
	// 启动网关HTTP服务，监听指定端口
	logger.Info("Starting gateway server", serverName, "port", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), gw); err != nil {
		logger.Error(err, "Failed to start gateway server", serverName)
	}
}
