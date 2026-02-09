package discovery

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// HealthChecker 健康检查器接口
type HealthChecker interface {
	Check(address string) bool
}

// NoopHealthChecker always reports healthy.
type NoopHealthChecker struct{}

func (n *NoopHealthChecker) Check(address string) bool {
	return true
}

// HTTPHealthChecker HTTP健康检查器
type HTTPHealthChecker struct {
	client *http.Client
	path   string
}

// NewHTTPHealthChecker 创建HTTP健康检查器
func NewHTTPHealthChecker(timeout time.Duration, healthPath string) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		client: &http.Client{
			Timeout: timeout,
		},
		path: healthPath,
	}
}

// Check 执行HTTP健康检查
func (h *HTTPHealthChecker) Check(address string) bool {
	url := "http://" + address + h.path
	resp, err := h.client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// TCPHealthChecker TCP健康检查器
type TCPHealthChecker struct {
	timeout time.Duration
}

// NewTCPHealthChecker 创建TCP健康检查器
func NewTCPHealthChecker(timeout time.Duration) *TCPHealthChecker {
	return &TCPHealthChecker{
		timeout: timeout,
	}
}

// Check 执行TCP健康检查
func (t *TCPHealthChecker) Check(address string) bool {
	conn, err := net.DialTimeout("tcp", address, t.timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ServiceHealthManager 服务健康管理器
type ServiceHealthManager struct {
	checker       HealthChecker
	serviceHealth map[string]bool
	mutex         sync.RWMutex
}

// NewServiceHealthManager 创建服务健康管理器
func NewServiceHealthManager(checker HealthChecker) *ServiceHealthManager {
	return &ServiceHealthManager{
		checker:       checker,
		serviceHealth: make(map[string]bool),
	}
}

// UpdateHealthStatus 更新服务健康状态
func (s *ServiceHealthManager) UpdateHealthStatus(address string, healthy bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.serviceHealth[address] = healthy
}

// IsHealthy 检查服务是否健康
func (s *ServiceHealthManager) IsHealthy(address string) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if healthy, exists := s.serviceHealth[address]; exists {
		return healthy
	}

	// 如果没有记录，则执行一次健康检查
	healthy := s.checker.Check(address)
	s.serviceHealth[address] = healthy
	return healthy
}

// HealthCheckWorker 健康检查工作协程
func (s *ServiceHealthManager) HealthCheckWorker(addresses []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		for _, addr := range addresses {
			healthy := s.checker.Check(addr)
			s.UpdateHealthStatus(addr, healthy)

			if !healthy {
				log.Printf("Service %s is unhealthy", addr)
			}
		}
	}
}

// SetHealthChecker 设置健康检查器
func (s *ServiceHealthManager) SetHealthChecker(checker HealthChecker) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.checker = checker
}

// GetHealthChecker 获取健康检查器
func (s *ServiceHealthManager) GetHealthChecker() HealthChecker {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.checker
}
