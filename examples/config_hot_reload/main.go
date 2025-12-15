package main

import (
	"fmt"
	"time"

	"easyms/pkg/config"
	"easyms/pkg/discovery"
	"easyms/pkg/entities"
)

func main() {
	// 模拟服务名称和环境
	serverName := "example-svc"
	env := "dev"

	// 连接到Consul
	d, err := discovery.NewDiscovery("localhost:8500")
	if err != nil {
		panic(err)
	}

	// 初始化配置存储
	cfgStore := &entities.AppConfigStore{
		Env:       env,
		StoreType: "consul",
		Consul: entities.ConsulConfig{
			Host:            "localhost:8500",
			KeyPath:         "easyms/%s/%s.yaml",
			ReloadOnChanges: true,
		},
	}

	// 保存配置存储到文件（实际项目中这应该是预设的）
	// 这里仅为演示目的

	// 创建Consul配置提供者
	provider := config.NewConsulConfig(d, serverName, cfgStore.Consul.KeyPath, env)

	// 加载配置
	err = provider.LoadAppConfig()
	if err != nil {
		panic(err)
	}

	// 获取当前配置
	currentConfig := config.GetAppConfig()
	fmt.Printf("初始配置: %+v\n", currentConfig)

	// 模拟配置变更监听
	fmt.Println("监听配置变更中... (请在Consul中修改配置进行测试)")
	fmt.Println("按 Ctrl+C 退出")

	// 保持程序运行以监听配置变更
	for {
		time.Sleep(1 * time.Second)
		// 实际的配置变更会在后台自动处理
	}
}
