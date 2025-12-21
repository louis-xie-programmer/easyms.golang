package middleware

import (
	"easyms/internal/platform/gateway/internal/domain/model"
	"fmt"
	"net"
	"regexp"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type LimiterManager struct {
	ipRules        []model.IPLimitRule
	uaRules        []model.UALimitRule
	ipLimiters     map[string]*rate.Limiter // CIDR -> limiter
	uaLimiters     map[string]*rate.Limiter // Pattern -> limiter
	defaultLimiter *rate.Limiter
	mu             sync.RWMutex
}

type ConsulRateLimitConfig struct {
	IPLimits []model.IPLimitRule `yaml:"ip_limits" json:"ip_limits"`
	UALimits []model.UALimitRule `yaml:"ua_limits" json:"ua_limits"`
}

// NewLimiterManager 初始化限流器管理器
// 已经弃用
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
