package plugins

import (
	"context"
	"easyms/internal/platform/gateway/plugin"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mercari/go-circuitbreaker"
)

// CircuitBreakerPlugin provides circuit breaking functionality for upstream services.
type CircuitBreakerPlugin struct {
	breakers map[string]*circuitbreaker.CircuitBreaker
	mu       sync.RWMutex
}

// NewCircuitBreakerPlugin creates a new circuit breaker plugin.
func NewCircuitBreakerPlugin() *CircuitBreakerPlugin {
	return &CircuitBreakerPlugin{
		breakers: make(map[string]*circuitbreaker.CircuitBreaker),
	}
}

func (p *CircuitBreakerPlugin) Name() string {
	return "circuitbreaker"
}

func (p *CircuitBreakerPlugin) Order() int {
	return 40
}

const (
	// UpstreamErrorKey is the key to store upstream errors for the circuit breaker to check.
	UpstreamErrorKey = "upstreamError"
)

func (p *CircuitBreakerPlugin) Execute(ctx *plugin.Context) {
	upstreamURL, ok := ctx.Get(UpstreamServiceURLKey)
	if !ok {
		ctx.Next()
		return
	}

	serviceIdentifier := p.getServiceIdentifier(upstreamURL.(string))
	cb := p.getBreaker(serviceIdentifier)

	operation := func() (interface{}, error) {
		ctx.Next()
		if err, exists := ctx.Get(UpstreamErrorKey); exists && err != nil {
			return nil, err.(error)
		}
		return nil, nil
	}

	_, err := cb.Do(context.Background(), operation)

	if err != nil {
		// The proxy plugin is designed not to write the response on error,
		// so we can safely write our own error response here.
		http.Error(ctx.ResponseWriter, fmt.Sprintf("service '%s' unavailable: %v", serviceIdentifier, err), http.StatusServiceUnavailable)
		return
	}
}

func (p *CircuitBreakerPlugin) getServiceIdentifier(urlStr string) string {
	parts := strings.Split(strings.TrimPrefix(urlStr, "http://"), "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown-service"
}

func (p *CircuitBreakerPlugin) getBreaker(serviceIdentifier string) *circuitbreaker.CircuitBreaker {
	p.mu.RLock()
	cb, exists := p.breakers[serviceIdentifier]
	p.mu.RUnlock()

	if exists {
		return cb
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if cb, exists = p.breakers[serviceIdentifier]; exists {
		return cb
	}

	// Create a new circuit breaker with default settings.
	cb = circuitbreaker.New(
		circuitbreaker.WithOpenTimeout(10*time.Second),
		circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(10, 0.5)), // 10 requests window, 50% failure rate
	)
	p.breakers[serviceIdentifier] = cb
	return cb
}
