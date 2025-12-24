package main

import (
	"context"
	"easyms/internal/shared/config"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/mq"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	serverName := "notification-svc"

	// --- 配置加载 ---
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize app config store: %v", err))
	}
	provider := config.NewLocalConfig(serverName, cfgStore.Env)
	if err := provider.LoadAppConfig(); err != nil {
		panic(fmt.Sprintf("Failed to load local app config: %v", err))
	}
	appConfig := config.GetAppConfig()
	if appConfig == nil {
		panic("Application config is not loaded")
	}

	// --- 日志初始化 ---
	logger.Init(serverName, appConfig)

	// --- 消息队列 Consumer 初始化 ---
	consumer, err := mq.NewRabbitMQConsumer(appConfig.RabbitMQ.URL)
	if err != nil {
		logger.Error(err, "Failed to create RabbitMQ consumer", serverName)
		panic(err)
	}
	defer consumer.Close()

	// --- 定义事件处理器 ---
	orderCreatedHandler := func(ctx context.Context, payload []byte) error {
		var order model.Order
		if err := json.Unmarshal(payload, &order); err != nil {
			logger.Error(err, "Failed to unmarshal order payload", serverName)
			return err // 返回错误，消息将不会被 ack
		}

		logger.Info(
			"✅ OrderCreated event received, simulating sending notification...",
			serverName,
			"order_id", order.ID,
			"user_id", order.UserID,
			"total_amount", order.TotalAmount,
		)
		// 在这里可以添加发送邮件、短信或 WebSocket 推送的真实逻辑
		return nil
	}

	// --- 启动消费者 ---
	// 开始监听 "orders.topic" 交换机上路由键为 "order.created" 的消息
	err = consumer.Consume(
		"notifications_queue", // 队列名
		"order.created",       // 路由键
		"orders.topic",        // 交换机名
		orderCreatedHandler,
	)
	if err != nil {
		logger.Error(err, "Failed to start consuming messages", serverName)
		panic(err)
	}

	logger.Info("Notification service started. Waiting for events...", serverName)

	// --- 优雅地等待退出信号 ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down notification service...", serverName)
}
