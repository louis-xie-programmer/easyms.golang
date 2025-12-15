// server.go 服务发现与负载均衡模块
// 主要功能：
// 1. 监听 Consul 中服务的变化
// 2. 实现服务实例的负载均衡
// 3. 集成熔断器功能
// 4. 提供服务实例管理和健康检查
package discovery

import (
	"fmt"
	"github.com/hashicorp/consul/api"
	"log"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

// ServiceDiscovery 服务发现客户端
// 提供服务监听、负载均衡和熔断功能
// 通过长轮询监听 Consul 中服务实例的变化
// 实现轮询负载均衡算法
// 为每个服务维护独立的熔断器
type ServiceDiscovery struct {
	client *api.Client                 // Consul 客户端

	services map[string][]string       // 服务实例列表，key为服务名，value为实例地址列表
	serviceDetails map[string][]*api.AgentService // 服务实例详细信息
	lbIndex  map[string]int            // 负载均衡索引，记录每个服务的轮询位置

	lock sync.RWMutex                  // 读写锁，保护服务实例列表的并发访问

	breakers   map[string]*gobreaker.CircuitBreaker  // 熔断器映射，key为服务名
	breakerMtx sync.RWMutex                           // 熔断器读写锁
	
	// 负载均衡策略映射
	loadBalancers map[string]LoadBalanceStrategy
	lbMutex       sync.RWMutex
	
	// 健康检查管理器
	healthManager *ServiceHealthManager
	
	// 权重负载均衡器
	weightedLoadBalancer *WeightedLoadBalancer
}

// NewServiceDiscovery 创建新的服务发现客户端
// 初始化服务发现客户端，创建必要的数据结构
// 参数:
//   - addr: Consul 服务地址
// 返回值:
//   - *ServiceDiscovery: 服务发现客户端实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewServiceDiscovery(addr string) (*ServiceDiscovery, error) {
	config := api.DefaultConfig()
	config.Address = addr

	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	// 创建健康检查器
	healthChecker := NewHTTPHealthChecker(3*time.Second, "/health")
	healthManager := NewServiceHealthManager(healthChecker)

	sd := &ServiceDiscovery{
		client:        client,
		services:      make(map[string][]string),
		serviceDetails: make(map[string][]*api.AgentService),
		lbIndex:       make(map[string]int),
		breakers:      make(map[string]*gobreaker.CircuitBreaker),
		loadBalancers: make(map[string]LoadBalanceStrategy),
		healthManager: healthManager,
		weightedLoadBalancer: NewWeightedLoadBalancer(),
	}

	// 默认使用轮询负载均衡策略
	sd.SetLoadBalancer("default", &RoundRobinLoadBalancer{})

	return sd, nil
}

// SetLoadBalancer 为特定服务设置负载均衡策略
func (sd *ServiceDiscovery) SetLoadBalancer(serviceName string, strategy LoadBalanceStrategy) {
	sd.lbMutex.Lock()
	defer sd.lbMutex.Unlock()
	sd.loadBalancers[serviceName] = strategy
}

// GetLoadBalancer 获取服务的负载均衡策略
func (sd *ServiceDiscovery) GetLoadBalancer(serviceName string) LoadBalanceStrategy {
	sd.lbMutex.RLock()
	defer sd.lbMutex.RUnlock()
	
	if lb, exists := sd.loadBalancers[serviceName]; exists {
		return lb
	}
	
	// 返回默认负载均衡策略
	if lb, exists := sd.loadBalancers["default"]; exists {
		return lb
	}
	
	// 如果没有默认策略，创建一个轮询策略
	defaultLB := &RoundRobinLoadBalancer{}
	sd.loadBalancers["default"] = defaultLB
	return defaultLB
}

// GetHealthyInstances 获取健康的服务实例
// 通过 Consul Health API 查询指定服务的健康实例
// 参数:
//   - service: 服务名称
// 返回值:
//   - []*api.ServiceEntry: 健康的服务实例列表
//   - error: 操作成功返回nil，失败返回具体错误
func (d *ServiceDiscovery) GetHealthyInstances(service string) ([]*api.ServiceEntry, error) {
	services, _, err := d.client.Health().Service(service, "", true, nil)
	return services, err
}

