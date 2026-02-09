// Package mq 提供了消息队列的通用抽象接口。
// 它的目的是定义一套标准的消息发布和消费模式，
// 使得上层业务逻辑可以与具体的消息队列实现 (如 RabbitMQ, Kafka) 解耦。
package mq

import "context"

// Event 代表一个通用的、准备发布到消息队列的事件。
type Event struct {
	Exchange   string                 // 目标交换机 (Exchange) 的名称。
	RoutingKey string                 // 路由键 (Routing Key)，用于决定消息如何被路由到队列。
	Payload    []byte                 // 消息的实际内容，通常是序列化后的数据 (如 JSON)。
	Headers    map[string]interface{} // 消息头，用于传递元数据，例如 OpenTelemetry 的追踪上下文 (Trace Context)。
}

// Publisher 定义了消息发布者的标准接口。
type Publisher interface {
	// Publish 发布一个事件到消息队列。
	// ctx: 上下文，用于传递请求范围的数据，特别是分布式追踪信息。
	// event: 要发布的事件对象。
	Publish(ctx context.Context, event Event) error
	// Close 关闭发布者及其底层的连接。
	Close()
}

// ConsumerHandler 是一个函数类型，定义了如何处理从队列中接收到的单个消息。
// ctx: 从消息头中提取出的上下文，包含了上游服务的追踪信息。
// payload: 消息的原始字节内容。
// 如果处理成功，应返回 nil；如果处理失败，应返回一个 error，消息将被拒绝 (Nack)。
type ConsumerHandler func(ctx context.Context, payload []byte) error

// Consumer 定义了消息消费者的标准接口。
type Consumer interface {
	// Consume 开始从指定的队列消费消息。
	// 这个方法通常会启动一个后台 goroutine 来持续接收和处理消息。
	// queueName: 要消费的队列名称。
	// routingKey: 队列绑定的路由键。
	// exchangeName: 队列所绑定的交换机名称。
	// handler: 用于处理每条接收到的消息的回调函数。
	Consume(queueName, routingKey, exchangeName string, handler ConsumerHandler) error
	// Close 关闭消费者及其底层的连接。
	Close()
}
