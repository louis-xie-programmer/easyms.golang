// config.go
// 配置管理模块
package config

import (
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/models"
	"fmt"
	"sync"
	"time"

	"dario.cat/mergo"
	"gopkg.in/yaml.v2"
)

var now = time.Now // For testability

// ConfigurationManager 配置管理器，统一管理配置的加载、合并和更新
type ConfigurationManager struct {
	configLock sync.RWMutex
	provider   AppConfigProvider
	watcher    ConfigWatcherInterface
}

// NewConfigurationManager 创建新的配置管理器
func NewConfigurationManager(provider AppConfigProvider) *ConfigurationManager {
	return &ConfigurationManager{
		provider: provider,
	}
}

// LoadConfig 加载配置
func (cm *ConfigurationManager) LoadConfig() error {
	return cm.provider.LoadAppConfig()
}

// GetConfig 获取当前配置
func (cm *ConfigurationManager) GetConfig() *models.AppConfig {
	return GetAppConfig()
}

// UpdateConfig 更新配置
func (cm *ConfigurationManager) UpdateConfig(newConfig *models.AppConfig) {
	SetAppConfig(newConfig)
}

// MergeConfigs 合并配置，将source配置合并到target配置中
func (cm *ConfigurationManager) MergeConfigs(target, source *models.AppConfig) error {
	// Use deep merge to avoid losing partial configurations.
	return mergo.Merge(target, source, mergo.WithOverride)
}

// EnsureBasicConfig 确保配置包含基本项
func (cm *ConfigurationManager) EnsureBasicConfig(config *models.AppConfig, isGateway bool) {
	// 确保至少有基本配置
	if config.Log == (models.LogConfig{}) {
		config.Log = models.LogConfig{
			LogLevel: "info",
			LogType:  "local",
		}
	}

	// 网关服务不需要数据库配置
	if isGateway {
		config.Database = models.DatabaseConfig{}
	}
}

// SaveConfigVersion 保存配置版本
func (cm *ConfigurationManager) SaveConfigVersion(d *discovery.Discovery, serverName, env, description string, isGateway bool) (string, error) {
	// 获取当前配置
	currentConfig := cm.GetConfig()
	if currentConfig == nil {
		return "", fmt.Errorf("current config is nil")
	}

	// 序列化配置
	configData, err := yaml.Marshal(currentConfig)
	if err != nil {
		return "", err
	}

	// 生成版本ID
	versionID := fmt.Sprintf("%s-%d", serverName, getCurrentTimestamp())

	// 创建版本信息
	versionInfo := models.ConfigVersion{
		VersionID:   versionID,
		Timestamp:   now(),
		Description: description,
		ConfigData:  string(configData),
	}

	// 序列化版本信息
	versionData, err := yaml.Marshal(versionInfo)
	if err != nil {
		return "", err
	}

	// 保存到Consul
	key := fmt.Sprintf("easyms/versions/%s/%s/%s", env, serverName, versionID)
	err = d.Put(key, string(versionData))
	if err != nil {
		return "", err
	}

	return versionID, nil
}

// GetConfigVersions 获取配置版本列表
func (cm *ConfigurationManager) GetConfigVersions(d *discovery.Discovery, serverName, env string, isGateway bool) ([]*models.ConfigVersion, error) {
	// 构建键前缀
	prefix := fmt.Sprintf("easyms/versions/%s/%s/", env, serverName)

	// 获取所有版本键
	keys, err := d.ListKeys(prefix)
	if err != nil {
		return nil, err
	}

	// 获取所有版本信息
	versions := make([]*models.ConfigVersion, 0, len(keys))
	for _, key := range keys {
		val, err := d.Get(key)
		if err != nil {
			return nil, err
		}

		var version models.ConfigVersion
		err = yaml.Unmarshal([]byte(val), &version)
		if err != nil {
			return nil, err
		}

		// 验证配置
		var config models.AppConfig
		err = yaml.Unmarshal([]byte(version.ConfigData), &config)
		if err != nil {
			return nil, err
		}

		versions = append(versions, &version)
	}

	return versions, nil
}

// GetConfigVersion retrieves a specific configuration version.
func (cm *ConfigurationManager) GetConfigVersion(d *discovery.Discovery, serverName, env, versionID string) (*models.ConfigVersion, error) {
	// 获取版本信息
	key := fmt.Sprintf("easyms/versions/%s/%s/%s", env, serverName, versionID)
	val, err := d.Get(key)
	if err != nil {
		return nil, err
	}
	if val == "" {
		return nil, fmt.Errorf("version %s not found", versionID)
	}

	var version models.ConfigVersion
	if err := yaml.Unmarshal([]byte(val), &version); err != nil {
		return nil, err
	}
	return &version, nil
}

// RollbackToVersion 回滚到指定版本
func (cm *ConfigurationManager) RollbackToVersion(d *discovery.Discovery, serverName, env, versionID string) error {
	version, err := cm.GetConfigVersion(d, serverName, env, versionID)
	if err != nil {
		return err
	}
	// 将配置数据写入当前配置键
	configKey := fmt.Sprintf("easyms/%s/%s.yaml", env, serverName)
	return d.Put(configKey, version.ConfigData)
}

// ReloadConfig 重新加载配置
func (cm *ConfigurationManager) ReloadConfig() error {
	if cm.provider != nil {
		return cm.provider.LoadAppConfig()
	}
	return fmt.Errorf("no config provider available")
}

// SetProvider 设置配置提供者
func (cm *ConfigurationManager) SetProvider(provider AppConfigProvider) {
	cm.configLock.Lock()
	defer cm.configLock.Unlock()
	cm.provider = provider
}

// getCurrentTimestamp 获取当前时间戳
func getCurrentTimestamp() int64 {
	return now().Unix()
}
