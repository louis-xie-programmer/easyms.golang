package config

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
)

// AppConfig 代表cfg/app.yaml的结构
// 只从本地读取
type AppConfig struct {
	Env       string            `yaml:"env"`
	Log       map[string]string `yaml:"log"`
	Loki      map[string]string `yaml:"loki"`
	StoreType string            `yaml:"store_type"`
	Consul    struct {
		Host            string `yaml:"host"`
		KeyPath         string `yaml:"key_path"`
		ReloadOnChanges bool   `yaml:"reload_on_changes"`
	} `yaml:"consul"`
}

// LoadAppConfig 只从本地读取cfg/app.yaml
func LoadAppConfig() (*AppConfig, error) {
	data, err := ioutil.ReadFile("cfg/app.yaml")
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ServiceConfigLoader 支持本地yaml和consul两种方式
type ServiceConfigLoader struct {
	appConfig *AppConfig
	service   string
	mu        sync.RWMutex
	config    map[string]interface{}
	stopCh    chan struct{}
}

// NewServiceConfigLoader 创建服务配置加载器
func NewServiceConfigLoader(appConfig *AppConfig, service string) (*ServiceConfigLoader, error) {
	loader := &ServiceConfigLoader{
		appConfig: appConfig,
		service:   service,
		config:    make(map[string]interface{}),
		stopCh:    make(chan struct{}),
	}
	if appConfig.StoreType == "consul" {
		if err := loader.loadFromConsul(); err != nil {
			return nil, err
		}
		if appConfig.Consul.ReloadOnChanges {
			go loader.watchConsul()
		}
	} else {
		if err := loader.loadFromFile(); err != nil {
			return nil, err
		}
	}
	return loader, nil
}

func (l *ServiceConfigLoader) loadFromFile() error {
	path := filepath.Join("cfg", l.service, l.appConfig.Env+".yaml")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return yaml.Unmarshal(data, &l.config)
}

func (l *ServiceConfigLoader) loadFromConsul() error {
	key := fmt.Sprintf(l.appConfig.Consul.KeyPath, l.service, l.appConfig.Env)
	loader, err := NewLoader(l.appConfig.Consul.Host, "")
	if err != nil {
		return err
	}
	val, err := loader.Get(key)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return yaml.Unmarshal([]byte(val), &l.config)
}

// watchConsul 支持动态更新
func (l *ServiceConfigLoader) watchConsul() {
	key := fmt.Sprintf(l.appConfig.Consul.KeyPath, l.service, l.appConfig.Env)
	loader, _ := NewLoader(l.appConfig.Consul.Host, "")
	var lastIndex uint64
	for {
		select {
		case <-l.stopCh:
			return
		default:
		}
		kv := loader.Client.KV()
		pair, meta, err := kv.Get(key, &api.QueryOptions{WaitIndex: lastIndex, WaitTime: 30 * time.Second})
		if err == nil && pair != nil && meta.LastIndex != lastIndex {
			l.mu.Lock()
			yaml.Unmarshal(pair.Value, &l.config)
			l.mu.Unlock()
			lastIndex = meta.LastIndex
		}
		time.Sleep(1 * time.Second)
	}
}

// GetConfig 返回当前配置快照
func (l *ServiceConfigLoader) GetConfig() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()
	copy := make(map[string]interface{})
	for k, v := range l.config {
		copy[k] = v
	}
	return copy
}

// Stop 停止动态监听
func (l *ServiceConfigLoader) Stop() {
	close(l.stopCh)
}
