package config

import (
	"easyms/pkg/entities"
	"fmt"
	"io/ioutil"
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

func (lc *LocalConfig) LoadAppConfig() error {
	appPath := GetLocalAppConfigFileName(lc.Env)

	data, err := ioutil.ReadFile(appPath)
	if err != nil {
		return err
	}

	// 读取app配置文件
	var appCfg entities.AppConfig
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
	data, err = ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg entities.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	currentAppConfig := lc.configMgr.GetConfig()
	if currentAppConfig == nil {
		currentAppConfig = &entities.AppConfig{}
	}

	// 深度合并配置
	lc.configMgr.MergeConfigs(currentAppConfig, &cfg)

	// 确保至少有基本配置
	isGateway := lc.ServerName == "gateway"
	lc.configMgr.EnsureBasicConfig(currentAppConfig, isGateway)

	// 更新全局配置
	globalAppConfig = currentAppConfig

	// 验证配置的有效性
	if err := globalAppConfig.Validate(isGateway); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

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