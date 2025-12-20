package db

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ExampleRedisPipeline 演示如何使用 Pipeline
func ExampleRedisPipeline() {
	// 注意：这只是示例代码，实际使用时需要连接真实的 Redis 服务器
	address := "localhost:6379"
	password := ""
	dbNum := 0

	redisClient := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       dbNum,
	})

	// 创建 EasyRedis 实例
	easyRedis := &EasyRedis{redis: redisClient}

	// 使用 Pipeline 执行多个命令
	pipe := easyRedis.Pipeline()

	// 添加多个命令到 pipeline
	setCmd := pipe.Set(context.Background(), "key1", "value1", 0)
	incrCmd := pipe.Incr(context.Background(), "counter")
	getCmd := pipe.Get(context.Background(), "key1")

	// 执行 pipeline 中的所有命令
	_, err := pipe.Exec(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	// 获取命令结果
	fmt.Println("SET command result:", setCmd.Err())
	fmt.Println("INCR command result:", incrCmd.Val())
	fmt.Println("GET command result:", getCmd.Val())

	// Output:
	// SET command result: <nil>
	// INCR command result: 1
	// GET command result: value1
}

// ExampleRedisPubSub 演示如何使用 Pub/Sub 功能
func ExampleRedisPubSub() {
	// 注意：这只是示例代码，实际使用时需要连接真实的 Redis 服务器
	address := "localhost:6379"
	password := ""
	dbNum := 0

	redisClient := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       dbNum,
	})

	// 创建 EasyRedis 实例
	easyRedis := &EasyRedis{redis: redisClient}

	// 订阅频道
	ctx := context.Background()
	pubsub := easyRedis.Subscribe(ctx, "example_channel")
	defer pubsub.Close()

	// 处理订阅消息的 goroutine
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		ch := pubsub.Channel()
		for msg := range ch {
			fmt.Printf("Received message from %s: %s\n", msg.Channel, msg.Payload)
		}
	}()

	// 发布消息
	err := easyRedis.Publish(ctx, "example_channel", "Hello, World!")
	if err != nil {
		log.Fatal(err)
	}

	// 等待一段时间以接收消息
	time.Sleep(100 * time.Millisecond)

	// Output:
	// Received message from example_channel: Hello, World!
}

// ExampleRedisSortedSet 演示如何使用 Sorted Set 功能
func ExampleRedisSortedSet() {
	// 注意：这只是示例代码，实际使用时需要连接真实的 Redis 服务器
	address := "localhost:6379"
	password := ""
	dbNum := 0

	redisClient := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       dbNum,
	})

	// 创建 EasyRedis 实例
	easyRedis := &EasyRedis{redis: redisClient}

	// 添加成员到有序集合
	members := []redis.Z{
		{Score: 1, Member: "member1"},
		{Score: 2, Member: "member2"},
		{Score: 3, Member: "member3"},
	}

	err := easyRedis.ZAdd("sorted_set_key", members...)
	if err != nil {
		log.Fatal(err)
	}

	// 获取有序集合的成员（按分数从低到高）
	rangeResult, err := easyRedis.ZRange("sorted_set_key", 0, -1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ZRange result:", rangeResult)

	// 获取有序集合的成员（按分数从高到低）
	revRangeResult, err := easyRedis.ZRevRange("sorted_set_key", 0, -1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ZRevRange result:", revRangeResult)

	// 获取成员的分数
	score, err := easyRedis.ZScore("sorted_set_key", "member2")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ZScore result:", score)

	// Output:
	// ZRange result: [member1 member2 member3]
	// ZRevRange result: [member3 member2 member1]
	// ZScore result: 2
}

// ExampleRedisHyperLogLog 演示如何使用 HyperLogLog 功能
func ExampleRedisHyperLogLog() {
	// 注意：这只是示例代码，实际使用时需要连接真实的 Redis 服务器
	address := "localhost:6379"
	password := ""
	dbNum := 0

	redisClient := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       dbNum,
	})

	// 创建 EasyRedis 实例
	easyRedis := &EasyRedis{redis: redisClient}

	// 添加元素到 HyperLogLog
	err := easyRedis.PFAdd("hll_key", "element1", "element2", "element3")
	if err != nil {
		log.Fatal(err)
	}

	// 获取基数估算值
	count, err := easyRedis.PFCount("hll_key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("PFCount result:", count)

	// Output:
	// PFCount result: 3
}

// ExampleRedlock 演示如何使用 Redlock 分布式锁
func ExampleRedlock() {
	// 注意：这只是示例代码，实际使用时需要连接真实的 Redis 服务器集群
	address := "localhost:6379"
	password := ""
	dbNum := 0

	redisClient := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: password,
		DB:       dbNum,
	})

	// 创建 Redlock 实例
	redlock := NewRedlock([]*redis.Client{redisClient})

	// 获取锁
	lock, err := redlock.Acquire("my_lock_key", 10*time.Second)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Lock acquired successfully")

	// 执行一些需要锁保护的操作
	// ...

	// 释放锁
	err = redlock.Release(lock)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Lock released successfully")

	// Output:
	// Lock acquired successfully
	// Lock released successfully
}
