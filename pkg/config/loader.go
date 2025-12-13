// loader.go 配置加载模块
// 主要职责：
// - 解析app.yaml确定配置源类型
// - 统一入口加载Consul/本地配置
// - 多层级配置合并（共享+私有）
// - 初始化全局配置对象
// - 提供配置加载的统一入口
package config

import (
	"easyms/pkg/discovery"
	"easyms/pkg/entitis"
)

// LoadServiceConfig 根据配置类型加载服务配置
// 支持本地配置文件和Consul配置中心两种方式
// 参数:
//   - client: Consul 客户端实例
//   - keyPath: Consul 键路径
//   - storeType: 存储类型（local 或 consul）
//   - serviceName: 服务名称
//   - env: 环境标识（dev/prod）
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func LoadServiceConfig(client *discovery.Discovery, keyPath string, storeType string, serviceName, env string) error {
	// 确保全局配置对象已初始化
	if appConfig == nil {
		appConfig = &entitis.AppConfig{}
	}
	
	// 根据配置类型加载配置
	// 支持Consul配置中心和本地配置文件两种方式
	if storeType == "consul" && client != nil {
		// 使用 Consul 配置提供者
		// 从Consul配置中心加载配置
		provider := NewConsulConfig(client, serviceName, keyPath, env)
		err := provider.LoadAppConfig()
		if err != nil {
			return err
		}
	} else {
		// 使用本地配置提供者
		// 从本地配置文件加载配置
		provider := NewLocalConfig(serviceName, env)
		err := provider.LoadAppConfig()
		if err != nil {
			return err
		}
	}

	return nil
}