package main

import (
	"fmt"
	"easyms/pkg/db"
)

func main() {
	// 测试 Redis 连接
	address := "172.29.16.1:6379"
	password := "123456" // Redis 密码
	dbNum := 0
	
	redisClient, err := db.NewEasyRedis(&address, &password, &dbNum)
	if err != nil {
		fmt.Printf("Failed to connect to Redis: %v\n", err)
		return
	}
	
	fmt.Println("Successfully connected to Redis")
	
	// 测试设置和获取值
	err = redisClient.SetCache("test_key", "test_value")
	if err != nil {
		fmt.Printf("Failed to set cache: %v\n", err)
		return
	}
	
	value, err := redisClient.GetCache("test_key")
	if err != nil {
		fmt.Printf("Failed to get cache: %v\n", err)
		return
	}
	
	fmt.Printf("Retrieved value from Redis: %s\n", value)
	
	// 测试删除键
	err = redisClient.RemoveKeyCache("test_key")
	if err != nil {
		fmt.Printf("Failed to remove key: %v\n", err)
		return
	}
	
	fmt.Println("Successfully tested Redis operations")
}