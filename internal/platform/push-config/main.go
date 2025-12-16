package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"easyms/internal/shared/discovery"
	"gopkg.in/yaml.v2"
)

// PushConfigToConsul 将本地配置推送到Consul
// 执行流程：
// 1. 读取app.yaml获取Consul连接参数
// 2. 遍历configs目录下的服务配置
// 3. 处理配置继承关系（base.yaml）
// 4. 按Consul规范路径推送配置
func main() {
	// 1. 加载基础配置
	currentDir, _ := os.Getwd()
	fmt.Printf("当前工作目录: %s\n", currentDir)
	
	configPath := filepath.Join(currentDir, "configs")
	fmt.Printf("尝试读取配置路径: %s\n", configPath)
	
	viper.AddConfigPath(configPath)
	viper.SetConfigName("app")
	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("加载app.yaml失败: %w", err))
	}

	// 2. 获取Consul配置
	consulHost := viper.GetString("consul.host")
	keyPath := viper.GetString("consul.key_path")

	// 3. 遍历服务配置目录
	servicesDir := filepath.Join(currentDir, "configs")
	services, err := os.ReadDir(servicesDir)
	if err != nil {
		panic(fmt.Errorf("读取configs目录失败: %w", err))
	}

	for _, service := range services {
		if !service.IsDir() || service.Name() == "share" {
			continue
		}

		servicePath := filepath.Join(servicesDir, service.Name())
		envFiles, _ := os.ReadDir(servicePath)

		for _, envFile := range envFiles {
			if !strings.HasSuffix(envFile.Name(), ".yaml") {
				continue
			}

			// 4. 解析环境类型 (dev.yaml -> dev)
			env := strings.TrimSuffix(envFile.Name(), ".yaml")
			if env == "routes" {
				continue // 跳过路由特殊配置
			}

			// 5. 加载并合并配置
			v := viper.New()
			v.SetConfigFile(filepath.Join(servicePath, envFile.Name()))
			if err := v.ReadInConfig(); err != nil {
				fmt.Printf("⚠️ 跳过 %s/%s: %v\n", service.Name(), envFile.Name(), err)
				continue
			}

			// 处理配置继承
			if extends := v.GetString("extends"); extends != "" {
				basePath := filepath.Join(servicePath, extends)
				baseViper := viper.New()
				baseViper.SetConfigFile(basePath)
				if err := baseViper.ReadInConfig(); err == nil {
					v.MergeConfigMap(baseViper.AllSettings())
				}
			}

			// 6. 推送到Consul
			configKey := fmt.Sprintf(keyPath, service.Name(), env)
			configData, err := yaml.Marshal(v.AllSettings())
			if err != nil {
				fmt.Printf("❌ 序列化配置失败 [%s]: %v\n", configKey, err)
				continue
			}

			// 创建Consul客户端
			consulDiscovery, err := discovery.NewDiscovery(consulHost)
			if err != nil {
				fmt.Printf("❌ 创建Consul客户端失败 [%s]: %v\n", configKey, err)
				continue
			}

			// 推送到Consul KV存储
			if err := consulDiscovery.Put(configKey, string(configData)); err != nil {
				fmt.Printf("❌ 推送失败 [%s]: %v\n", configKey, err)
				continue
			}

			fmt.Printf("✅ 成功推送 [%s] -> %s\n", envFile.Name(), configKey)
		}
	}

	fmt.Println("\n配置推送完成！")
}