package config

import (
	"easyms/pkg/discovery"
	"easyms/pkg/entities"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"
)

type ConsulConfig struct {
	Client     *discovery.Discovery
	ServerName string
	Env        string
	AppKeyPath string
	watcher    *ConfigWatcher
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

// LoadAppConfig 从Consul加载配置并合并
func (cc *ConsulConfig) LoadAppConfig() error {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// consul appConfig
	appKeys := GetConsulAppConfigKey(cc.AppKeyPath, cc.ServerName, cc.Env)

	currentAppConfig := cc.configMgr.GetConfig()
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
		cc.configMgr.MergeConfigs(currentAppConfig, &cfg)
	}

	// 确保至少有基本配置
	isGateway := cc.ServerName == "gateway"
	cc.configMgr.EnsureBasicConfig(currentAppConfig, isGateway)

	// 确保至少有基本配置
	cc.configMgr.EnsureBasicConfig(currentAppConfig, isGateway)

	// 更新全局配置
	globalAppConfig = currentAppConfig

	// 验证配置的有效性
	if err := globalAppConfig.Validate(isGateway); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return nil
}

// startWatcher 启动配置监听器
func (cc *ConsulConfig) startWatcher() {
	if cc.watcher != nil {
		// 如果监听器已经存在，先停止它
		cc.watcher.Stop()
	}

	// 创建新的监听器
	cc.watcher = NewConfigWatcher(cc.Client, cc.AppKeyPath, cc.ServerName, cc.Env, cc.onConfigChange)

	// 启动监听器
	cc.watcher.Start()
}

// onConfigChange 配置变更回调函数
func (cc *ConsulConfig) onConfigChange(newConfig *entities.AppConfig) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// 更新全局配置
	globalAppConfig = newConfig

	fmt.Println("Configuration reloaded successfully")
}

func GetConsulAppConfigKey(keyPath string, serverName string, env string) []string {
	return strings.Split(fmt.Sprintf(keyPath, env, env), "")
}
