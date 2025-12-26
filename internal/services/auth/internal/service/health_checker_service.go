package service

import (
	"context"
	"easyms/internal/shared/db"
	"time"
)

// HealthStatus 健康检查状态
type HealthStatus struct {
	Status string              `json:"status"`
	Checks []HealthCheckResult `json:"checks,omitempty"`
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Message string  `json:"message,omitempty"`
	Latency float64 `json:"latency_ms,omitempty"`
}

// HealthChecker 健康检查器
type HealthCheckerService struct {
	db db.Database
}

// NewHealthChecker 创建健康检查器
func NewHealthCheckerService(database db.Database) *HealthCheckerService {
	return &HealthCheckerService{db: database}
}

// CheckHealth 执行健康检查
func (hc *HealthCheckerService) CheckHealth() *HealthStatus {
	status := &HealthStatus{
		Status: "pass",
		Checks: make([]HealthCheckResult, 0),
	}

	// 检查数据库连接
	dbCheck := hc.checkDatabase()
	if dbCheck.Status == "fail" {
		status.Status = "fail"
	}
	status.Checks = append(status.Checks, dbCheck)

	return status
}

// checkDatabase 检查数据库连接
func (hc *HealthCheckerService) checkDatabase() HealthCheckResult {
	start := time.Now()
	// 尝试执行简单查询
	_, err := hc.db.Count(context.Background(), "SELECT 1")

	latency := time.Since(start).Seconds() * 1000

	result := HealthCheckResult{
		Name:    "database",
		Status:  "pass",
		Latency: latency,
	}

	if err != nil {
		result.Status = "fail"
		result.Message = err.Error()
	}

	return result
}
