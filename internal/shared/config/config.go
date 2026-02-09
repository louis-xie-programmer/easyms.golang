// Package config 提供了统一的配置管理功能。
//
// 该包的核心设计思想是:
// 1. 定义一个全局的、线程安全的应用配置实例 (globalAppConfig)。
// 2. 抽象出 AppConfigProvider 接口，用于支持不同的配置源 (如本地文件、Consul)。
// 3. 提供简单的方法 (GetAppConfig, SetAppConfig) 来访问和更新全局配置。
package config

import (
	"easyms/internal/shared/models"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"sync/atomic"
)

var (
	// globalAppConfig 用于原子性地存储全局唯一的应用配置。
	// 使用 atomic.Value 可以确保在并发读写场景下的高性能和类型安全。
	globalAppConfig atomic.Value
)

// AppConfigProvider 定义了配置提供者的标准接口。
// 任何配置源 (如本地文件、远程配置中心) 都应实现此接口，
// 以便统一加载和更新配置的逻辑。
type AppConfigProvider interface {
	// LoadAppConfig 加载并初始化应用的全部配置。
	// 实现此方法时，应负责读取配置、解析、合并，并最终调用 SetAppConfig 设置到全局。
	LoadAppConfig() error

	// OnChange 返回一个用于处理配置变更的回调函数。
	// 对于支持热重载的配置提供者，此方法可以用来注册监听器或触发更新。
	// 对于不支持热重载的，可以返回一个空操作函数。
	OnChange() func(*models.AppConfig)
}

// GetAppConfig 安全地获取全局应用配置对象。
// 这是一个线程安全的方法，可以在项目的任何地方并发调用。
// 如果配置尚未加载，它将返回 nil。
func GetAppConfig() *models.AppConfig {
	val := globalAppConfig.Load()
	if val == nil {
		return nil
	}
	return val.(*models.AppConfig)
}

// SetAppConfig 安全地设置或更新全局应用配置对象。
// 这是一个线程安全的方法，通常在配置初始化或热重载时由 AppConfigProvider 调用。
func SetAppConfig(cfg *models.AppConfig) {
	globalAppConfig.Store(cfg)
}

// InitAppConfigStore 初始化应用配置的 "元配置"。
// 它负责读取项目根目录下的 `configs/app.yaml` 文件。
// 这个文件本身不包含业务配置，而是定义了从哪里加载真正的业务配置（例如，从 "local" 文件系统还是从 "consul"）。
// 返回一个包含存储策略的 AppConfigStore 对象。
func InitAppConfigStore() (*models.AppConfigStore, error) {
	// 读取应用的元配置文件
	data, err := os.ReadFile("configs/app.yaml")
	if err != nil {
		return nil, fmt.Errorf("读取应用元配置文件 'configs/app.yaml' 失败: %w", err)
	}

	// 解析 YAML 格式的元配置
	var cfg models.AppConfigStore
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("解析 'configs/app.yaml' 失败: %w", err)
	}

	return &cfg, nil
}
