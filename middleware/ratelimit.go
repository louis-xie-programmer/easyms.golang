package middleware

import (
	"easyms/pkg/config"
	"easyms/pkg/entitis"
	"fmt"
	"net"
	"regexp"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type LimiterManager struct {
	ipRules        []entitis.IPLimitRule
	uaRules        []entitis.UALimitRule
	ipLimiters     map[string]*rate.Limiter // CIDR -> limiter
	uaLimiters     map[string]*rate.Limiter // Pattern -> limiter
	defaultLimiter *rate.Limiter
	mu             sync.RWMutex
}

type ConsulRateLimitConfig struct {
	IPLimits []entitis.IPLimitRule `yaml:"ip_limits" json:"ip_limits"`
	UALimits []entitis.UALimitRule `yaml:"ua_limits" json:"ua_limits"`
}

// 初始化限流器管理器
func NewLimiterManager(cfg *ConsulRateLimitConfig, defaultRate float64, defaultBurst int) (*LimiterManager, error) {
	lm := &LimiterManager{
		ipLimiters:     make(map[string]*rate.Limiter),
		uaLimiters:     make(map[string]*rate.Limiter),
		defaultLimiter: rate.NewLimiter(rate.Limit(defaultRate), defaultBurst),
	}
	// 解析IP规则
	for _, rule := range cfg.IPLimits {
		_, ipnet, err := net.ParseCIDR(rule.CIDR)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR: %s", rule.CIDR)
		}
		r := rule
		r.Net = ipnet
		lm.ipRules = append(lm.ipRules, r)
		lm.ipLimiters[rule.CIDR] = rate.NewLimiter(rate.Limit(rule.Rate), rule.Burst)
	}
	// 解析UA规则
	for _, rule := range cfg.UALimits {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid UA regex: %s", rule.Pattern)
		}
		r := rule
		r.Regexp = re
		lm.uaRules = append(lm.uaRules, r)
		lm.uaLimiters[rule.Pattern] = rate.NewLimiter(rate.Limit(rule.Rate), rule.Burst)
	}
	return lm, nil
}

// 基于 config.GetAppConfig() 实时维护限流规则
func (lm *LimiterManager) SyncFromAppConfig() {
	cfg := config.GetAppConfig()
	if cfg == nil || cfg.RateLimit == nil {
		return
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.ipRules = nil
	lm.uaRules = nil
	lm.ipLimiters = make(map[string]*rate.Limiter)
	lm.uaLimiters = make(map[string]*rate.Limiter)
	for _, rule := range cfg.RateLimit.IPLimits {
		_, ipnet, err := net.ParseCIDR(rule.CIDR)
		if err != nil {
			continue
		}
		r := rule
		r.Net = ipnet
		lm.ipRules = append(lm.ipRules, r)
		lm.ipLimiters[rule.CIDR] = rate.NewLimiter(rate.Limit(rule.Rate), rule.Burst)
	}
	for _, rule := range cfg.RateLimit.UALimits {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		r := rule
		r.Regexp = re
		lm.uaRules = append(lm.uaRules, r)
		lm.uaLimiters[rule.Pattern] = rate.NewLimiter(rate.Limit(rule.Rate), rule.Burst)
	}
	if cfg.RateLimit.DefaultRate > 0 && cfg.RateLimit.DefaultBurst > 0 {
		lm.defaultLimiter = rate.NewLimiter(rate.Limit(cfg.RateLimit.DefaultRate), cfg.RateLimit.DefaultBurst)
	}
}

// Gin限流中间件
func (lm *LimiterManager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		ua := c.Request.UserAgent()
		lm.mu.RLock()
		defer lm.mu.RUnlock()
		// 1. IP段匹配
		for _, rule := range lm.ipRules {
			if rule.Net.Contains(net.ParseIP(ip)) {
				limiter := lm.ipLimiters[rule.CIDR]
				if !limiter.Allow() {
					c.AbortWithStatusJSON(429, gin.H{"error": "ip rate limit"})
					return
				}
				break
			}
		}
		// 2. UserAgent匹配
		for _, rule := range lm.uaRules {
			if rule.Regexp.MatchString(ua) {
				limiter := lm.uaLimiters[rule.Pattern]
				if !limiter.Allow() {
					c.AbortWithStatusJSON(429, gin.H{"error": "ua rate limit"})
					return
				}
				break
			}
		}
		// 3. 默认限流
		if lm.defaultLimiter != nil && !lm.defaultLimiter.Allow() {
			c.AbortWithStatusJSON(429, gin.H{"error": "default rate limit"})
			return
		}
		c.Next()
	}
}
