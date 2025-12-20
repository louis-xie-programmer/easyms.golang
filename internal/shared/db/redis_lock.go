package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redlock 分布式锁结构体
type Redlock struct {
	clients []*redis.Client
	quorum  int
}

// Lock 分布式锁实例
type Lock struct {
	Key     string
	Value   string
	Expires time.Duration
}

// NewRedlock 创建一个新的 Redlock 实例
// clients: Redis 客户端列表（通常连接到不同的 Redis 实例）
func NewRedlock(clients []*redis.Client) *Redlock {
	quorum := len(clients)/2 + 1
	return &Redlock{
		clients: clients,
		quorum:  quorum,
	}
}

// generateRandomValue 生成随机值用于标识锁的持有者
func (r *Redlock) generateRandomValue() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// Acquire 获取分布式锁
// key: 锁的键名
// expires: 锁的过期时间
func (r *Redlock) Acquire(key string, expires time.Duration) (*Lock, error) {
	value, err := r.generateRandomValue()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	startTime := time.Now()

	for {
		acquired := r.acquireInstance(ctx, key, value, expires)
		elapsed := time.Since(startTime)

		// 检查是否获得大多数节点的锁
		if acquired >= r.quorum {
			// 锁获取成功
			return &Lock{
				Key:     key,
				Value:   value,
				Expires: expires,
			}, nil
		}

		// 如果超时则退出
		if elapsed > expires {
			// 尝试释放部分获取到的锁
			r.releaseInstance(ctx, key, value)
			return nil, redis.Nil
		}

		// 等待一小段时间后重试
		time.Sleep(5 * time.Millisecond)
	}
}

// acquireInstance 尝试在所有实例上获取锁
func (r *Redlock) acquireInstance(ctx context.Context, key, value string, expires time.Duration) int {
	acquired := 0
	for _, client := range r.clients {
		ok, err := client.SetNX(ctx, key, value, expires).Result()
		if err == nil && ok {
			acquired++
		}
	}
	return acquired
}

// releaseInstance 在所有实例上释放锁
func (r *Redlock) releaseInstance(ctx context.Context, key, value string) {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	for _, client := range r.clients {
		client.Eval(ctx, script, []string{key}, value)
	}
}

// Release 释放分布式锁
func (r *Redlock) Release(lock *Lock) error {
	ctx := context.Background()
	r.releaseInstance(ctx, lock.Key, lock.Value)
	return nil
}

// Extend 延长锁的过期时间
func (r *Redlock) Extend(lock *Lock) error {
	ctx := context.Background()
	extended := 0

	for _, client := range r.clients {
		// 检查当前锁是否仍由我们持有
		val, err := client.Get(ctx, lock.Key).Result()
		if err == nil && val == lock.Value {
			// 延长过期时间
			_, err := client.Expire(ctx, lock.Key, lock.Expires).Result()
			if err == nil {
				extended++
			}
		}
	}

	// 如果大多数节点上的锁都被延长了，则认为成功
	if extended >= r.quorum {
		return nil
	}

	return redis.Nil
}
