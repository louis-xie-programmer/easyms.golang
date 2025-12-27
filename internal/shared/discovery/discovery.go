// discovery.go 服务注册与发现模块
package discovery

import (
	"fmt"
	"github.com/hashicorp/consul/api"
)

// Discovery 服务发现客户端
type Discovery struct {
	client *api.Client
}

// NewDiscovery 创建新的服务发现客户端
func NewDiscovery(addr string) (*Discovery, error) {
	config := api.DefaultConfig()
	config.Address = addr
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &Discovery{client: client}, nil
}

// Register 注册服务到 Consul
// check 参数是可选的，如果为 nil，则会创建一个默认的 HTTP 健康检查。
func (d *Discovery) Register(name, host string, port int, tags []string, check *api.AgentServiceCheck) error {
	// 如果没有提供自定义的健康检查，则创建一个默认的。
	if check == nil {
		check = &api.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d/health", host, port),
			Interval:                       "5s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "60s",
		}
	}

	svc := &api.AgentServiceRegistration{
		Name:    name,
		Address: host,
		Port:    port,
		Tags:    tags,
		Check:   check,
	}

	return d.client.Agent().ServiceRegister(svc)
}

// DeRegister 从 Consul 注销服务
func (d *Discovery) DeRegister(name string) error {
	return d.client.Agent().ServiceDeregister(name)
}

// Get 从 Consul KV 存储获取值
func (d *Discovery) Get(key string) (string, error) {
	kv := d.client.KV()
	pair, _, err := kv.Get(key, nil)
	if err != nil {
		return "", err
	}
	if pair == nil {
		return "", nil
	}
	return string(pair.Value), nil
}

// Put 向 Consul KV 存储设置值
func (d *Discovery) Put(key, val string) error {
	kv := d.client.KV()
	_, err := kv.Put(&api.KVPair{Key: key, Value: []byte(val)}, nil)
	return err
}

// ListKeys 获取指定前缀的所有键
func (d *Discovery) ListKeys(prefix string) ([]string, error) {
	kv := d.client.KV()
	pairs, _, err := kv.List(prefix, nil)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		keys = append(keys, pair.Key)
	}
	return keys, nil
}

// Delete 删除指定键
func (d *Discovery) Delete(key string) error {
	kv := d.client.KV()
	_, err := kv.Delete(key, nil)
	return err
}
