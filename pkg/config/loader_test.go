package config

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestLoadAppConfig(t *testing.T) {
	// 创建临时配置文件用于测试
	tempConfig := `env: test
log:
  log_level: info
loki:
  host: localhost:3100
store_type: local
consul:
  host: localhost:8500
  key_path: "easyms/%s/%s"
  reload_on_changes: false
`

	// 创建临时目录和文件
	tempDir := os.TempDir()
	configFile := tempDir + "/app.yaml"

	err := ioutil.WriteFile(configFile, []byte(tempConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	// 保存原始工作目录并切换到临时目录
	originalWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalWd)

	// 创建cfg目录结构
	os.MkdirAll("cfg", 0755)
	os.Rename(configFile, "cfg/app.yaml")

	// 测试加载配置
	config, err := LoadAppConfig()
	if err != nil {
		t.Fatalf("Failed to load app config: %v", err)
	}

	// 验证配置值
	if config.Env != "test" {
		t.Errorf("Expected env to be 'test', got '%s'", config.Env)
	}

	if config.Log["log_level"] != "info" {
		t.Errorf("Expected log_level to be 'info', got '%s'", config.Log["log_level"])
	}

	if config.StoreType != "local" {
		t.Errorf("Expected store_type to be 'local', got '%s'", config.StoreType)
	}

	if config.Consul.Host != "localhost:8500" {
		t.Errorf("Expected consul host to be 'localhost:8500', got '%s'", config.Consul.Host)
	}
}

func TestServiceConfigLoader(t *testing.T) {
	// 创建临时配置文件用于测试
	tempConfig := `env: test
log:
  log_level: info
loki:
  host: localhost:3100
store_type: local
consul:
  host: localhost:8500
  key_path: "easyms/%s/%s"
  reload_on_changes: false
`

	serviceConfig := `
server:
  host: localhost
  port: 8080
database:
  host: localhost
  port: 5432
`

	// 创建临时目录和文件
	tempDir := os.TempDir()
	configFile := tempDir + "/app.yaml"
	serviceConfigFile := tempDir + "/auth-svc.yaml"

	err := ioutil.WriteFile(configFile, []byte(tempConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp config file: %v", err)
	}

	err = ioutil.WriteFile(serviceConfigFile, []byte(serviceConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp service config file: %v", err)
	}

	// 保存原始工作目录并切换到临时目录
	originalWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalWd)

	// 创建cfg目录结构
	os.MkdirAll("cfg", 0755)
	os.Rename(configFile, "cfg/app.yaml")
	os.MkdirAll("cfg/auth-svc", 0755)
	os.Rename(serviceConfigFile, "cfg/auth-svc/test.yaml")

	// 加载应用配置
	appConfig, err := LoadAppConfig()
	if err != nil {
		t.Fatalf("Failed to load app config: %v", err)
	}

	// 创建服务配置加载器
	loader, err := NewServiceConfigLoader(appConfig, "auth-svc")
	if err != nil {
		t.Fatalf("Failed to create service config loader: %v", err)
	}

	// 获取配置
	config := loader.GetConfig()

	// 验证配置值
	if config["server"] == nil {
		t.Error("Expected server config to exist")
		return
	}

	// 在YAML中，map[interface{}]interface{}是常见的，需要进行类型断言处理
	serverConfigMap := config["server"].(map[interface{}]interface{})
	if serverConfigMap["host"] != "localhost" {
		t.Errorf("Expected server host to be 'localhost', got '%s'", serverConfigMap["host"])
	}

	if serverConfigMap["port"] != 8080 {
		t.Errorf("Expected server port to be 8080, got '%v'", serverConfigMap["port"])
	}
}
