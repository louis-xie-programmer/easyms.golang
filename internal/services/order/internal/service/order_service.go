package service

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OrderService 定义了订单服务的接口
type OrderService interface {
	CreateOrder(ctx context.Context, order *model.Order) error
}

// orderService 实现了 OrderService 接口
type orderService struct {
	db db.Database
}

// NewOrderService 创建一个新的订单服务实例
func NewOrderService(db db.Database) OrderService {
	return &orderService{
		db: db,
	}
}

// CreateOrder 创建一个新订单，并将一个 "order.created" 事件存入发件箱表
func (s *orderService) CreateOrder(ctx context.Context, order *model.Order) error {
	return s.db.RunInTransaction(func(tx db.TxTransaction) error {
		// 1. 在事务中创建订单和订单项
		if err := tx.Insert(order); err != nil {
			return fmt.Errorf("failed to create order in db: %w", err)
		}

		// 2. 准备事件内容
		eventPayload, err := json.Marshal(order)
		if err != nil {
			return fmt.Errorf("failed to marshal order for event: %w", err)
		}

		// 3. 创建 OutboxEvent 记录
		outboxEvent := &model.OutboxEvent{
			ID:         uuid.New(),
			Exchange:   "orders.topic",
			RoutingKey: "order.created",
			Payload:    eventPayload,
			CreatedAt:  time.Now(),
		}

		// 4. 将 OutboxEvent 记录一同存入数据库
		if err := tx.Insert(outboxEvent); err != nil {
			return fmt.Errorf("failed to create outbox event: %w", err)
		}

		return nil // 事务将在此处自动提交
	})
}
