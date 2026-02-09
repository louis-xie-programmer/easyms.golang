// Package discovery 提供了基于 Consul 的服务注册与发现功能。
// 它封装了 Consul API，为微服务提供了注册、注销、健康检查以及 KV 存储等核心能力。
package discovery

import (
	"fmt"
	"github.com/hashicorp/consul/api"
)

// Discovery 是一个 Consul 客户端的封装，提供了服务发现和 KV 存储的便捷方法。
type Discovery struct {
	client *api.Client
}

// NewDiscovery 创建一个新的服务发现客户端实例。
// addr: Consul agent 的地址，例如 "127.0.0.1:8500"。
func NewDiscovery(addr string) (*Discovery, error) {
	config := api.DefaultConfig()
	config.Address = addr
	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("创建 Consul 客户端失败: %w", err)
	}
	return &Discovery{client: client}, nil
}

// Register 将一个服务实例注册到 Consul。
// name: 服务名称。
// host: 服务实例的 IP 地址或主机名。
// port: 服务实例的端口号。
// tags: 为服务实例添加的标签，可用于服务过滤。
// check: 自定义的健康检查配置。如果为 nil，将使用一个默认的 HTTP 健康检查。
func (d *Discovery) Register(name, host string, port int, tags []string, check *api.AgentServiceCheck) error {
	// 如果没有提供自定义的健康检查，则创建一个默认的 HTTP 健康检查。
	// 这个默认检查会定期请求服务的 /health 接口。
	if check == nil {
		check = &api.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d/health", host, port),
			Interval:                       "5s",  // 每5秒检查一次
			Timeout:                        "2s",  // 检查超时时间为2秒
			DeregisterCriticalServiceAfter: "60s", // 如果服务持续60秒处于 "critical" 状态，则自动注销
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

// DeRegister 从 Consul 中注销一个服务实例。
// name: 要注销的服务名称。
func (d *Discovery) DeRegister(name string) error {
	return d.client.Agent().ServiceDeregister(name)
}

// Get 从 Consul 的 KV 存储中获取一个键的值。
// key: 要获取的键。
// 如果键不存在，返回一个空字符串和 nil 错误。
func (d *Discovery) Get(key string) (string, error) {
	kv := d.client.KV()
	pair, _, err := kv.Get(key, nil)
	if err != nil {
		return "", err
	}
	if pair == nil {
		return "", nil // 键不存在
	}
	return string(pair.Value), nil
}

// Put 向 Consul 的 KV 存储中设置一个键值对。
// 如果键已存在，其值将被覆盖。
// key: 要设置的键。
// val: 要设置的值。
func (d *Discovery) Put(key, val string) error {
	kv := d.client.KV()
	_, err := kv.Put(&api.KVPair{Key: key, Value: []byte(val)}, nil)
	return err
}

// ListKeys 获取 Consul KV 存储中指定前缀下的所有键。
// prefix: 要查询的键前缀。
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

// Delete 从 Consul KV 存储中删除一个键。
// key: 要删除的键。
func (d *Discovery) Delete(key string) error {
	kv := d.client.KV()
	_, err := kv.Delete(key, nil)
	return err
}
