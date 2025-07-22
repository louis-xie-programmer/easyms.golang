package config

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"strings"
	"sync"
)

var (
	appConfigStore *AppConfigStore
	appConfig      *AppConfig
	appConfigLock  sync.RWMutex
)

// InitAppConfig 初始化配置
func InitAppConfig(appConfigs []AppConfig) {
	appConfigLock.Lock()
	appConfig = &appConfigs[0]
	for i := 1; i < len(appConfigs); i++ {
		appConfig = DeepMergeConfig(appConfig, &appConfigs[i])
	}
	appConfigLock.Unlock()
}

// GetAppConfig 获取配置
func GetAppConfig() *AppConfig {
	return appConfig
}

// ReInitAppConfig 重新初始化配置
func ReInitAppConfig() {
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
}

// GetLocalConfigFileName 获取本地配置文件名
func GetLocalConfigFileName(server string, env string) string {
	return fmt.Sprintf("configs/%s/%s.yaml", server, env)
}

// GetConsulConfigKey 获取Consul配置的keypath
func GetConsulConfigKey(server string, env string) []string {
	return strings.Split(fmt.Sprintf(appConfigStore.Consul.KeyPath, env, server, env), ",")
}

// LoadAppConfigStore 从指定路径加载app.yaml并解析configuration_type
func LoadAppConfigStore(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg AppConfigStore
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	appConfigStore = &cfg

	return nil
}

// GetAppConfigStore 获取app.yaml配置
func GetAppConfigStore() *AppConfigStore {
	return appConfigStore
}

// DeepMergeConfig 深度合并两个配置对象，私有配置优先
func DeepMergeConfig(target, source *AppConfig) *AppConfig {
	if target == nil || source == nil {
		return target
	}
	merged := &AppConfig{
		ConfigLock: sync.RWMutex{},
	}

	if source.Log != (LogConfig{}) {
		merged.Log = source.Log
	} else if target.Log != (LogConfig{}) {
		merged.Log = target.Log
	}

	if source.Loki != (LokiConfig{}) {
		merged.Loki = source.Loki
	} else if target.Loki != (LokiConfig{}) {
		merged.Loki = target.Loki
	}

	if source.Server != (ServerConfig{}) {
		merged.Server = source.Server
	} else if target.Server != (ServerConfig{}) {
		merged.Server = target.Server
	}

	if source.Database != (DatabaseConfig{}) {
		merged.Database = source.Database
	} else if target.Database != (DatabaseConfig{}) {
		merged.Database = target.Database
	}

	return merged
}
