// Package plugins contains all gateway plugins.
package plugins

import (
	"context"
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type RateLimitPlugin struct {
	config      *models.RateLimitConfig
	redisClient db.RedisClient
	limitScript *redis.Script
}

const tokenBucketScript = `
	local tokens_key = KEYS[1]
	local timestamp_key = KEYS[2]
	local rate = tonumber(ARGV[1])
	local capacity = tonumber(ARGV[2])
	local now = tonumber(ARGV[3])
	local requested = tonumber(ARGV[4])

	local fill_time = capacity / rate
	local ttl = math.floor(fill_time * 2)

	local last_tokens = tonumber(redis.call("get", tokens_key))
	if last_tokens == nil then
		last_tokens = capacity
	end

	local last_refreshed = tonumber(redis.call("get", timestamp_key))
	if last_refreshed == nil then
		last_refreshed = 0
	end

	local delta = math.max(0, now - last_refreshed)
	local filled_tokens = math.min(capacity, last_tokens + (delta * rate))

	local allowed = filled_tokens >= requested
	local new_tokens = filled_tokens
	local result = 0
	if allowed then
		new_tokens = filled_tokens - requested
		result = 1
	end

	redis.call("setex", tokens_key, ttl, new_tokens)
	redis.call("setex", timestamp_key, ttl, now)

	return result
`

// Prometheus metrics
var (
	rateLimitBlockedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_ratelimit_blocked_total",
			Help: "Rate limit blocked requests",
		},
		[]string{"rule"},
	)
	rateLimitRedisErrorTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_ratelimit_redis_errors_total",
			Help: "Rate limit Redis script errors",
		},
	)
	rateLimitDisabledTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gateway_ratelimit_disabled_total",
			Help: "Rate limit disabled due to missing Redis",
		},
	)
)

func init() {
	prometheus.MustRegister(rateLimitBlockedTotal)
	prometheus.MustRegister(rateLimitRedisErrorTotal)
	prometheus.MustRegister(rateLimitDisabledTotal)
}

func NewRateLimitPlugin(config *models.RateLimitConfig, redisClient db.RedisClient) *RateLimitPlugin {
	if config == nil {
		config = &models.RateLimitConfig{
			DefaultRate:  100,
			DefaultBurst: 200,
		}
	}

	for i := range config.IPLimits {
		_, ipNet, err := net.ParseCIDR(config.IPLimits[i].CIDR)
		if err != nil {
			logger.Warn("Invalid rate limit CIDR, skipping", "gateway", "cidr", config.IPLimits[i].CIDR)
			continue
		}
		config.IPLimits[i].Net = ipNet
	}
	for i := range config.UALimits {
		re, err := regexp.Compile(config.UALimits[i].Pattern)
		if err != nil {
			logger.Warn("Invalid rate limit regex, skipping", "gateway", "pattern", config.UALimits[i].Pattern)
			continue
		}
		config.UALimits[i].Regexp = re
	}

	return &RateLimitPlugin{
		config:      config,
		redisClient: redisClient,
		limitScript: redis.NewScript(tokenBucketScript),
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
		http.Error(ctx.ResponseWriter, "rate limited", http.StatusTooManyRequests)
		return
	}
	ctx.Next()
}

func (p *RateLimitPlugin) isAllowed(req *http.Request) bool {
	if p.redisClient == nil {
		rateLimitDisabledTotal.Inc()
		logger.Warn("Redis not initialized, rate limit disabled", "gateway")
		return true
	}

	ipStr := p.getClientIP(req)
	ip := net.ParseIP(ipStr)
	ua := req.UserAgent()

	for _, rule := range p.config.UALimits {
		if rule.Regexp != nil && rule.Regexp.MatchString(ua) {
			return p.allowRequest(req.Context(), "ua", "ua:"+rule.Pattern, rule.Rate, rule.Burst)
		}
	}

	if ip != nil {
		for _, rule := range p.config.IPLimits {
			if rule.Net != nil && rule.Net.Contains(ip) {
				return p.allowRequest(req.Context(), "ip", "ip:"+rule.CIDR, rule.Rate, rule.Burst)
			}
		}
	}

	if p.config.DefaultRate > 0 {
		return p.allowRequest(req.Context(), "default", "default", p.config.DefaultRate, p.config.DefaultBurst)
	}

	return true
}

func (p *RateLimitPlugin) allowRequest(ctx context.Context, rule string, key string, rate float64, burst int) bool {
	now := float64(time.Now().UnixNano()) / 1e9
	keys := []string{"ratelimit:tokens:" + key, "ratelimit:ts:" + key}
	args := []interface{}{rate, burst, now, 1}

	cmd := p.redisClient.RunScript(ctx, p.limitScript, keys, args...)
	if cmd.Err() != nil {
		rateLimitRedisErrorTotal.Inc()
		logger.Error(cmd.Err(), "Rate limit script error", "gateway", "key", key)
		return true
	}

	res, _ := cmd.Int64()
	if res != 1 {
		rateLimitBlockedTotal.WithLabelValues(rule).Inc()
	}
	return res == 1
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
