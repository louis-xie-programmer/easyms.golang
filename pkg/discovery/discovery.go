// discovery.go 服务注册与发现模块
// 主要功能：
// 1. 与 Consul 服务进行交互
// 2. 实现服务注册与注销
// 3. 提供键值存储功能
// 4. 实现服务健康检查机制
package discovery

import (
	"fmt"
	"github.com/hashicorp/consul/api"
)

// Discovery 服务发现客户端
// 封装了与 Consul 的交互操作
// 提供服务注册、注销和键值存储功能
type Discovery struct {
	client *api.Client  // Consul API 客户端
}

// NewDiscovery 创建新的服务发现客户端
// 初始化 Consul 客户端配置并建立连接
// 参数:
//   - addr: Consul 服务地址
// 返回值:
//   - *Discovery: 服务发现客户端实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewDiscovery(addr string) (*Discovery, error) {
	// 创建 Consul 客户端配置
	config := api.DefaultConfig()
	config.Address = addr

	// 创建 Consul 客户端
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &Discovery{
		client: client,
	}, nil
}

// Register 注册服务到 Consul
// 配置服务健康检查并注册服务
// 参数:
//   - name: 服务名称
//   - host: 服务主机地址
//   - port: 服务端口
//   - tags: 服务标签
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (d *Discovery) Register(name, host string, port int, tags []string) error {
	// 配置服务健康检查
	// 定期检查服务的 /health 端点以确认服务状态
	check := &api.AgentServiceCheck{
		HTTP:                           fmt.Sprintf("http://%s:%d/health", host, port),
		Interval:                       "5s",     // 检查间隔
		Timeout:                        "2s",     // 超时时间
		DeregisterCriticalServiceAfter: "60s",     // 自动删除不健康服务
	}
	
	// 配置服务注册信息
	svc := &api.AgentServiceRegistration{
		Name:    name,
		Address: host,
		Port:    port,
		Tags:    tags,
		Check:   check,
	}
	
	// 执行服务注册
	return d.client.Agent().ServiceRegister(svc)
}

// DeRegister 从 Consul 注销服务
// 当服务关闭时调用此方法从 Consul 中移除服务注册信息
// 参数:
//   - name: 服务名称
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (d *Discovery) DeRegister(name string) error {
	return d.client.Agent().ServiceDeregister(name)
}

// Get 从 Consul KV 存储获取值
// 参数:
//   - key: 键名
// 返回值:
//   - string: 键对应的值
//   - error: 操作成功返回nil，失败返回具体错误
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
// 用于在 Consul 中存储配置信息
// 参数:
//   - key: 键名
//   - val: 键值
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (d *Discovery) Put(key, val string) error {
	kv := d.client.KV()
	_, err := kv.Put(&api.KVPair{Key: key, Value: []byte(val)}, nil)
	return err
}

// ListKeys 获取指定前缀的所有键
// 参数:
//   - prefix: 键前缀
// 返回值:
//   - []string: 符合条件的键列表
//   - error: 操作成功返回nil，失败返回具体错误
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
// 参数:
//   - key: 要删除的键
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (d *Discovery) Delete(key string) error {
	kv := d.client.KV()
	_, err := kv.Delete(key, nil)
	return err
}