package middleware

import (
	"fmt"
	"time"

	"github.com/mercari/go-circuitbreaker"
)

// 熔断器配置结构体
type CircuitBreakerConfig struct {
	CounterResetInterval time.Duration
	HalfOpenMaxSuccesses int64
	FailureRateWindow    int64
	FailureRateThreshold float64
	Name                 string // 用于监控标识
}

// 熔断器初始化（支持配置和状态变更监控）
func NewCircuitBreakerWithConfig(cfg CircuitBreakerConfig) *circuitbreaker.CircuitBreaker {
	cb := circuitbreaker.New(
		circuitbreaker.WithCounterResetInterval(cfg.CounterResetInterval),
		circuitbreaker.WithHalfOpenMaxSuccesses(cfg.HalfOpenMaxSuccesses),
		circuitbreaker.WithTripFunc(
			circuitbreaker.NewTripFuncFailureRate(cfg.FailureRateWindow, cfg.FailureRateThreshold),
		),
		circuitbreaker.WithOnStateChangeHookFn(func(from, to circuitbreaker.State) {
			// 可替换为 logger 或埋点
			fmt.Printf("[CB][%s] 状态变更: %s -> %s\n", cfg.Name, from, to)
		}),
	)
	return cb
}

// 默认熔断器（兼容旧用法）
func NewCircuitBreaker() *circuitbreaker.CircuitBreaker {
	return NewCircuitBreakerWithConfig(CircuitBreakerConfig{
		CounterResetInterval: 10 * time.Second,
		HalfOpenMaxSuccesses: 4,
		FailureRateWindow:    10,
		FailureRateThreshold: 0.4,
		Name:                 "default",
	})
}
