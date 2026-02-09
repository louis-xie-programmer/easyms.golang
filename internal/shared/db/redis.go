// redis.go Redis缓存模块
// 主要功能：
// 1. Redis客户端封装
// 2. 提供常用的缓存操作方法
// 3. 实现缓存穿透、击穿、雪崩防护机制
// 4. 提供分布式锁功能
package db

import (
	"context"
	"encoding/json"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// EasyRedis Redis客户端封装
// 对redis-go客户端进行封装，提供更便捷的操作接口
type EasyRedis struct {
	redis *redis.Client      // Redis客户端实例
	sf    singleflight.Group // 用于防止缓存击穿的 singleflight
}

// NewEasyRedis 创建新的Redis客户端
// 初始化Redis客户端并测试连接
// 参数:
//   - address: Redis服务器地址
//   - password: Redis密码
//   - dbNum: 数据库编号
//
// 返回值:
//   - *EasyRedis: Redis客户端实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewEasyRedis(address *string, password *string, dbNum *int) (*EasyRedis, error) {
	// 创建Redis客户端配置
	// 配置Redis服务器地址、密码和数据库编号
	redis := redis.NewClient(&redis.Options{
		Addr:       *address,  // Redis服务器地址
		Password:   *password, // 密码（如果有的话）
		DB:         *dbNum,    // 使用的数据库编号
		MaxRetries: 3,         // 最大重试次数
	})

	// 测试连接
	// 通过Ping命令测试Redis连接是否正常
	if redis.Ping(context.Background()).Err() != nil {
		return nil, redis.Ping(context.Background()).Err()
	}

	return &EasyRedis{redis: redis}, nil
}

// ExistsKey 判断key是否存在
// 使用Redis EXISTS命令检查指定键是否存在
// 参数:
//   - key: 键名
//
// 返回值:
//   - bool: 存在返回true，否则返回false
func (r *EasyRedis) ExistsKey(key string) bool {
	ctx := context.Background()
	res := r.redis.Exists(ctx, key).Val()
	return res == 1
}

// SetCache 设置缓存值
// 将值序列化为JSON格式后存储到Redis中
// 参数:
//   - key: 键名
//   - value: 键值
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) SetCache(key string, value interface{}) error {
	ctx := context.Background()
	// 将值序列化为JSON格式
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 设置缓存，永不过期
	return r.redis.Set(ctx, key, val, 0).Err()
}

// SetHashCache 设置hash值
// 参数:
//   - key: 键名
//   - values: 键值对映射
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) SetHashCache(key string, values map[string]interface{}) error {
	ctx := context.Background()

	// 如果键已存在，先删除
	if r.ExistsKey(key) {
		err := r.redis.Del(ctx, key).Err()
		if err != nil {
			return err
		}
	}

	// 设置hash值
	return r.redis.HSet(ctx, key, values).Err()
}

// RemoveParentKeyCache 删除父键中的所有子键
// 参数:
//   - parentKeyPattern: 父键模式
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) RemoveParentKeyCache(parentKeyPattern string) error {
	ctx := context.Background()

	// 扫描匹配的键
	iter := r.redis.Scan(ctx, 0, parentKeyPattern, 0).Iterator()
	for iter.Next(ctx) {
		// 删除匹配的键
		err := r.redis.Del(ctx, iter.Val()).Err()
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoveKeyCache 删除指定键
// 参数:
//   - key: 键名
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) RemoveKeyCache(key string) error {
	ctx := context.Background()
	return r.redis.Del(ctx, key).Err()
}

// GetCache 获取缓存值
// 从Redis中获取指定键的值
// 参数:
//   - key: 键名
//
// 返回值:
//   - string: 键值
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) GetCache(key string) (string, error) {
	ctx := context.Background()
	return r.redis.Get(ctx, key).Result()
}

// GetHashCache 获取hash缓存值
// 参数:
//   - key: 键名
//   - field: 字段名
//
// 返回值:
//   - map[string]string: hash值映射
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) GetHashCache(key string, field string) (map[string]string, error) {
	ctx := context.Background()
	vals, err := r.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return vals, nil
}

