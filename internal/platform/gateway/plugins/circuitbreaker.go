package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/models"
	"fmt"
	lru "github.com/hashicorp/golang-lru/v2" // Use the thread-safe v2 root package
	"github.com/mercari/go-circuitbreaker"
	"log"
	"net/http"
	"sync"
	"time"
)

const defaultBreakerCacheSize = 256

// CircuitBreakerPlugin provides circuit breaking functionality for upstream services.
type CircuitBreakerPlugin struct {
	config       *models.CircuitBreakerConfig
	breakerCache *lru.Cache[string, *circuitbreaker.CircuitBreaker] // Thread-safe generic LRU
	mu           sync.Mutex
}

// NewCircuitBreakerPlugin creates a new circuit breaker plugin from the given configuration.
func NewCircuitBreakerPlugin(config *models.CircuitBreakerConfig) *CircuitBreakerPlugin {
	if config == nil {
		config = &models.CircuitBreakerConfig{
			Services: make(map[string]*models.CircuitBreakerServiceConfig),
		}
	}

	// Create a thread-safe LRU cache with generics
	cache, err := lru.New[string, *circuitbreaker.CircuitBreaker](defaultBreakerCacheSize)
	if err != nil {
		log.Fatalf("Failed to create LRU cache for circuit breakers: %v", err)
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

const (
	UpstreamErrorKey = "upstreamError"
	ServiceNameKey   = "serviceName"
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
		return nil, nil
	}

	_, err := cb.Do(ctx.Request.Context(), operation)

	if err != nil {
		if err == circuitbreaker.ErrOpen {
			http.Error(ctx.ResponseWriter, fmt.Sprintf("service '%s' is unavailable (circuit open)", serviceName), http.StatusServiceUnavailable)
		} else {
			http.Error(ctx.ResponseWriter, fmt.Sprintf("error from service '%s': %v", serviceName, err), http.StatusBadGateway)
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
