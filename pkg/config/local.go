package config

import (
	"easyms/pkg/entitis"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

type LocalConfig struct {
	ServerName string
	Env        string
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
	}
}

func (lc *LocalConfig) LoadAppConfig() error {
	appPath := GetLocalAppConfigFileName(lc.Env)

	data, err := ioutil.ReadFile(appPath)
	if err != nil {
		return err
	}

	// 读取app配置文件
	var appCfg entitis.AppConfig
	if err := yaml.Unmarshal(data, &appCfg); err != nil {
		return err
	}

	// 读取服务配置文件
	path := GetLocalServerConfigFileName(lc.ServerName, lc.Env)

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		fmt.Printf("服务配置文件不存在，使用全局配置 %s", appPath)
		// 本地服务配置文件不存在，合并到全局配置
		appConfig = &appCfg
		return nil
	}

	// 读取本地服务配置文件
	data, err = ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg entitis.AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// 确保全局配置对象已初始化
	if appConfig == nil {
		appConfig = &entitis.AppConfig{}
	}

	if cfg.Log != (entitis.LogConfig{}) {
		appConfig.Log = cfg.Log
	}
	if cfg.Loki != (entitis.LokiConfig{}) {
		appConfig.Loki = cfg.Loki
	}
	if cfg.Server != (entitis.ServerConfig{}) {
		appConfig.Server = cfg.Server
	}
	if cfg.Database != (entitis.DatabaseConfig{}) {
		appConfig.Database = cfg.Database
	}

	// 确保至少有基本配置
	if appConfig.Log == (entitis.LogConfig{}) {
		appConfig.Log = entitis.LogConfig{
			LogLevel: "info",
			LogType:  "zerolog",
		}
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
