// watcher.go
package config

import (
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/models"
	"fmt"
	"reflect"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

// ConfigWatcher 配置监听器
// 用于监听Consul中配置的变化并触发更新
type ConfigWatcher struct {
	client      *discovery.Discovery
	keyPath     string
	serverName  string
	env         string
	onChange    func(*models.AppConfig)
	stopCh      chan struct{}
	ticker      *time.Ticker
	mu          sync.RWMutex
	lastConfigs map[string]string // 存储上次配置值，用于比较变化
	configMgr   *ConfigurationManager
}

// ConfigWatcherInterface 配置监听器接口
type ConfigWatcherInterface interface {
	Start()
	Stop()
}

// NewConfigWatcher 创建一个新的配置监听器
func NewConfigWatcher(
	client *discovery.Discovery,
	keyPath, serverName, env string,
	onChange func(*models.AppConfig)) *ConfigWatcher {
	return &ConfigWatcher{
		client:      client,
		keyPath:     keyPath,
		serverName:  serverName,
		env:         env,
		onChange:    onChange,
		stopCh:      make(chan struct{}),
		lastConfigs: make(map[string]string),
		configMgr:   NewConfigurationManager(nil),
	}
}

// Start 启动配置监听器
func (cw *ConfigWatcher) Start() {
	// 创建定时器，每隔5秒检查一次配置变化（比原来更频繁以提高响应速度）
	cw.ticker = time.NewTicker(5 * time.Second)

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
	newConfig := &models.AppConfig{}

	// 是否有配置发生变化
	configChanged := false

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

		// 检查配置值是否有变化
		cw.mu.RLock()
		lastVal, exists := cw.lastConfigs[key]
		cw.mu.RUnlock()

		if !exists || lastVal != val {
			configChanged = true
			cw.mu.Lock()
			cw.lastConfigs[key] = val
			cw.mu.Unlock()

			fmt.Printf("Detected config change for key: %s\n", key)
		}
	}

	// 如果配置发生了变化
	if configChanged {
		// 重新加载所有配置以确保正确的覆盖顺序
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

			var cfg models.AppConfig
			// 解析配置
			err = yaml.Unmarshal([]byte(val), &cfg)
			if err != nil {
				fmt.Printf("Failed to unmarshal config: %v\n", err)
				continue
			}

			// 深度合并配置
			cw.configMgr.MergeConfigs(newConfig, &cfg)
		}

		fmt.Println("Configuration reloaded successfully")

		// 触发配置变更回调
		if cw.onChange != nil {
			cw.onChange(newConfig)
		}
	}
}

// isConfigChanged 检查配置是否发生变化
func (cw *ConfigWatcher) isConfigChanged(newConfig *models.AppConfig) bool {
	currentConfig := GetAppConfig()
	if currentConfig == nil {
		return true
	}

	// 比较配置是否发生变化
	return !reflect.DeepEqual(currentConfig, newConfig)
}

// ForceReload 强制重新加载配置
func (cw *ConfigWatcher) ForceReload() {
	fmt.Println("Forcing config reload...")
	cw.checkConfigChange()
}
