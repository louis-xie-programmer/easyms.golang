package discovery

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AdvancedHealthChecker 高级健康检查器
type AdvancedHealthChecker struct {
	httpClient  *http.Client
	tcpTimeout  time.Duration
	customCheck CustomHealthCheckFunc
	checkType   HealthCheckType
	path        string
}

// HealthCheckType 健康检查类型
type HealthCheckType string

const (
	HTTPHealthCheck HealthCheckType = "http"
	TCPHealthCheck  HealthCheckType = "tcp"
	CustomHealthCheck HealthCheckType = "custom"
)

// CustomHealthCheckFunc 自定义健康检查函数
type CustomHealthCheckFunc func(address string) bool

// NewAdvancedHTTPHealthChecker 创建高级HTTP健康检查器
func NewAdvancedHTTPHealthChecker(timeout time.Duration, healthPath string) *AdvancedHealthChecker {
	return &AdvancedHealthChecker{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		tcpTimeout: timeout,
		checkType:  HTTPHealthCheck,
		path:       healthPath,
	}
}

// NewAdvancedTCPHealthChecker 创建高级TCP健康检查器
func NewAdvancedTCPHealthChecker(timeout time.Duration) *AdvancedHealthChecker {
	return &AdvancedHealthChecker{
		tcpTimeout: timeout,
		checkType:  TCPHealthCheck,
	}
}

// NewAdvancedCustomHealthChecker 创建高级自定义健康检查器
func NewAdvancedCustomHealthChecker(checkFunc CustomHealthCheckFunc) *AdvancedHealthChecker {
	return &AdvancedHealthChecker{
		customCheck: checkFunc,
		checkType:   CustomHealthCheck,
	}
}

// Check 执行健康检查
func (a *AdvancedHealthChecker) Check(address string) bool {
	switch a.checkType {
	case HTTPHealthCheck:
		return a.httpCheck(address)
	case TCPHealthCheck:
		return a.tcpCheck(address)
	case CustomHealthCheck:
		return a.customCheck(address)
	default:
		// 默认使用TCP检查
		return a.tcpCheck(address)
	}
}

// httpCheck HTTP健康检查
func (a *AdvancedHealthChecker) httpCheck(address string) bool {
	// 构造完整的URL
	var url string
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		url = address + a.path
	} else {
		url = "http://" + address + a.path
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), a.httpClient.Timeout)
	defer cancel()

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	// 发送请求
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 检查状态码
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// tcpCheck TCP健康检查
func (a *AdvancedHealthChecker) tcpCheck(address string) bool {
	// 解析地址，如果没有端口则添加默认端口
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// 如果地址不包含端口，使用默认端口80
		host = address
		port = "80"
	}

	// 构造完整的地址
	fullAddress := net.JoinHostPort(host, port)

	// 创建带超时的连接
	conn, err := net.DialTimeout("tcp", fullAddress, a.tcpTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// MultiHealthChecker 多种健康检查组合器
type MultiHealthChecker struct {
	checkers []HealthChecker
	strategy MultiCheckStrategy
}

// MultiCheckStrategy 多检查策略
type MultiCheckStrategy string

const (
	AllHealthy   MultiCheckStrategy = "all"   // 所有检查都通过才算健康
	AnyHealthy   MultiCheckStrategy = "any"   // 任何一个检查通过就算健康
	MajorityHealthy MultiCheckStrategy = "majority" // 大多数检查通过才算健康
)

// NewMultiHealthChecker 创建多健康检查器
func NewMultiHealthChecker(strategy MultiCheckStrategy, checkers ...HealthChecker) *MultiHealthChecker {
	return &MultiHealthChecker{
		checkers: checkers,
		strategy: strategy,
	}
}

// Check 执行多重健康检查
func (m *MultiHealthChecker) Check(address string) bool {
	if len(m.checkers) == 0 {
		return true // 没有检查器，默认健康
	}

	healthyChecks := 0
	for _, checker := range m.checkers {
		if checker.Check(address) {
			healthyChecks++
		}
	}

	switch m.strategy {
	case AllHealthy:
		return healthyChecks == len(m.checkers)
	case AnyHealthy:
		return healthyChecks > 0
	case MajorityHealthy:
		return healthyChecks > len(m.checkers)/2
	default:
		return healthyChecks == len(m.checkers)
	}
}

// AdaptiveHealthChecker 自适应健康检查器
type AdaptiveHealthChecker struct {
	primaryChecker   HealthChecker
	secondaryChecker HealthChecker
	failureThreshold int
	failureCount     map[string]int
	mutex            sync.RWMutex
}

// NewAdaptiveHealthChecker 创建自适应健康检查器
func NewAdaptiveHealthChecker(primary, secondary HealthChecker, failureThreshold int) *AdaptiveHealthChecker {
	return &AdaptiveHealthChecker{
		primaryChecker:   primary,
		secondaryChecker: secondary,
		failureThreshold: failureThreshold,
		failureCount:     make(map[string]int),
	}
}

// Check 执行自适应健康检查
func (a *AdaptiveHealthChecker) Check(address string) bool {
	a.mutex.RLock()
	failures := a.failureCount[address]
	a.mutex.RUnlock()

	// 如果失败次数超过阈值，使用备用检查器
	if failures >= a.failureThreshold {
		return a.secondaryChecker.Check(address)
	}

	// 使用主检查器
	result := a.primaryChecker.Check(address)
	
	// 更新失败计数
	a.mutex.Lock()
	if result {
		a.failureCount[address] = 0 // 重置失败计数
	} else {
		a.failureCount[address]++
	}
	a.mutex.Unlock()
	
	return result
}

// ReportFailure 报告检查失败
func (a *AdaptiveHealthChecker) ReportFailure(address string) {
	a.mutex.Lock()
	a.failureCount[address]++
	a.mutex.Unlock()
}

// ReportSuccess 报告检查成功
func (a *AdaptiveHealthChecker) ReportSuccess(address string) {
	a.mutex.Lock()
	a.failureCount[address] = 0
	a.mutex.Unlock()
}