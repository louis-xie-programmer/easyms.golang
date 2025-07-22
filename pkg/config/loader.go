// loader.go 配置加载模块

package config

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
)

// LoadServiceConfig 根据配置类型加载服务配置
func LoadServiceConfig(serviceName, env string) error {
	err := LoadAppConfigStore("configs/app.yaml")
	if err != nil {
		return err
	}
	// 读取app.yaml获取配置类型
	appCfgType := GetAppConfigStore()

	// 根据配置类型加载配置
	if appCfgType.StoreType == "consul" {
		// 创建Consul客户端
		consulClient, err := CreateConsulClient()
		if err != nil {
			return err
		}

		// 从Consul加载配置
		InitConsulConfigProviders(consulClient, GetConsulConfigKey(serviceName, env))
		cps := GetConsulConfigProvider()

		cfgs := make([]AppConfig, 0)
		for _, cp := range cps {
			var cfg AppConfig
			if err := yaml.Unmarshal(cp.Data, &cfg); err != nil {
				log.Printf("[WARN] Failed to unmarshal config: %v", err)
			}
			cfgs = append(cfgs, cfg)
		}
		InitAppConfig(cfgs)

	} else {
		// 构建本地配置路径
		sharePath := fmt.Sprintf("configs/share/%s.yaml", env)
		privatePath := fmt.Sprintf("configs/%s/%s.yaml", serviceName, env)

		// 从本地加载共享配置
		var shareCfg AppConfig
		if shareData, err := ioutil.ReadFile(sharePath); err == nil {
			if err := yaml.Unmarshal(shareData, &shareCfg); err != nil {
				log.Printf("[WARN] Failed to unmarshal local share config: %v", err)
			}
		}

		// 从本地加载私有配置
		privateData, err := ioutil.ReadFile(privatePath)
		if err != nil {
			return fmt.Errorf("failed to read local config: %v", err)
		}

		// 解析私有配置
		var privateCfg AppConfig
		if err := yaml.Unmarshal(privateData, &privateCfg); err != nil {
			return err
		}

		InitAppConfig([]AppConfig{shareCfg, privateCfg})
	}

	return nil
}
