// config.go
// 配置管理模块
package config

import (
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/entities"
	"fmt"
	"log"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"
)

type ConsulConfig struct {
	Client     *discovery.Discovery
	ServerName string
	Env        string
	AppKeyPath string
	mutex      sync.Mutex
	configMgr  *ConfigurationManager
}

// NewConsulConfig 创建一个新的Consul配置提供者
func NewConsulConfig(client *discovery.Discovery, serviceName, keyPath, env string) AppConfigProvider {
	if serviceName == "" {
		serviceName = "gateway"
	}
	if env == "" {
		env = "dev"
	}
	return &ConsulConfig{
		Client:     client,
		ServerName: serviceName,
		Env:        env,
		AppKeyPath: keyPath,
		configMgr:  NewConfigurationManager(nil),
	}
}

func (cc *ConsulConfig) OnChange() func(newConfig *entities.AppConfig) {
	// 更新配置
	return func(newConfig *entities.AppConfig) {
		cc.configMgr.UpdateConfig(newConfig)
	}
}

// LoadAppConfig 从Consul加载配置并合并
func (cc *ConsulConfig) LoadAppConfig() error {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// consul appConfig
	appKeys := GetConsulAppConfigKey(cc.AppKeyPath, cc.ServerName, cc.Env)

	newConfig := &entities.AppConfig{}
	for _, key := range appKeys {
		val, err := cc.Client.Get(key)
		if err != nil {
			return err
		}
		if val == "" {
			return fmt.Errorf("key not found: %s", key)
		}
		var cfg entities.AppConfig
		// 解析配置
		err = yaml.Unmarshal([]byte(val), &cfg)
		if err != nil {
			return err
		}

		// 深度合并配置
		if err := cc.configMgr.MergeConfigs(newConfig, &cfg); err != nil {
			log.Printf("Failed to merge config from key %s: %v", key, err)
			// Decide if you want to continue or return an error
			return fmt.Errorf("failed to merge config from key %s: %w", key, err)
		}
	}

	// Update the config in the manager, which in turn updates the global config
	// This ensures thread-safe update.
	cc.configMgr.UpdateConfig(newConfig)
	globalAppConfig = newConfig // This should be updated via the manager

	return nil
}

func GetConsulAppConfigKey(keyPath string, serverName string, env string) []string {
	// 解析 keyPath 中的变量
	keyPath = strings.ReplaceAll(keyPath, "${env}", env)
	keyPath = strings.ReplaceAll(keyPath, "${server_name}", serverName)
	return strings.Split(keyPath, ";")
}