// SetHashSetCache 在集合中插入数据
// 参数:
//   - key: 键名
//   - value: 要插入的值
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) SetHashSetCache(key string, value interface{}) error {
	ctx := context.Background()

	// 将值序列化为JSON格式
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 添加到集合中
	err = r.redis.SAdd(ctx, key, val).Err()
	if err != nil {
		return err
	}

	return nil
}

// GetHashSetCache 获取集合值
// 参数:
//   - key: 键名
//   - field: 字段名
//
// 返回值:
//   - []string: 集合中的所有值
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) GetHashSetCache(key string, field string) ([]string, error) {
	ctx := context.Background()
	vals, err := r.redis.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return vals, nil
}

// ZAdd 向有序集合添加成员
func (r *EasyRedis) ZAdd(key string, members ...redis.Z) error {
	ctx := context.Background()
	return r.redis.ZAdd(ctx, key, members...).Err()
}

// ZRange 获取有序集合指定范围的成员
func (r *EasyRedis) ZRange(key string, start, stop int64) ([]string, error) {
	ctx := context.Background()
	return r.redis.ZRange(ctx, key, start, stop).Result()
}

// ZRevRange 获取有序集合指定范围的成员（按分数从高到低）
func (r *EasyRedis) ZRevRange(key string, start, stop int64) ([]string, error) {
	ctx := context.Background()
	return r.redis.ZRevRange(ctx, key, start, stop).Result()
}

// ZScore 获取有序集合成员的分数
func (r *EasyRedis) ZScore(key, member string) (float64, error) {
	ctx := context.Background()
	return r.redis.ZScore(ctx, key, member).Result()
}

// ZRank 获取有序集合成员的排名
func (r *EasyRedis) ZRank(key, member string) (int64, error) {
	ctx := context.Background()
	return r.redis.ZRank(ctx, key, member).Result()
}

// PFAdd 添加元素到 HyperLogLog
func (r *EasyRedis) PFAdd(key string, els ...interface{}) error {
	ctx := context.Background()
	return r.redis.PFAdd(ctx, key, els...).Err()
}

// PFCount 获取 HyperLogLog 的基数估算值
func (r *EasyRedis) PFCount(keys ...string) (int64, error) {
	ctx := context.Background()
	return r.redis.PFCount(ctx, keys...).Result()
}

// PFMerge 将多个 HyperLogLog 合并为一个
func (r *EasyRedis) PFMerge(dest string, keys ...string) error {
	ctx := context.Background()
	return r.redis.PFMerge(ctx, dest, keys...).Err()
}

// GetCacheWithProtection 带防护机制的缓存获取方法
// 实现缓存穿透、击穿、雪崩防护
// 通过空值缓存防止穿透，通过 singleflight 防止击穿，通过随机过期时间防止雪崩
// 参数:
//   - key: 键名
//   - nullCacheExpire: 空值缓存过期时间（秒）
//   - mutexExpire: 互斥锁过期时间（秒）(保留参数以兼容接口，实际使用 singleflight)
//   - fallback: 回退函数，用于从数据源获取数据
//
// 返回值:
//   - interface{}: 缓存值或数据源返回的值
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) GetCacheWithProtection(key string, nullCacheExpire, mutexExpire int, fallback func() (interface{}, error)) (interface{}, error) {
	// 使用 singleflight 合并并发请求
	val, err, _ := r.sf.Do(key, func() (interface{}, error) {
		return r.getCacheWithProtection(key, nullCacheExpire, mutexExpire, fallback, 0)
	})
	return val, err
}