// ensureBreaker 确保服务对应的熔断器存在
// 如果指定服务的熔断器不存在，则创建一个新的熔断器
// 参数:
//   - service: 服务名称
// 返回值:
//   - *gobreaker.CircuitBreaker: 熔断器实例
func (sd *ServiceDiscovery) ensureBreaker(service string) *gobreaker.CircuitBreaker {
	sd.breakerMtx.RLock()
	cb, ok := sd.breakers[service]
	sd.breakerMtx.RUnlock()
	if ok {
		return cb
	}

	// 创建默认熔断器配置
	settings := gobreaker.Settings{
		Name:        service,           // 熔断器名称，通常为服务名
		MaxRequests: 5,                // 半开状态下允许的请求数
		Interval:    60 * time.Second, // 清除失败计数的时间窗口
		Timeout:     30 * time.Second, // 保持断路状态的超时时间
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 触发熔断的条件：失败次数超过阈值且失败率超过50%
			const minFailures = 5
			const failureRatio = 0.5
			if counts.Requests < uint32(minFailures) {
				return false
			}
			if counts.TotalFailures == 0 {
				return false
			}
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)
			return ratio >= failureRatio
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Printf("circuit[%s] state change: %v -> %v\n", name, from, to)
			// TODO: 向 Prometheus 发送指标
		},
	}

	cb = gobreaker.NewCircuitBreaker(settings)

	sd.breakerMtx.Lock()
	// 双重检查防止竞态条件
	if existing, exists := sd.breakers[service]; exists {
		cb = existing
	} else {
		sd.breakers[service] = cb
	}
	sd.breakerMtx.Unlock()

	return cb
}

// WatchService 监听服务变化
// 启动一个 goroutine 监听指定服务的实例变化
// 使用 Consul 的长轮询机制实现实时更新
// 参数:
//   - serviceName: 服务名称
func (d *ServiceDiscovery) WatchService(serviceName string) {
	go func() {
		var lastIndex uint64 = 0

		for {
			// 长轮询监听服务变化
			// 通过 WaitIndex 实现长轮询，只有服务发生变化时才返回
			entries, meta, err := d.client.Health().Service(serviceName, "", true, &api.QueryOptions{
				WaitIndex: lastIndex, // 长轮询索引
				WaitTime:  0,         // 等待时间，默认使用 Consul 的最大等待时间 (10分钟)
			})
			if err != nil {
				log.Println("Consul watch error:", err)
				continue
			}

			// WaitIndex 变化表示服务有更新
			// 当服务实例发生变化时，LastIndex 会更新
			if meta.LastIndex != lastIndex {
				lastIndex = meta.LastIndex

				// 提取服务实例地址列表
				// 从服务条目中提取地址和端口，组合成完整的地址字符串
				addresses := make([]string, 0)
				serviceDetails := make([]*api.AgentService, 0)
				for _, entry := range entries {
					addr := entry.Service.Address
					port := entry.Service.Port
					addresses = append(addresses, addr+":"+portString(port))
					serviceDetails = append(serviceDetails, entry.Service)
					
					// 更新权重负载均衡器中的实例信息
					weight := 1
					// 注意：这里简化处理，实际应该从entry.Service.Tags或其他地方获取权重
					/*
					if entry.Service.Weights != nil {
						weight = entry.Service.Weights.Passing
					}
					*/
					
					metadata := entry.Service.Meta
					if metadata == nil {
						metadata = make(map[string]string)
					}
					
					d.weightedLoadBalancer.AddInstance(
						addr+":"+portString(port),
						weight,
						metadata,
					)
				}

				// 更新服务实例列表
				// 加锁确保并发安全
				d.lock.Lock()
				d.services[serviceName] = addresses
				d.serviceDetails[serviceName] = serviceDetails
				d.lock.Unlock()

				// 启动健康检查工作协程（如果还没有运行）
				go d.healthManager.HealthCheckWorker(addresses, 10*time.Second)

				log.Printf("Service [%s] refreshed: %v\n", serviceName, addresses)
			}
		}
	}()
}

