// Package mq 提供了消息队列的抽象和实现。
package mq

import (
	"context"
	"easyms/internal/shared/logger"
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// amqpHeadersCarrier 实现了 OpenTelemetry 的 TextMapCarrier 接口。
// 它允许 OpenTelemetry 的传播器 (propagator) 从 AMQP 消息的 Headers (类型为 amqp.Table) 中读取或写入追踪上下文。
type amqpHeadersCarrier map[string]interface{}

// Get 实现了 TextMapCarrier 接口的 Get 方法。
func (c amqpHeadersCarrier) Get(key string) string {
	if val, ok := c[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Set 实现了 TextMapCarrier 接口的 Set 方法。
func (c amqpHeadersCarrier) Set(key, value string) {
	c[key] = value
}

// Keys 实现了 TextMapCarrier 接口的 Keys 方法。
func (c amqpHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// 确保 amqpHeadersCarrier 实现了 propagation.TextMapCarrier 接口。
var _ propagation.TextMapCarrier = (*amqpHeadersCarrier)(nil)

const (
	reconnectDelay = 5 * time.Second // 连接断开后的重连延迟
	resendDelay    = 5 * time.Second // 消息发送失败后的重试延迟
	maxRetries     = 3               // 最大重试次数
)

// RabbitMQClient 封装了 RabbitMQ 的连接 (Connection) 和通道 (Channel)。
// 它还包含了自动重连的逻辑。
type RabbitMQClient struct {
	url        string
	connection *amqp.Connection
	channel    *amqp.Channel
	mu         sync.RWMutex // 用于保护连接和通道的并发访问
	isClosed   bool         // 标记客户端是否已被主动关闭
}

// NewRabbitMQClient 创建一个新的 RabbitMQ 客户端并建立连接。
// 它还会启动一个后台 goroutine 来处理连接断开后的自动重连。
func NewRabbitMQClient(url string) (*RabbitMQClient, error) {
	client := &RabbitMQClient{
		url: url,
	}
	if err := client.connect(); err != nil {
		return nil, err
	}
	go client.handleReconnect()
	return client, nil
}

// connect 是一个内部方法，用于建立与 RabbitMQ 的连接并打开一个通道。
func (c *RabbitMQClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	c.connection, err = amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("拨号 RabbitMQ 失败: %w", err)
	}

	c.channel, err = c.connection.Channel()
	if err != nil {
		// 如果打开通道失败，关闭已建立的连接
		c.connection.Close()
		return fmt.Errorf("打开通道失败: %w", err)
	}

	logger.Info("成功连接到 RabbitMQ", "rabbitmq")
	return nil
}

// handleReconnect 在后台监控连接状态，并在连接断开时尝试重连。
func (c *RabbitMQClient) handleReconnect() {
	for {
		c.mu.RLock()
		isClosed := c.isClosed
		c.mu.RUnlock()
		if isClosed {
			return // 如果是主动关闭，则退出 goroutine
		}

		// 监听连接关闭的通知
		notifyClose := make(chan *amqp.Error)
		c.connection.NotifyClose(notifyClose)

		err := <-notifyClose // 阻塞直到连接关闭
		if err != nil {
			logger.Error(err, "RabbitMQ 连接已关闭，正在尝试重连...", "rabbitmq")
			// 进入无限重连循环，直到成功
			for {
				if err := c.connect(); err != nil {
					logger.Error(err, "重连 RabbitMQ 失败，正在重试...", "rabbitmq", "delay", reconnectDelay)
					time.Sleep(reconnectDelay)
				} else {
					break // 重连成功，跳出内层循环
				}
			}
		}
	}
}

// Close 主动关闭 RabbitMQ 客户端的连接和通道。
func (c *RabbitMQClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed {
		return
	}
	c.isClosed = true
	if c.channel != nil {
		c.channel.Close()
	}
	if c.connection != nil {
		c.connection.Close()
	}
	logger.Info("RabbitMQ 客户端已关闭", "rabbitmq")
}

// RabbitMQPublisher 是 Publisher 接口基于 RabbitMQ 的实现。
type RabbitMQPublisher struct {
	client *RabbitMQClient
}

// NewRabbitMQPublisher 创建一个新的 RabbitMQ 发布者。
func NewRabbitMQPublisher(url string) (Publisher, error) {
	client, err := NewRabbitMQClient(url)
	if err != nil {
		return nil, err
	}
	return &RabbitMQPublisher{client: client}, nil
}

// Publish 发布一个事件到 RabbitMQ。
// 它会自动声明交换机，并将 OpenTelemetry 的追踪上下文注入到消息头中。
func (p *RabbitMQPublisher) Publish(ctx context.Context, event Event) error {
	p.client.mu.RLock()
	channel := p.client.channel
	p.client.mu.RUnlock()

	// 声明一个 "topic" 类型的交换机，如果它不存在的话。
	// 声明是幂等的，所以重复调用是安全的。
	if err := channel.ExchangeDeclare(
		event.Exchange, // 交换机名称
		"topic",        // 类型
		true,           // durable: 持久化，服务器重启后交换机依然存在
		false,          // auto-deleted: 当没有队列绑定时，不会自动删除
		false,          // internal: 非内部交换机
		false,          // no-wait: 等待服务器确认
		nil,            // arguments
	); err != nil {
		return fmt.Errorf("声明交换机失败: %w", err)
	}

	if event.Headers == nil {
		event.Headers = make(map[string]interface{})
	}

	// 将 OpenTelemetry 的追踪上下文注入到消息头中
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, amqpHeadersCarrier(event.Headers))

	msg := amqp.Publishing{
		ContentType: "application/json",
		Body:        event.Payload,
		Headers:     amqp.Table(event.Headers),
	}

	// 带有重试逻辑的发布
	for i := 0; i < maxRetries; i++ {
		err := channel.Publish(
			event.Exchange,
			event.RoutingKey,
			false, // mandatory: 如果为 true，当消息无法路由到任何队列时，会返回给生产者
			false, // immediate: 如果为 true，当没有消费者连接到队列时，会返回给生产者 (已废弃)
			msg,
		)
		if err == nil {
			return nil // 发布成功
		}
		logger.Error(err, "发布消息失败，正在重试...", "rabbitmq", "attempt", i+1)
		time.Sleep(resendDelay)
	}
	return fmt.Errorf("在 %d 次重试后，发布消息失败", maxRetries)
}

// Close 关闭发布者底层的 RabbitMQ 客户端。
func (p *RabbitMQPublisher) Close() {
	p.client.Close()
}

// RabbitMQConsumer 是 Consumer 接口基于 RabbitMQ 的实现。
type RabbitMQConsumer struct {
	client *RabbitMQClient
}

// NewRabbitMQConsumer 创建一个新的 RabbitMQ 消费者。
func NewRabbitMQConsumer(url string) (Consumer, error) {
	client, err := NewRabbitMQClient(url)
	if err != nil {
		return nil, err
	}
	return &RabbitMQConsumer{client: client}, nil
}

// Consume 开始从指定的队列消费消息。
// 它会自动声明交换机、队列，并将它们绑定。
// 消息处理在一个新的 goroutine 中进行。
func (c *RabbitMQConsumer) Consume(queueName, routingKey, exchangeName string, handler ConsumerHandler) error {
	c.client.mu.RLock()
	channel := c.client.channel
	c.client.mu.RUnlock()

	// 声明交换机
	if err := channel.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("声明交换机失败: %w", err)
	}

	// 声明队列
	q, err := channel.QueueDeclare(
		queueName, // 队列名称
		true,      // durable: 持久化
		false,     // delete when unused: 当没有消费者时，不会自动删除
		false,     // exclusive: 非独占队列
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf("声明队列失败: %w", err)
	}

	// 将队列绑定到交换机
	if err := channel.QueueBind(q.Name, routingKey, exchangeName, false, nil); err != nil {
		return fmt.Errorf("绑定队列失败: %w", err)
	}

	// 开始消费消息
	msgs, err := channel.Consume(
		q.Name,
		"",      // consumer tag: 消费者标签，为空则由服务器生成
		false,   // auto-ack: 关闭自动确认，改为手动确认
		false,   // exclusive
		false,   // no-local
		false,   // no-wait
		nil,     // args
	)
	if err != nil {
		return fmt.Errorf("注册消费者失败: %w", err)
	}

	// 在一个新的 goroutine 中处理接收到的消息
	go func() {
		for d := range msgs {
			// 从消息头中提取 OpenTelemetry 的追踪上下文
			propagator := otel.GetTextMapPropagator()
			ctx := propagator.Extract(context.Background(), amqpHeadersCarrier(d.Headers))

			// 调用业务处理器
			if err := handler(ctx, d.Body); err == nil {
				// 处理成功，手动确认消息
				d.Ack(false) // false 表示只确认当前这一条消息
			} else {
				logger.Error(err, "处理消息失败", "rabbitmq")
				// 处理失败，根据业务决定是重入队列还是丢弃
				d.Nack(false, true) // false 表示只拒绝当前这一条，true 表示让消息重新入队
			}
		}
	}()

	return nil
}

// Close 关闭消费者底层的 RabbitMQ 客户端。
func (c *RabbitMQConsumer) Close() {
	c.client.Close()
}
