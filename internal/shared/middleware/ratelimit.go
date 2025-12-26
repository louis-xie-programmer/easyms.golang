package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiterConfig 配置
type RateLimiterConfig struct {
	Client    *redis.Client // Redis 客户端
	Limit     int64         // 时间窗口内允许的最大请求数
	Window    time.Duration // 时间窗口
	KeyPrefix string        // Redis 键前缀
}

// RateLimiter 中间件
func RateLimiter(config RateLimiterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 生成唯一的键 (例如，基于 IP 地址)
		key := config.KeyPrefix + c.ClientIP()
		ctx := c.Request.Context()

		// 2. 使用 Redis 的 ZSET 实现滑动窗口限流
		now := time.Now().UnixNano()
		windowStart := now - config.Window.Nanoseconds()

		// a. 移除窗口之外的旧请求记录
		config.Client.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))

		// b. 获取当前窗口内的请求数量
		count := config.Client.ZCard(ctx, key).Val()

		// c. 检查是否超过限制
		if count >= config.Limit {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			return
		}

		// d. 记录当前请求
		config.Client.ZAdd(ctx, key, redis.Z{
			Score:  float64(now),
			Member: strconv.FormatInt(now, 10),
		})
		// 设置一个过期时间，防止冷数据永久占用内存
		config.Client.Expire(ctx, key, config.Window)

		c.Next()
	}
}

// GetRateLimiterKeyFunc 定义一个函数类型，用于从 Gin 上下文中生成限流的 key
type GetRateLimiterKeyFunc func(c *gin.Context) string

// KeyByIP 基于 IP 生成 Key
func KeyByIP(prefix string) GetRateLimiterKeyFunc {
	return func(c *gin.Context) string {
		return prefix + c.ClientIP()
	}
}

// KeyByUserID 基于用户 ID 生成 Key (需要配合认证中间件使用)
func KeyByUserID(prefix string, userKeyInContext string) GetRateLimiterKeyFunc {
	return func(c *gin.Context) string {
		if userID, exists := c.Get(userKeyInContext); exists {
			if idStr, ok := userID.(string); ok {
				return prefix + idStr
			}
		}
		// 如果没有用户信息，则回退到 IP 限流
		return prefix + c.ClientIP()
	}
}

// FlexibleRateLimiter 更灵活的限流中间件
func FlexibleRateLimiter(client *redis.Client, limit int64, window time.Duration, getKey GetRateLimiterKeyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := getKey(c)
		ctx := context.Background()

		now := time.Now().UnixNano()
		windowStart := now - window.Nanoseconds()

		// 使用 Pipeline 提高性能
		pipe := client.Pipeline()
		// 1. 移除旧记录
		pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))
		// 2. 获取当前数量
		cardCmd := pipe.ZCard(ctx, key)
		// 3. 添加当前记录
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: strconv.FormatInt(now, 10)})
		// 4. 设置过期时间
		pipe.Expire(ctx, key, window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			// 如果 Redis 出错，为了不影响业务，可以选择放行
			c.Next()
			return
		}

		// 检查是否超限
		if cardCmd.Val() > limit {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			return
		}

		c.Next()
	}
}
