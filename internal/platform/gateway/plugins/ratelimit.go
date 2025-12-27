package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/models"
	"github.com/hashicorp/golang-lru/v2" // Use the thread-safe v2 root package
	"golang.org/x/time/rate"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

const defaultLimiterCacheSize = 1024

// RateLimitPlugin implements rate limiting based on IP, User-Agent, and a default.
type RateLimitPlugin struct {
	config       *models.RateLimitConfig
	limiterCache *lru.Cache[string, *rate.Limiter] // Thread-safe generic LRU
	mu           sync.Mutex
}

// NewRateLimitPlugin creates a new rate-limiting plugin from the given configuration.
func NewRateLimitPlugin(config *models.RateLimitConfig) *RateLimitPlugin {
	if config == nil {
		config = &models.RateLimitConfig{
			DefaultRate:  100,
			DefaultBurst: 200,
		}
	}

	for i := range config.IPLimits {
		_, ipNet, err := net.ParseCIDR(config.IPLimits[i].CIDR)
		if err != nil {
			log.Printf("Invalid CIDR in rate limit config, skipping: %s", config.IPLimits[i].CIDR)
			continue
		}
		config.IPLimits[i].Net = ipNet
	}
	for i := range config.UALimits {
		re, err := regexp.Compile(config.UALimits[i].Pattern)
		if err != nil {
			log.Printf("Invalid regex in rate limit config, skipping: %s", config.UALimits[i].Pattern)
			continue
		}
		config.UALimits[i].Regexp = re
	}

	// Create a thread-safe LRU cache with generics
	cache, err := lru.New[string, *rate.Limiter](defaultLimiterCacheSize)
	if err != nil {
		log.Fatalf("Failed to create LRU cache for rate limiters: %v", err)
	}

	return &RateLimitPlugin{
		config:       config,
		limiterCache: cache,
	}
}

func (p *RateLimitPlugin) Name() string {
	return "ratelimit"
}

func (p *RateLimitPlugin) Order() int {
	return 20
}

func (p *RateLimitPlugin) Execute(ctx *plugin.Context) {
	if !p.isAllowed(ctx.Request) {
		http.Error(ctx.ResponseWriter, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	ctx.Next()
}

func (p *RateLimitPlugin) isAllowed(req *http.Request) bool {
	ipStr := p.getClientIP(req)
	ip := net.ParseIP(ipStr)
	ua := req.UserAgent()

	for _, rule := range p.config.UALimits {
		if rule.Regexp != nil && rule.Regexp.MatchString(ua) {
			limiter := p.getLimiter("ua:"+rule.Pattern, rule.Rate, rule.Burst)
			return limiter.Allow()
		}
	}

	if ip != nil {
		for _, rule := range p.config.IPLimits {
			if rule.Net != nil && rule.Net.Contains(ip) {
				limiter := p.getLimiter("ip:"+rule.CIDR, rule.Rate, rule.Burst)
				return limiter.Allow()
			}
		}
	}

	if p.config.DefaultRate > 0 {
		defaultLimiter := p.getLimiter("default", p.config.DefaultRate, p.config.DefaultBurst)
		return defaultLimiter.Allow()
	}

	return true
}

func (p *RateLimitPlugin) getClientIP(req *http.Request) string {
	if fwd := req.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		ipStr := strings.TrimSpace(parts[0])
		if host, _, err := net.SplitHostPort(ipStr); err == nil {
			return host
		}
		return ipStr
	}

	if realIP := req.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		return host
	}
	return req.RemoteAddr
}

func (p *RateLimitPlugin) getLimiter(key string, r float64, b int) *rate.Limiter {
	// lru.Cache is thread-safe, so Get is safe.
	if limiter, ok := p.limiterCache.Get(key); ok {
		return limiter
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check locking
	if limiter, ok := p.limiterCache.Get(key); ok {
		return limiter
	}

	newLimiter := rate.NewLimiter(rate.Limit(r), b)
	p.limiterCache.Add(key, newLimiter)
	return newLimiter
}
