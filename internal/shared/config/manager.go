package config

import (
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/entities"
	"fmt"
	"gopkg.in/yaml.v2"
	"sync"
	"time"
)

// ConfigurationManager 配置管理器，统一管理配置的加载、合并和更新
type ConfigurationManager struct {
	appConfig  *entities.AppConfig
	configLock sync.RWMutex
	provider   AppConfigProvider
	watcher    ConfigWatcherInterface
}

// NewConfigurationManager 创建新的配置管理器
func NewConfigurationManager(provider AppConfigProvider) *ConfigurationManager {
	return &ConfigurationManager{
		appConfig: &entities.AppConfig{},
		provider:  provider,
	}
}

// LoadConfig 加载配置
func (cm *ConfigurationManager) LoadConfig() error {
	return cm.provider.LoadAppConfig()
}

// GetConfig 获取当前配置
func (cm *ConfigurationManager) GetConfig() *entities.AppConfig {
	cm.configLock.RLock()
	defer cm.configLock.RUnlock()
	return cm.appConfig
}

// UpdateConfig 更新配置
func (cm *ConfigurationManager) UpdateConfig(newConfig *entities.AppConfig) {
	cm.configLock.Lock()
	defer cm.configLock.Unlock()
	cm.appConfig = newConfig
}

// MergeConfigs 合并配置，将source配置合并到target配置中
func (cm *ConfigurationManager) MergeConfigs(target, source *entities.AppConfig) {
	if source.Log != (entities.LogConfig{}) {
		target.Log = source.Log
	}
	if source.Loki != (entities.LokiConfig{}) {
		target.Loki = source.Loki
	}
	if source.Server != (entities.ServerConfig{}) {
		target.Server = source.Server
	}
	if source.Database != (entities.DatabaseConfig{}) {
		target.Database = source.Database
	}
	if source.RateLimit != nil {
		target.RateLimit = source.RateLimit
	}
	if source.OAuth2 != (entities.OAuth2Config{}) {
		target.OAuth2 = source.OAuth2
	}
}

// EnsureBasicConfig 确保配置包含基本项
func (cm *ConfigurationManager) EnsureBasicConfig(config *entities.AppConfig, isGateway bool) {
	// 确保至少有基本配置
	if config.Log == (entities.LogConfig{}) {
		config.Log = entities.LogConfig{
			LogLevel: "info",
			LogType:  "zerolog",
		}
	}

	// 网关服务不需要数据库配置
	if isGateway {
		config.Database = entities.DatabaseConfig{}
	}
}

// SaveConfigVersion 保存配置版本
func (cm *ConfigurationManager) SaveConfigVersion(d *discovery.Discovery, serverName, env, description string, isGateway bool) (string, error) {
	// 获取当前配置
	currentConfig := cm.GetConfig()
	if currentConfig == nil {
		return "", fmt.Errorf("current config is nil")
	}

	// 验证配置
	if err := currentConfig.Validate(isGateway); err != nil {
		return "", fmt.Errorf("config validation failed: %w", err)
	}

	// 序列化配置
	configData, err := yaml.Marshal(currentConfig)
	if err != nil {
		return "", err
	}

	// 生成版本ID
	versionID := fmt.Sprintf("%s-%d", serverName, getCurrentTimestamp())

	// 创建版本信息
	versionInfo := entities.ConfigVersion{
		VersionID:   versionID,
		Timestamp:   getCurrentTime(),
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
func (cm *ConfigurationManager) GetConfigVersions(d *discovery.Discovery, serverName, env string, isGateway bool) ([]*entities.ConfigVersion, error) {
	// 构建键前缀
	prefix := fmt.Sprintf("easyms/versions/%s/%s/", env, serverName)

	// 获取所有版本键
	keys, err := d.ListKeys(prefix)
	if err != nil {
		return nil, err
	}

	// 获取所有版本信息
	versions := make([]*entities.ConfigVersion, 0, len(keys))
	for _, key := range keys {
		val, err := d.Get(key)
		if err != nil {
			return nil, err
		}

		var version entities.ConfigVersion
		err = yaml.Unmarshal([]byte(val), &version)
		if err != nil {
			return nil, err
		}

		// 验证配置
		var config entities.AppConfig
		err = yaml.Unmarshal([]byte(version.ConfigData), &config)
		if err != nil {
			return nil, err
		}

		if err := config.Validate(isGateway); err != nil {
			return nil, fmt.Errorf("config validation failed for version %s: %w", version.VersionID, err)
		}

		versions = append(versions, &version)
	}

	return versions, nil
}

// RollbackToVersion 回滚到指定版本
func (cm *ConfigurationManager) RollbackToVersion(d *discovery.Discovery, serverName, env, versionID string) error {
	// 获取版本信息
	key := fmt.Sprintf("easyms/versions/%s/%s/%s", env, serverName, versionID)
	val, err := d.Get(key)
	if err != nil {
		return err
	}

	var version entities.ConfigVersion
	err = yaml.Unmarshal([]byte(val), &version)
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
	return getCurrentTime().Unix()
}

// getCurrentTime 获取当前时间
func getCurrentTime() time.Time {
	return time.Now()
}