// getCacheWithProtection 带防护机制的缓存获取方法（内部递归版本）
// retries: 重试次数
func (r *EasyRedis) getCacheWithProtection(key string, nullCacheExpire, mutexExpire int, fallback func() (interface{}, error), retries int) (interface{}, error) {
	// 限制重试次数，防止无限递归
	const maxRetries = 3
	if retries >= maxRetries {
		return fallback()
	}

	ctx := context.Background()

	// 1. 尝试从缓存获取
	// 使用Redis GET命令获取缓存值
	val, err := r.redis.Get(ctx, key).Result()
	if err == nil {
		// 缓存命中
		// 检查是否为空值占位符
		// 防止缓存穿透，对于不存在的数据也进行缓存
		if val == "NULL_CACHE_PLACEHOLDER" {
			return nil, nil // 返回空值，表示数据源中也不存在
		}

		// 反序列化缓存值
		// 将JSON格式的缓存值反序列化为Go对象
		var result interface{}
		if err := json.Unmarshal([]byte(val), &result); err != nil {
			return nil, err
		}

		return result, nil
	}

	// 2. 缓存未命中，查询数据源
	// 注意：由于外层已经使用了 singleflight，这里不需要再加分布式锁
	// singleflight 已经确保了同一时间只有一个 goroutine 会执行到这里

	// 调用回退函数从数据源获取数据
	result, err := fallback()
	if err != nil {
		// 数据源查询失败，直接返回错误
		return nil, err
	}

	// 3. 将结果写入缓存
	// 缓存数据以减轻数据库压力
	var expireTime time.Duration
	if result == nil {
		// 空值缓存，设置较短的过期时间
		// 防止缓存穿透，对不存在的数据也进行缓存
		expireTime = time.Duration(nullCacheExpire) * time.Second
		r.redis.Set(ctx, key, "NULL_CACHE_PLACEHOLDER", expireTime)
	} else {
		// 正常值缓存，设置随机过期时间防止雪崩 (300-600秒)
		// 随机过期时间避免大量缓存同时失效导致的雪崩
		expireTime = time.Duration(300+rand.Intn(300)) * time.Second
		data, _ := json.Marshal(result)
		r.redis.Set(ctx, key, data, expireTime)
	}

	return result, nil
}

// acquireLock 获取分布式锁
// 使用Redis SET 命令的 NX 和 EX 选项实现原子加锁
// 参数:
//   - key: 锁键名
//   - expireSeconds: 锁过期时间（秒）
//
// 返回值:
//   - bool: 获取锁成功返回true，否则返回false
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) acquireLock(key string, expireSeconds int) (bool, error) {
	ctx := context.Background()
	// 使用 SET 命令的 NX 和 EX 选项实现原子加锁
	result, err := r.redis.SetNX(ctx, key, "1", time.Duration(expireSeconds)*time.Second).Result()
	if err != nil {
		return false, err
	}
	return result, nil
}

// releaseLock 释放分布式锁
// 参数:
//   - key: 锁键名
//
// 返回值:
//   - error: 操作成功返回nil，失败返回具体错误
func (r *EasyRedis) releaseLock(key string) error {
	ctx := context.Background()
	// 使用 Lua 脚本确保解锁的原子性
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	return r.redis.Eval(ctx, script, []string{key}, "1").Err()
}

// Pipeline 创建一个 Redis Pipeline
func (r *EasyRedis) Pipeline() redis.Pipeliner {
	return r.redis.Pipeline()
}

// TxPipeline 创建一个支持事务的 Redis Pipeline
func (r *EasyRedis) TxPipeline() redis.Pipeliner {
	return r.redis.TxPipeline()
}

// Subscribe 订阅频道
func (r *EasyRedis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return r.redis.Subscribe(ctx, channels...)
}

// Publish 发布消息到频道
func (r *EasyRedis) Publish(ctx context.Context, channel string, message interface{}) error {
	return r.redis.Publish(ctx, channel, message).Err()
}

func (r *EasyRedis) SetEx(key string, value interface{}, expireSeconds int) error {
	ctx := context.Background()
	return r.redis.Set(ctx, key, value, time.Duration(expireSeconds)*time.Second).Err()
}

// Eval executes a Lua script on the server.
func (r *EasyRedis) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return r.redis.Eval(ctx, script, keys, args...)
}

// RunScript executes a redis.Script on the server.
// It automatically handles SCRIPT LOAD and EVALSHA optimization.
func (r *EasyRedis) RunScript(ctx context.Context, script *redis.Script, keys []string, args ...interface{}) *redis.Cmd {
	return script.Run(ctx, r.redis, keys, args...)
}

// Close closes the Redis client.
func (r *EasyRedis) Close() error {
	return r.redis.Close()
}
