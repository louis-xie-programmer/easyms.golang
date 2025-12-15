package discovery

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalanceStrategy 负载均衡策略接口
type LoadBalanceStrategy interface {
	Select(instances []string) string
}

// RoundRobinLoadBalancer 轮询负载均衡器
type RoundRobinLoadBalancer struct {
	index int64
}

func (r *RoundRobinLoadBalancer) Select(instances []string) string {
	if len(instances) == 0 {
		return ""
	}

	index := atomic.AddInt64(&r.index, 1)
	return instances[(index-1)%int64(len(instances))]
}

// RandomLoadBalancer 随机负载均衡器
type RandomLoadBalancer struct{}

func (r *RandomLoadBalancer) Select(instances []string) string {
	if len(instances) == 0 {
		return ""
	}

	rand.Seed(time.Now().UnixNano())
	return instances[rand.Intn(len(instances))]
}

// SimpleWeightedLoadBalancer 简单权重负载均衡器
type SimpleWeightedLoadBalancer struct {
	// 这里应该从服务元数据中获取权重信息
	// 为了简化，我们假设所有实例具有相同权重
}

func (w *SimpleWeightedLoadBalancer) Select(instances []string) string {
	if len(instances) == 0 {
		return ""
	}

	// 简单实现：随机选择（在真实场景中会基于权重选择）
	rand.Seed(time.Now().UnixNano())
	return instances[rand.Intn(len(instances))]
}

// LeastConnectionLoadBalancer 最少连接负载均衡器
// 注意：这需要跟踪每个实例的连接数，这里只是示意实现
type LeastConnectionLoadBalancer struct {
	connections map[string]int32
	mutex       sync.Mutex
}

func NewLeastConnectionLoadBalancer() *LeastConnectionLoadBalancer {
	return &LeastConnectionLoadBalancer{
		connections: make(map[string]int32),
	}
}

func (l *LeastConnectionLoadBalancer) Select(instances []string) string {
	if len(instances) == 0 {
		return ""
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	minConnections := int32(-1)
	selectedInstance := instances[0]

	for _, instance := range instances {
		connections := l.connections[instance]
		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selectedInstance = instance
		}
	}

	// 增加选中实例的连接数
	l.connections[selectedInstance]++

	return selectedInstance
}

// ReleaseConnection 释放连接（减少实例的连接计数）
func (l *LeastConnectionLoadBalancer) ReleaseConnection(instance string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if connections, exists := l.connections[instance]; exists && connections > 0 {
		l.connections[instance] = connections - 1
	}
}

// IPHashLoadBalancer IP哈希负载均衡器
// 根据客户端IP地址进行哈希，确保同一IP总是路由到同一实例
type IPHashLoadBalancer struct{}

func (i *IPHashLoadBalancer) Select(instances []string) string {
	// 注意：这个实现需要从上下文中获取客户端IP
	// 在实际使用中，需要将客户端IP传递给负载均衡器
	// 这里只是一个示意实现
	if len(instances) == 0 {
		return ""
	}

	// 简单的哈希实现（在实际应用中应该使用客户端IP）
	rand.Seed(time.Now().UnixNano())
	hash := rand.Intn(len(instances))
	return instances[hash]
}

// MetadataBasedLoadBalancer 基于元数据的负载均衡器
type MetadataBasedLoadBalancer struct {
	metadataKey   string
	metadataValue string
}

// NewMetadataBasedLoadBalancer 创建基于元数据的负载均衡器
func NewMetadataBasedLoadBalancer(metadataKey, metadataValue string) *MetadataBasedLoadBalancer {
	return &MetadataBasedLoadBalancer{
		metadataKey:   metadataKey,
		metadataValue: metadataValue,
	}
}

// Select 根据元数据选择实例
func (m *MetadataBasedLoadBalancer) Select(instances []string) string {
	// 这个实现需要访问服务发现中的元数据
	// 由于接口限制，这里简化实现
	if len(instances) == 0 {
		return ""
	}

	// 如果没有元数据匹配要求，随机选择
	rand.Seed(time.Now().UnixNano())
	return instances[rand.Intn(len(instances))]
}