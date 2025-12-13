package config

import (
	"easyms/pkg/discovery"
	"easyms/pkg/entitis"
	"fmt"
	"gopkg.in/yaml.v2"
	"time"
	"reflect"
)

// ConfigWatcher 配置监听器
// 用于监听Consul中配置的变化并触发更新
type ConfigWatcher struct {
	client      *discovery.Discovery
	keyPath     string
	serverName  string
	env         string
	onChange    func(*entitis.AppConfig)
	stopCh      chan struct{}
	ticker      *time.Ticker
}

// NewConfigWatcher 创建一个新的配置监听器
func NewConfigWatcher(
	client *discovery.Discovery,
	keyPath, serverName, env string,
	onChange func(*entitis.AppConfig)) *ConfigWatcher {
	return &ConfigWatcher{
		client:     client,
		keyPath:    keyPath,
		serverName: serverName,
		env:        env,
		onChange:   onChange,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动配置监听器
func (cw *ConfigWatcher) Start() {
	// 创建定时器，每隔10秒检查一次配置变化
	cw.ticker = time.NewTicker(10 * time.Second)
	
	go func() {
		// 立即检查一次
		cw.checkConfigChange()
		
		for {
			select {
			case <-cw.ticker.C:
				cw.checkConfigChange()
			case <-cw.stopCh:
				return
			}
		}
	}()
}

// Stop 停止配置监听器
func (cw *ConfigWatcher) Stop() {
	close(cw.stopCh)
	if cw.ticker != nil {
		cw.ticker.Stop()
	}
}

// checkConfigChange 检查配置变化
func (cw *ConfigWatcher) checkConfigChange() {
	// 获取当前配置键列表
	appKeys := GetConsulAppConfigKey(cw.keyPath, cw.serverName, cw.env)
	
	// 临时存储新配置
	newConfig := &entitis.AppConfig{}
	
	// 遍历所有配置键
	for _, key := range appKeys {
		val, err := cw.client.Get(key)
		if err != nil {
			fmt.Printf("Failed to get config from Consul: %v\n", err)
			continue
		}
		if val == "" {
			fmt.Printf("Config key not found: %s\n", key)
			continue
		}
		
		var cfg entitis.AppConfig
		// 解析配置
		err = yaml.Unmarshal([]byte(val), &cfg)
		if err != nil {
			fmt.Printf("Failed to unmarshal config: %v\n", err)
			continue
		}
		
		// 深度合并配置
		if cfg.Log != (entitis.LogConfig{}) {
			newConfig.Log = cfg.Log
		}
		if cfg.Loki != (entitis.LokiConfig{}) {
			newConfig.Loki = cfg.Loki
		}
		if cfg.Server != (entitis.ServerConfig{}) {
			newConfig.Server = cfg.Server
		}
		if cfg.Database != (entitis.DatabaseConfig{}) {
			newConfig.Database = cfg.Database
		}
		if cfg.RateLimit != nil {
			newConfig.RateLimit = cfg.RateLimit
		}
	}
	
	// 如果配置发生了变化，则触发变更回调
	if cw.isConfigChanged(newConfig) {
		fmt.Println("Config changed, triggering update...")
		cw.onChange(newConfig)
	}
}

// isConfigChanged 检查配置是否发生变化
func (cw *ConfigWatcher) isConfigChanged(newConfig *entitis.AppConfig) bool {
	currentConfig := GetAppConfig()
	if currentConfig == nil {
		return true
	}
	
	// 比较配置是否发生变化
	return !reflect.DeepEqual(currentConfig, newConfig)
}