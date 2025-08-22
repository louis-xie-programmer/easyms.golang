package cb

import (
	"context"
	"errors"
	"fmt"
	"github.com/benbjohnson/clock"
	"github.com/go-kit/kit/endpoint"
	gcb "github.com/mercari/go-circuitbreaker"
	"time"
)

type CircuitBreakerConfig struct {
	CounterResetInterval time.Duration
	HalfOpenMaxSuccesses int
	FailureRateWindow    int
	FailureRateThreshold float64
	Name                 string
}

func NewCircuitBreakerWithConfig(cfg CircuitBreakerConfig) *gcb.CircuitBreaker {
	cb := gcb.New(
		gcb.WithClock(clock.New()),
		gcb.WithCounterResetInterval(cfg.CounterResetInterval),
		gcb.WithHalfOpenMaxSuccesses(int64(cfg.HalfOpenMaxSuccesses)),
		gcb.WithTripFunc(
			gcb.NewTripFuncFailureRate(10, 0.4),
		),
		gcb.WithOnStateChangeHookFn(func(from, to gcb.State) {
			// 可替换为 logger 或埋点
			fmt.Printf("[CB][%s] 状态变更: %s -> %s\n", cfg.Name, from, to)
		}),
	)
	return cb
}

// WrapEndpoint Endpoint 熔断
func WrapEndpoint(name string, e endpoint.Endpoint) endpoint.Endpoint {
	br := NewCircuitBreakerWithConfig(CircuitBreakerConfig{
		CounterResetInterval: 10 * time.Second,
		HalfOpenMaxSuccesses: 4,
		FailureRateWindow:    10,
		FailureRateThreshold: 0.4,
		Name:                 name,
	})
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		resp, err := br.Do(ctx, func() (interface{}, error) {
			return e(ctx, request)
		})
		if err != nil {
			if errors.Is(err, gcb.ErrOpen) {
				return nil, fmt.Errorf("服务 %s 熔断中", name)
			}
			return nil, err
		}
		return resp, nil
	}
}
