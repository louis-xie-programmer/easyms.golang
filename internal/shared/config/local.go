// local.go
package config

import (
	"easyms/internal/shared/models"
	"fmt"
	"os"

	"dario.cat/mergo"
	"gopkg.in/yaml.v2"
)

type LocalConfig struct {
	ServerName string
	Env        string
}

func NewLocalConfig(serviceName string, env string) AppConfigProvider {
	if serviceName == "" {
		serviceName = "gateway" // Default service for safety
	}
	if env == "" {
		env = "dev" // Default env for safety
	}
	return &LocalConfig{
		ServerName: serviceName,
		Env:        env,
	}
}

func (lc *LocalConfig) OnChange() func(*models.AppConfig) {
	// This is a stub for local config. Real implementation would be in Consul provider.
	return func(newConfig *models.AppConfig) {
	}
}

func (lc *LocalConfig) LoadAppConfig() error {
	// 1. Load shared configuration file (e.g., configs/share/dev.yaml)
	sharedPath := GetLocalAppConfigFileName(lc.Env)
	sharedData, err := os.ReadFile(sharedPath)
	if err != nil {
		return fmt.Errorf("failed to read shared config file %s: %w", sharedPath, err)
	}

	var sharedCfg models.AppConfig
	if err := yaml.Unmarshal(sharedData, &sharedCfg); err != nil {
		return fmt.Errorf("failed to unmarshal shared config: %w", err)
	}

	// 2. Load service-specific configuration file (e.g., configs/gateway/dev.yaml)
	servicePath := GetLocalServerConfigFileName(lc.ServerName, lc.Env)
	serviceData, err := os.ReadFile(servicePath)
	if err != nil {
		// If the service-specific file doesn't exist, just use the shared config.
		if os.IsNotExist(err) {
			SetAppConfig(&sharedCfg)
			return nil
		}
		return fmt.Errorf("failed to read service config file %s: %w", servicePath, err)
	}

	var serviceCfg models.AppConfig
	if err := yaml.Unmarshal(serviceData, &serviceCfg); err != nil {
		return fmt.Errorf("failed to unmarshal service config: %w", err)
	}

	// 3. Merge shared config into service config.
	// Service-specific values will overwrite shared values if they exist.
	if err := mergo.Merge(&serviceCfg, sharedCfg); err != nil {
		return fmt.Errorf("failed to merge configs: %w", err)
	}

	// 4. Set the final, merged config as the global application config.
	SetAppConfig(&serviceCfg)

	return nil
}

// GetLocalServerConfigFileName returns the path to the service-specific config file.
func GetLocalServerConfigFileName(server string, env string) string {
	return fmt.Sprintf("configs/%s/%s.yaml", server, env)
}

// GetLocalAppConfigFileName returns the path to the shared config file.
func GetLocalAppConfigFileName(env string) string {
	return fmt.Sprintf("configs/share/%s.yaml", env)
}
