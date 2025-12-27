package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"golang.org/x/time/rate"
	"net/http"
	"strings"
	"sync"
)

// RateLimitConfig 定义了限流插件的配置结构。
type RateLimitConfig struct {
	DefaultRate  float64
	DefaultBurst int
	IPLimits     map[string]struct {
		Rate  float64
		Burst int
	}
	UALimits map[string]struct {
		Rate  float64
		Burst int
	}
}

// RateLimitPlugin 实现了基于 IP、User-Agent 和默认值的限流功能。
type RateLimitPlugin struct {
	config       *RateLimitConfig
	rateLimiters map[string]*rate.Limiter
	mu           sync.RWMutex
}

// NewRateLimitPlugin 创建一个新的限流插件。
func NewRateLimitPlugin(config *RateLimitConfig) *RateLimitPlugin {
	if config == nil {
		// 提供一个默认配置，以防万一
		config = &RateLimitConfig{
			DefaultRate:  100,
			DefaultBurst: 200,
		}
	}
	return &RateLimitPlugin{
		config:       config,
		rateLimiters: make(map[string]*rate.Limiter),
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
		return // 超出限流，中断插件链
	}
	ctx.Next() // 未超出，继续下一个插件
}

// isAllowed 检查请求是否允许通过。
func (p *RateLimitPlugin) isAllowed(req *http.Request) bool {
	ip := req.RemoteAddr
	ua := req.UserAgent()

	// IP限流
	if p.config.IPLimits != nil {
		for cidr, limit := range p.config.IPLimits {
			if strings.HasPrefix(ip, cidr) {
				limiter := p.getLimiter("ip:"+cidr, limit.Rate, limit.Burst)
				if !limiter.Allow() {
					return false
				}
				break // 一个IP只匹配一个规则
			}
		}
	}

	// User-Agent限流
	if p.config.UALimits != nil {
		for pattern, limit := range p.config.UALimits {
			if strings.Contains(ua, pattern) {
				limiter := p.getLimiter("ua:"+pattern, limit.Rate, limit.Burst)
				if !limiter.Allow() {
					return false
				}
				break // 一个UA只匹配一个规则
			}
		}
	}

	// 默认限流
	if p.config.DefaultRate > 0 && p.config.DefaultBurst > 0 {
		defaultLimiter := p.getLimiter("default", p.config.DefaultRate, p.config.DefaultBurst)
		return defaultLimiter.Allow()
	}

	return true
}

// getLimiter 获取或创建一个新的限流器。
func (p *RateLimitPlugin) getLimiter(key string, r float64, b int) *rate.Limiter {
	p.mu.RLock()
	limiter, exists := p.rateLimiters[key]
	p.mu.RUnlock()

	if exists {
		return limiter
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Double-check locking
	if limiter, exists = p.rateLimiters[key]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rate.Limit(r), b)
	p.rateLimiters[key] = limiter
	return limiter
}
