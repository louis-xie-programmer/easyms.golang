package discovery

import (
	"math/rand"
	"sort"
	"sync"
	"time"
)

// ServiceInstance 服务实例信息，包含地址和元数据
type ServiceInstance struct {
	Address  string
	Weight   int
	Metadata map[string]string
}

// WeightedLoadBalancer 权重负载均衡器
type WeightedLoadBalancer struct {
	// 存储每个实例的权重和元数据
	instances map[string]*ServiceInstance
	mutex     sync.RWMutex
}

// NewWeightedLoadBalancer 创建权重负载均衡器
func NewWeightedLoadBalancer() *WeightedLoadBalancer {
	return &WeightedLoadBalancer{
		instances: make(map[string]*ServiceInstance),
	}
}

// AddInstance 添加服务实例
func (w *WeightedLoadBalancer) AddInstance(address string, weight int, metadata map[string]string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	w.instances[address] = &ServiceInstance{
		Address:  address,
		Weight:   weight,
		Metadata: metadata,
	}
}

// RemoveInstance 移除服务实例
func (w *WeightedLoadBalancer) RemoveInstance(address string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	
	delete(w.instances, address)
}

// Select 根据权重选择实例
func (w *WeightedLoadBalancer) Select(instances []string) string {
	if len(instances) == 0 {
		return ""
	}

	w.mutex.RLock()
	defer w.mutex.RUnlock()

	// 如果没有配置权重信息，使用随机选择
	if len(w.instances) == 0 {
		rand.Seed(time.Now().UnixNano())
		return instances[rand.Intn(len(instances))]
	}

	// 构建权重列表
	type weightedInstance struct {
		address string
		weight  int
	}
	
	var weightedInstances []weightedInstance
	totalWeight := 0
	
	for _, addr := range instances {
		if instance, exists := w.instances[addr]; exists {
			weightedInstances = append(weightedInstances, weightedInstance{
				address: addr,
				weight:  instance.Weight,
			})
			totalWeight += instance.Weight
		} else {
			// 如果没有配置权重，默认权重为1
			weightedInstances = append(weightedInstances, weightedInstance{
				address: addr,
				weight:  1,
			})
			totalWeight += 1
		}
	}
	
	// 如果总权重为0，随机选择
	if totalWeight <= 0 {
		rand.Seed(time.Now().UnixNano())
		return instances[rand.Intn(len(instances))]
	}
	
	// 按权重排序
	sort.Slice(weightedInstances, func(i, j int) bool {
		return weightedInstances[i].weight > weightedInstances[j].weight
	})
	
	// 权重随机选择算法
	rand.Seed(time.Now().UnixNano())
	randomWeight := rand.Intn(totalWeight)
	
	currentWeight := 0
	for _, wi := range weightedInstances {
		currentWeight += wi.weight
		if randomWeight < currentWeight {
			return wi.address
		}
	}
	
	// fallback to first instance
	return instances[0]
}

// GetInstanceMetadata 获取实例元数据
func (w *WeightedLoadBalancer) GetInstanceMetadata(address string) map[string]string {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	
	if instance, exists := w.instances[address]; exists {
		return instance.Metadata
	}
	
	return nil
}

// LeastConnectionWeightedLoadBalancer 带权重的最少连接负载均衡器
type LeastConnectionWeightedLoadBalancer struct {
	connections map[string]int32
	weights     map[string]int
	mutex       sync.Mutex
}

// NewLeastConnectionWeightedLoadBalancer 创建带权重的最少连接负载均衡器
func NewLeastConnectionWeightedLoadBalancer() *LeastConnectionWeightedLoadBalancer {
	return &LeastConnectionWeightedLoadBalancer{
		connections: make(map[string]int32),
		weights:     make(map[string]int),
	}
}

// SetWeights 设置实例权重
func (l *LeastConnectionWeightedLoadBalancer) SetWeights(weights map[string]int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	
	for addr, weight := range weights {
		l.weights[addr] = weight
	}
}

// Select 根据连接数和权重选择实例
func (l *LeastConnectionWeightedLoadBalancer) Select(instances []string) string {
	if len(instances) == 0 {
		return ""
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	// 计算每个实例的连接数与权重的比率
	minRatio := float64(-1)
	selectedInstance := instances[0]

	for _, instance := range instances {
		connections := l.connections[instance]
		weight := l.weights[instance]
		if weight <= 0 {
			weight = 1 // 默认权重
		}
		
		ratio := float64(connections) / float64(weight)
		
		if minRatio == -1 || ratio < minRatio {
			minRatio = ratio
			selectedInstance = instance
		}
	}

	// 增加选中实例的连接数
	l.connections[selectedInstance]++

	return selectedInstance
}

// ReleaseConnection 释放连接（减少实例的连接计数）
func (l *LeastConnectionWeightedLoadBalancer) ReleaseConnection(instance string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if connections, exists := l.connections[instance]; exists && connections > 0 {
		l.connections[instance] = connections - 1
	}
}