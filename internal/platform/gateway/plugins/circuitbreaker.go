// Package plugins contains all gateway plugins.
package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/models"
	"fmt"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/mercari/go-circuitbreaker"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"net/http"
	"sync"
	"time"
)

// Prometheus metrics
var (
	circuitBreakerOpenTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_circuitbreaker_open_total",
			Help: "Circuit breaker open rejections",
		},
		[]string{"service"},
	)
	circuitBreakerFailureTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_circuitbreaker_failures_total",
			Help: "Circuit breaker protected failures",
		},
		[]string{"service"},
	)
	circuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_circuitbreaker_state",
			Help: "Circuit breaker state by service (1=open, 0=closed)",
		},
		[]string{"service", "state"},
	)
	circuitBreakerStateChangeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_circuitbreaker_state_changes_total",
			Help: "Circuit breaker state transitions",
		},
		[]string{"service", "from", "to"},
	)
	cbStateMu        sync.Mutex
	cbStateByService = map[string]string{}
)

func init() {
	prometheus.MustRegister(circuitBreakerOpenTotal)
	prometheus.MustRegister(circuitBreakerFailureTotal)
	prometheus.MustRegister(circuitBreakerState)
	prometheus.MustRegister(circuitBreakerStateChangeTotal)
}

const defaultBreakerCacheSize = 256

type CircuitBreakerPlugin struct {
	config       *models.CircuitBreakerConfig
	breakerCache *lru.Cache[string, *circuitbreaker.CircuitBreaker]
	mu           sync.Mutex
}

func NewCircuitBreakerPlugin(config *models.CircuitBreakerConfig) *CircuitBreakerPlugin {
	if config == nil {
		config = &models.CircuitBreakerConfig{
			Services: make(map[string]*models.CircuitBreakerServiceConfig),
		}
	}

	cache, err := lru.New[string, *circuitbreaker.CircuitBreaker](defaultBreakerCacheSize)
	if err != nil {
		log.Fatalf("failed to create circuit breaker cache: %v", err)
	}

	return &CircuitBreakerPlugin{
		config:       config,
		breakerCache: cache,
	}
}

func (p *CircuitBreakerPlugin) Name() string {
	return "circuitbreaker"
}

func (p *CircuitBreakerPlugin) Order() int {
	return 40
}

func setCircuitBreakerState(service string, state string) {
	cbStateMu.Lock()
	prev := cbStateByService[service]
	if prev == "" {
		prev = "unknown"
	}
	if prev != state {
		circuitBreakerStateChangeTotal.WithLabelValues(service, prev, state).Inc()
		cbStateByService[service] = state
	}
	cbStateMu.Unlock()

	if state == "open" {
		circuitBreakerState.WithLabelValues(service, "open").Set(1)
		circuitBreakerState.WithLabelValues(service, "closed").Set(0)
		return
	}
	circuitBreakerState.WithLabelValues(service, "open").Set(0)
	circuitBreakerState.WithLabelValues(service, "closed").Set(1)
}

const (
	UpstreamErrorKey = "upstreamError"
)

func (p *CircuitBreakerPlugin) Execute(ctx *plugin.Context) {
	serviceName, ok := ctx.Get(ServiceNameKey)
	if !ok {
		ctx.Next()
		return
	}

	cb := p.getBreaker(serviceName.(string))
	operation := func() (interface{}, error) {
		ctx.Next()
		if err, exists := ctx.Get(UpstreamErrorKey); exists && err != nil {
			return nil, err.(error)
		}
		setCircuitBreakerState(serviceName.(string), "closed")
		return nil, nil
	}

	_, err := cb.Do(ctx.Request.Context(), operation)
	if err != nil {
		if err == circuitbreaker.ErrOpen {
			circuitBreakerOpenTotal.WithLabelValues(serviceName.(string)).Inc()
			setCircuitBreakerState(serviceName.(string), "open")
			http.Error(ctx.ResponseWriter, fmt.Sprintf("service '%s' unavailable (circuit open)", serviceName), http.StatusServiceUnavailable)
		} else {
			circuitBreakerFailureTotal.WithLabelValues(serviceName.(string)).Inc()
			setCircuitBreakerState(serviceName.(string), "closed")
			http.Error(ctx.ResponseWriter, fmt.Sprintf("upstream error from '%s': %v", serviceName, err), http.StatusBadGateway)
		}
		return
	}
}

func (p *CircuitBreakerPlugin) getBreaker(serviceName string) *circuitbreaker.CircuitBreaker {
	if cb, ok := p.breakerCache.Get(serviceName); ok {
		return cb
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if cb, ok := p.breakerCache.Get(serviceName); ok {
		return cb
	}

	var newBreaker *circuitbreaker.CircuitBreaker
	if serviceConfig, ok := p.config.Services[serviceName]; ok {
		newBreaker = circuitbreaker.New(
			circuitbreaker.WithCounterResetInterval(time.Duration(serviceConfig.CounterResetInterval)*time.Second),
			circuitbreaker.WithHalfOpenMaxSuccesses(serviceConfig.HalfOpenMaxSuccesses),
			circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(
				serviceConfig.FailureRateWindow,
				serviceConfig.FailureRateThreshold,
			)),
		)
	} else {
		newBreaker = circuitbreaker.New(
			circuitbreaker.WithCounterResetInterval(10*time.Second),
			circuitbreaker.WithHalfOpenMaxSuccesses(2),
			circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(10, 0.5)),
		)
	}

	p.breakerCache.Add(serviceName, newBreaker)
	return newBreaker
}
