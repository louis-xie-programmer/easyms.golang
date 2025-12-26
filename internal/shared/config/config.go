// config.go
// 配置管理模块
package config

import (
	"easyms/internal/shared/models"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"sync/atomic"
)

var (
	// 全局配置对象，用于存储应用的所有配置信息
	// 使用 atomic.Value 确保并发读写安全且高性能
	globalAppConfig atomic.Value
)

// AppConfigProvider 配置提供者接口
// 定义了加载应用配置的方法规范
// 不同的配置源（如本地文件、Consul）需要实现此接口
type AppConfigProvider interface {
	// LoadAppConfig 加载并初始化配置
	LoadAppConfig() error
	// OnChange 配置变更回调
	OnChange() func(*models.AppConfig)
}

// GetAppConfig 获取全局应用配置对象
// 返回当前的应用配置实例
// 线程安全，可在多个goroutine中并发访问
func GetAppConfig() *models.AppConfig {
	val := globalAppConfig.Load()
	if val == nil {
		return nil
	}
	return val.(*models.AppConfig)
}

// SetAppConfig 设置全局应用配置对象
// 线程安全
func SetAppConfig(cfg *models.AppConfig) {
	globalAppConfig.Store(cfg)
}

// InitAppConfigStore 初始化应用配置存储
// 读取并解析 configs/app.yaml 配置文件，初始化配置存储
// 该配置文件决定了使用哪种配置源（本地或Consul）
// 返回配置存储对象和可能的错误
func InitAppConfigStore() (*models.AppConfigStore, error) {
	// 读取应用配置文件
	data, err := os.ReadFile("configs/app.yaml")
	if err != nil {
		fmt.Println("Failed to read app config file")
		return nil, err
	}

	// 解析 YAML 格式的配置文件
	var cfg models.AppConfigStore
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
