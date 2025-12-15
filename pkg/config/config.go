// config.go
// 配置管理模块
// 主要功能：
// 1. 初始化应用配置存储
// 2. 提供全局配置对象访问接口
// 3. 定义配置提供者接口
// 4. 提供配置加载和管理功能
package config

import (
	"easyms/pkg/discovery"
	"easyms/pkg/entities"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"time"
)

var (
	// 全局配置对象，用于存储应用的所有配置信息
	// 通过GetAppConfig函数访问，确保线程安全
	globalAppConfig *entities.AppConfig
)

// AppConfigProvider 配置提供者接口
// 定义了加载应用配置的方法规范
// 不同的配置源（如本地文件、Consul）需要实现此接口
type AppConfigProvider interface {
	// LoadAppConfig 加载并初始化配置
	LoadAppConfig() error
}

// GetAppConfig 获取全局应用配置对象
// 返回当前的应用配置实例
// 线程安全，可在多个goroutine中并发访问
func GetAppConfig() *entities.AppConfig {
	return globalAppConfig
}

// InitAppConfigStore 初始化应用配置存储
// 读取并解析 configs/app.yaml 配置文件，初始化配置存储
// 该配置文件决定了使用哪种配置源（本地或Consul）
// 返回配置存储对象和可能的错误
func InitAppConfigStore() (*entities.AppConfigStore, error) {
	// 读取应用配置文件
	data, err := ioutil.ReadFile("configs/app.yaml")
	if err != nil {
		fmt.Println("Failed to read app config file")
		return nil, err
	}

	// 解析 YAML 格式的配置文件
	var cfg entities.AppConfigStore
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// SaveConfigVersion 保存配置版本
// 将当前配置保存为一个版本，用于回滚
// 参数:
//   - d: Consul客户端
//   - serverName: 服务名称
//   - env: 环境
//   - description: 版本描述
//
// 返回值:
//   - string: 版本ID
//   - error: 操作成功返回nil，失败返回具体错误
func SaveConfigVersion(d *discovery.Discovery, serverName, env, description string) (string, error) {
	// 获取当前配置
	currentConfig := GetAppConfig()
	if currentConfig == nil {
		return "", fmt.Errorf("current config is nil")
	}

	// 序列化配置
	configData, err := yaml.Marshal(currentConfig)
	if err != nil {
		return "", err
	}

	// 生成版本ID
	versionID := fmt.Sprintf("%s-%d", serverName, time.Now().Unix())

	// 创建版本信息
	versionInfo := entities.ConfigVersion{
		VersionID:   versionID,
		Timestamp:   time.Now(),
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
// 参数:
//   - d: Consul客户端
//   - serverName: 服务名称
//   - env: 环境
//
// 返回值:
//   - []*entities.ConfigVersion: 配置版本列表
//   - error: 操作成功返回nil，失败返回具体错误
func GetConfigVersions(d *discovery.Discovery, serverName, env string) ([]*entities.ConfigVersion, error) {
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

		versions = append(versions, &version)
	}

	return versions, nil
}

// RollbackToVersion 回滚到指定版本
// 参数:
//   - d: Consul客户端
//   - serverName: 服务名称
//   - env: 环境
//   - versionID: 版本ID
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func RollbackToVersion(d *discovery.Discovery, serverName, env, versionID string) error {
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
