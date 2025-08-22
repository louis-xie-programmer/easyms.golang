package db

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

type EasyRedis struct {
	redis *redis.Client
}

func NewEasyRedis(address *string, password *string, dbNum *int) (*EasyRedis, error) {
	redis := redis.NewClient(&redis.Options{
		Addr:     *address,
		Password: *password, // no password set
		DB:       *dbNum,    // use default DB
	})

	if redis.Ping(context.Background()).Err() != nil {
		return nil, redis.Ping(context.Background()).Err()
	}
	return &EasyRedis{redis: redis}, nil
}

// ExistsKey 判断key是否存在
func (r *EasyRedis) ExistsKey(key string) bool {
	ctx := context.Background()
	res := r.redis.Exists(ctx, key).Val()
	return res == 1
}

// SetCache 设置缓存值
func (r *EasyRedis) SetCache(key string, value interface{}) error {
	ctx := context.Background()
	val, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.redis.Set(ctx, key, val, 0).Err()
}

// SetHashCache 设置hash值
func (r *EasyRedis) SetHashCache(key string, values map[string]interface{}) error {
	ctx := context.Background()
	if r.ExistsKey(key) {
		err := r.redis.Del(ctx, key).Err()
		if err != nil {
			return err
		}
	}
	return r.redis.HSet(ctx, key, values).Err()
}

// RemoveParentKeyCache 删除父键中的所有子键
func (r *EasyRedis) RemoveParentKeyCache(parentKeyPattern string) error {
	ctx := context.Background()
	iter := r.redis.Scan(ctx, 0, parentKeyPattern, 0).Iterator()
	for iter.Next(ctx) {
		err := r.redis.Del(ctx, iter.Val()).Err()
		if err != nil {
			return err
		}
	}
	return nil
}

// RemoveKeyCache 删除
func (r *EasyRedis) RemoveKeyCache(key string) error {
	ctx := context.Background()
	return r.redis.Del(ctx, key).Err()
}

// GetCache 获取缓存值
func (r *EasyRedis) GetCache(key string) (string, error) {
	ctx := context.Background()
	return r.redis.Get(ctx, key).Result()
}

// GetHashCache 获取hash缓存值
func (r *EasyRedis) GetHashCache(key string, field string) (map[string]string, error) {
	ctx := context.Background()
	vals, err := r.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return vals, nil
}

// SetHashSetCache 在集合中插入数据
func (r *EasyRedis) SetHashSetCache(key string, value interface{}) error {
	ctx := context.Background()

	val, err := json.Marshal(value)
	if err != nil {
		return err
	}

	err = r.redis.SAdd(ctx, key, val).Err()
	if err != nil {
		return err
	}
	return nil
}

// GetHashSetCache 获取集合值
func (r *EasyRedis) GetHashSetCache(key string, field string) ([]string, error) {
	ctx := context.Background()
	vals, err := r.redis.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return vals, nil
}