// GetService 获取服务实例（根据负载均衡策略）
// 使用配置的负载均衡策略从服务实例列表中选择一个实例
// 参数:
//   - serviceName: 服务名称
// 返回值:
//   - string: 服务实例地址
func (d *ServiceDiscovery) GetService(serviceName string) string {
	d.lock.RLock()
	instances := d.services[serviceName]
	d.lock.RUnlock()

	if len(instances) == 0 {
		return ""
	}

	// 过滤不健康的实例
	healthyInstances := make([]string, 0)
	for _, instance := range instances {
		if d.healthManager.IsHealthy(instance) {
			healthyInstances = append(healthyInstances, instance)
		}
	}

	if len(healthyInstances) == 0 {
		// 如果没有健康实例，返回第一个实例（降级处理）
		return instances[0]
	}

	// 使用配置的负载均衡策略选择服务实例
	lb := d.GetLoadBalancer(serviceName)
	return lb.Select(healthyInstances)
}

// GetServiceWithStrategy 使用指定策略获取服务实例
// 参数:
//   - serviceName: 服务名称
//   - strategy: 负载均衡策略
// 返回值:
//   - string: 服务实例地址
func (d *ServiceDiscovery) GetServiceWithStrategy(serviceName string, strategy LoadBalanceStrategy) string {
	d.lock.RLock()
	instances := d.services[serviceName]
	d.lock.RUnlock()

	if len(instances) == 0 {
		return ""
	}

	// 过滤不健康的实例
	healthyInstances := make([]string, 0)
	for _, instance := range instances {
		if d.healthManager.IsHealthy(instance) {
			healthyInstances = append(healthyInstances, instance)
		}
	}

	if len(healthyInstances) == 0 {
		// 如果没有健康实例，返回第一个实例（降级处理）
		return instances[0]
	}

	return strategy.Select(healthyInstances)
}

// GetWeightedService 使用权重负载均衡策略获取服务实例
// 参数:
//   - serviceName: 服务名称
// 返回值:
//   - string: 服务实例地址
func (d *ServiceDiscovery) GetWeightedService(serviceName string) string {
	d.lock.RLock()
	instances := d.services[serviceName]
	d.lock.RUnlock()

	if len(instances) == 0 {
		return ""
	}

	// 过滤不健康的实例
	healthyInstances := make([]string, 0)
	for _, instance := range instances {
		if d.healthManager.IsHealthy(instance) {
			healthyInstances = append(healthyInstances, instance)
		}
	}

	if len(healthyInstances) == 0 {
		// 如果没有健康实例，返回第一个实例（降级处理）
		return instances[0]
	}

	// 使用权重负载均衡策略选择服务实例
	return d.weightedLoadBalancer.Select(healthyInstances)
}

// GetServiceDetails 获取服务实例详细信息
// 参数:
//   - serviceName: 服务名称
// 返回值:
//   - []*api.AgentService: 服务实例详细信息列表
func (d *ServiceDiscovery) GetServiceDetails(serviceName string) []*api.AgentService {
	d.lock.RLock()
	defer d.lock.RUnlock()
	
	if details, exists := d.serviceDetails[serviceName]; exists {
		return details
	}
	
	return nil
}

// GetBreaker 获取服务对应的熔断器
// 确保指定服务的熔断器存在并返回
// 参数:
//   - serviceName: 服务名称
// 返回值:
//   - *gobreaker.CircuitBreaker: 熔断器实例
func (sd *ServiceDiscovery) GetBreaker(serviceName string) *gobreaker.CircuitBreaker {
	return sd.ensureBreaker(serviceName)
}

// portString 将端口号转换为字符串
// 方便地址拼接操作
// 参数:
//   - port: 端口号
// 返回值:
//   - string: 端口字符串
func portString(port int) string {
	return fmt.Sprintf("%d", port)
}