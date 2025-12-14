// main.go API网关服务主程序
// 主要功能：
// 1. 初始化服务配置
// 2. 启动服务发现客户端
// 3. 监听目标服务
// 4. 启动HTTP服务
// 5. 提供反向代理和负载均衡功能
package main

import (
	"easyms/pkg/config"
	"easyms/pkg/discovery"
	"easyms/pkg/gateway"
	"easyms/pkg/logger"
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
		logger.Error(err, "Failed to initialize app config store", serverName, nil)
	}

	// 初始化 Consul 服务发现客户端
	// 连接到Consul服务注册与发现中心
	sd, err := discovery.NewServiceDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName, nil)
	}

	// 监听多个服务的变化
	// 启动goroutine监听user-svc和auth-svc服务实例变化
	sd.WatchService("user-svc")
	sd.WatchService("auth-svc")

	// 创建API网关实例
	// 初始化网关，传入服务发现客户端
	gw := gateway.NewGateway(sd)

	// 启动HTTP服务
	// 启动网关HTTP服务，监听指定端口
	fmt.Printf("Starting %s on port %d\n", serverName, port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), gw); err != nil {
		fmt.Printf("Failed to start %s: %v\n", serverName, err)
	}
}
