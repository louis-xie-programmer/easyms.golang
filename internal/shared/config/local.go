// local.go
package config

import (
	"easyms/internal/shared/models"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type LocalConfig struct {
	ServerName string
	Env        string
	configMgr  *ConfigurationManager
}

func NewLocalConfig(serviceName string, env string) AppConfigProvider {
	if serviceName == "" {
		serviceName = "gateway"
	}
	if env == "" {
		env = "dev"
	}
	return &LocalConfig{
		ServerName: serviceName,
		Env:        env,
		configMgr:  NewConfigurationManager(nil),
	}
}

func (lc *LocalConfig) OnChange() func(*models.AppConfig) {
	// 暂时只在consul中更新配置
	// lc.configMgr.UpdateConfig(newConfig)
	return func(newConfig *models.AppConfig) {
	}
}

func (lc *LocalConfig) LoadAppConfig() error {
	appPath := GetLocalAppConfigFileName(lc.Env)

	data, err := os.ReadFile(appPath)
	if err != nil {
		return err
	}

	// 读取app配置文件
	var appCfg models.AppConfig
	if err := yaml.Unmarshal(data, &appCfg); err != nil {
		return err
	}

	// 读取服务配置文件
	path := GetLocalServerConfigFileName(lc.ServerName, lc.Env)

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		fmt.Printf("服务配置文件不存在，使用全局配置 %s", appPath)
		// 本地服务配置文件不存在，合并到全局配置
		globalAppConfig = &appCfg
		return nil
	}

	// 读取本地服务配置文件
	data, err = os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg models.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// 深度合并配置
	// 先合并appCfg到cfg中，这样服务特定配置会覆盖共享配置
	if err := mergoConfig(&cfg, &appCfg); err != nil {
		return fmt.Errorf("failed to merge configs: %w", err)
	}

	// 更新全局配置
	globalAppConfig = &cfg

	return nil
}

// mergoConfig 合并两个配置对象
func mergoConfig(dst, src *models.AppConfig) error {
	if dst.Log == (models.LogConfig{}) {
		dst.Log = src.Log
	}

	if dst.Loki == (models.LokiConfig{}) {
		dst.Loki = src.Loki
	}

	if dst.Database == (models.DatabaseConfig{}) {
		dst.Database = src.Database
	} else {
		// 只有当目标配置中的字段为空时才从源配置复制
		if dst.Database.Type == "" {
			dst.Database.Type = src.Database.Type
		}
		if dst.Database.Host == "" {
			dst.Database.Host = src.Database.Host
		}
		if dst.Database.Port == 0 {
			dst.Database.Port = src.Database.Port
		}
		if dst.Database.UserName == "" && src.Database.UserName != "" {
			dst.Database.UserName = src.Database.UserName
		}
		if dst.Database.Password == "" && src.Database.Password != "" {
			dst.Database.Password = src.Database.Password
		}
		if dst.Database.Database == "" {
			dst.Database.Database = src.Database.Database
		}
		if dst.Database.MaxIdleConns == 0 {
			dst.Database.MaxIdleConns = src.Database.MaxIdleConns
		}
		if dst.Database.MaxOpenConns == 0 {
			dst.Database.MaxOpenConns = src.Database.MaxOpenConns
		}
		if dst.Database.ConnMaxLifetime == 0 {
			dst.Database.ConnMaxLifetime = src.Database.ConnMaxLifetime
		}
		if dst.Database.ConnMaxIdleTime == 0 {
			dst.Database.ConnMaxIdleTime = src.Database.ConnMaxIdleTime
		}
	}

	if dst.OAuth2 == (models.OAuth2Config{}) {
		dst.OAuth2 = src.OAuth2
	}

	// 合并Cache.Redis配置
	if dst.Cache.Redis == (models.RedisConfig{}) {
		dst.Cache.Redis = src.Cache.Redis
	} else {
		if dst.Cache.Redis.Address == "" {
			dst.Cache.Redis.Address = src.Cache.Redis.Address
		}
		if dst.Cache.Redis.Password == "" {
			dst.Cache.Redis.Password = src.Cache.Redis.Password
		}
		if dst.Cache.Redis.DB == 0 {
			dst.Cache.Redis.DB = src.Cache.Redis.DB
		}
		if dst.Cache.Redis.NullCacheExpire == 0 {
			dst.Cache.Redis.NullCacheExpire = src.Cache.Redis.NullCacheExpire
		}
		if dst.Cache.Redis.MutexExpire == 0 {
			dst.Cache.Redis.MutexExpire = src.Cache.Redis.MutexExpire
		}
	}

	if dst.RabbitMQ == (models.RabbitMQConfig{}) {
		dst.RabbitMQ = src.RabbitMQ
	}

	// Server配置通常来自服务特定配置文件，不需要合并

	return nil
}

// GetLocalServerConfigFileName 获取本地服务配置文件名
func GetLocalServerConfigFileName(server string, env string) string {
	return fmt.Sprintf("configs/%s/%s.yaml", server, env)
}

// GetLocalAppConfigFileName 获取本地app配置文件名
func GetLocalAppConfigFileName(env string) string {
	return fmt.Sprintf("configs/share/%s.yaml", env)
}
