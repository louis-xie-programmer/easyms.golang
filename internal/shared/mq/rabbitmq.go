package mq

import (
	"context"
	"easyms/internal/shared/logger"
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
)

// amqpHeadersCarrier implements propagation.TextMapCarrier for AMQP headers.
// It allows the OpenTelemetry propagator to read from and write to amqp.Table.
type amqpHeadersCarrier map[string]interface{}

// Get returns the value associated with the passed key.
func (c amqpHeadersCarrier) Get(key string) string {
	if val, ok := c[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Set stores the key-value pair.
func (c amqpHeadersCarrier) Set(key, value string) {
	c[key] = value
}

// Keys returns a slice of all keys in the carrier.
func (c amqpHeadersCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

const (
	reconnectDelay = 5 * time.Second
	resendDelay    = 5 * time.Second
	maxRetries     = 3
)

// RabbitMQClient 封装了 RabbitMQ 的连接和通道
type RabbitMQClient struct {
	url        string
	connection *amqp.Connection
	channel    *amqp.Channel
	mu         sync.RWMutex
	isClosed   bool
}

// NewRabbitMQClient 创建一个新的 RabbitMQ 客户端并连接
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

func (c *RabbitMQClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	c.connection, err = amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("failed to dial rabbitmq: %w", err)
	}

	c.channel, err = c.connection.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel: %w", err)
	}

	logger.Info("Successfully connected to RabbitMQ", "rabbitmq")
	return nil
}

func (c *RabbitMQClient) handleReconnect() {
	for {
		c.mu.RLock()
		isClosed := c.isClosed
		c.mu.RUnlock()
		if isClosed {
			return
		}

		notifyClose := make(chan *amqp.Error)
		c.connection.NotifyClose(notifyClose)

		err := <-notifyClose
		if err != nil {
			logger.Error(err, "RabbitMQ connection closed, attempting to reconnect...", "rabbitmq")
			for {
				if err := c.connect(); err != nil {
					logger.Error(err, "Failed to reconnect to RabbitMQ, retrying...", "rabbitmq", "delay", reconnectDelay)
					time.Sleep(reconnectDelay)
				} else {
					break // Reconnected
				}
			}
		}
	}
}

func (c *RabbitMQClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isClosed = true
	if c.channel != nil {
		c.channel.Close()
	}
	if c.connection != nil {
		c.connection.Close()
	}
}

// RabbitMQPublisher 实现了 Publisher 接口
type RabbitMQPublisher struct {
	client *RabbitMQClient
}

// NewRabbitMQPublisher 创建一个新的 RabbitMQ 发布者
func NewRabbitMQPublisher(url string) (Publisher, error) {
	client, err := NewRabbitMQClient(url)
	if err != nil {
		return nil, err
	}
	return &RabbitMQPublisher{client: client}, nil
}

func (p *RabbitMQPublisher) Publish(ctx context.Context, event Event) error {
	p.client.mu.RLock()
	channel := p.client.channel
	p.client.mu.RUnlock()

	if err := channel.ExchangeDeclare(
		event.Exchange, // name
		"topic",        // type
		true,           // durable
		false,          // auto-deleted
		false,          // internal
		false,          // no-wait
		nil,            // arguments
	); err != nil {
		return fmt.Errorf("failed to declare an exchange: %w", err)
	}

	// 如果 event.Headers 为 nil，则初始化
	if event.Headers == nil {
		event.Headers = make(map[string]interface{})
	}

	// 使用自定义的 carrier 直接向 event.Headers 注入追踪信息
	propagator := otel.GetTextMapPropagator()
	propagator.Inject(ctx, amqpHeadersCarrier(event.Headers))

	msg := amqp.Publishing{
		ContentType: "application/json",
		Body:        event.Payload,
		Headers:     amqp.Table(event.Headers), // amqp.Table 的底层就是 map[string]interface{}
	}

	for i := 0; i < maxRetries; i++ {
		err := channel.Publish(
			event.Exchange,
			event.RoutingKey,
			false, // mandatory
			false, // immediate
			msg,
		)
		if err == nil {
			return nil
		}
		logger.Error(err, "Failed to publish message, retrying...", "rabbitmq", "attempt", i+1)
		time.Sleep(resendDelay)
	}
	return fmt.Errorf("failed to publish message after %d retries", maxRetries)
}

func (p *RabbitMQPublisher) Close() {
	p.client.Close()
}

// RabbitMQConsumer 实现了 Consumer 接口
type RabbitMQConsumer struct {
	client *RabbitMQClient
}

// NewRabbitMQConsumer 创建一个新的 RabbitMQ 消费者
func NewRabbitMQConsumer(url string) (Consumer, error) {
	client, err := NewRabbitMQClient(url)
	if err != nil {
		return nil, err
	}
	return &RabbitMQConsumer{client: client}, nil
}

func (c *RabbitMQConsumer) Consume(queueName, routingKey, exchangeName string, handler ConsumerHandler) error {
	c.client.mu.RLock()
	channel := c.client.channel
	c.client.mu.RUnlock()

	if err := channel.ExchangeDeclare(
		exchangeName, // name
		"topic",      // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	); err != nil {
		return fmt.Errorf("failed to declare an exchange: %w", err)
	}

	q, err := channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	if err := channel.QueueBind(
		q.Name,
		routingKey,
		exchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to bind a queue: %w", err)
	}

	msgs, err := channel.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	go func() {
		for d := range msgs {
			// 使用自定义的 carrier 直接从 d.Headers 中提取追踪信息
			propagator := otel.GetTextMapPropagator()
			ctx := propagator.Extract(context.Background(), amqpHeadersCarrier(d.Headers))

			if err := handler(ctx, d.Body); err == nil {
				d.Ack(false)
			} else {
				logger.Error(err, "Failed to handle message", "rabbitmq")
				// 消息处理失败，根据业务决定是重入队列还是丢弃
				d.Nack(false, true) // true to requeue
			}
		}
	}()

	return nil
}

func (c *RabbitMQConsumer) Close() {
	c.client.Close()
}
