package mq

import "context"

// Event 代表一个通用的消息事件
type Event struct {
	Exchange   string                 // 交换机
	RoutingKey string                 // 路由键
	Payload    []byte                 // 消息内容
	Headers    map[string]interface{} // 用于传递元数据，如 Trace Context
}

// Publisher 定义了消息发布者的接口
type Publisher interface {
	// Publish 发布一个事件
	Publish(ctx context.Context, event Event) error
	// Close 关闭发布者连接
	Close()
}

// ConsumerHandler 是处理消息的函数类型
type ConsumerHandler func(ctx context.Context, payload []byte) error

// Consumer 定义了消息消费者的接口
type Consumer interface {
	// Consume 开始消费指定队列的消息
	// queueName: 队列名称
	// routingKey: 绑定的路由键
	// handler: 消息处理函数
	Consume(queueName, routingKey, exchangeName string, handler ConsumerHandler) error
	// Close 关闭消费者连接
	Close()
}
