// Package service 包含了订单服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/services/order/internal/constants"
	"easyms/internal/shared/db"
	"easyms/internal/shared/events"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"fmt"
)

const serviceName = "order_service"

// --- 数据传输对象 (DTOs) ---

// CreateOrderRequestDTO 用于在服务层传递订单创建请求的数据。
type CreateOrderRequestDTO struct {
	UserID     uint                 // 下单用户ID
	OrderItems []CreateOrderItemDTO // 订单项列表
}

// CreateOrderItemDTO 包含了订单中单个商品项的数据。
type CreateOrderItemDTO struct {
	ProductID uint    // 商品ID
	Quantity  int     // 商品数量
	Price     float64 // 商品单价
}

// --- 服务接口定义 ---

// OrderService 定义了订单服务的标准接口。
// 它抽象了所有与订单相关的业务操作。
type OrderService interface {
	// CreateOrder 创建一个新订单，并处理相关的业务逻辑和事件发布。
	CreateOrder(ctx context.Context, req *CreateOrderRequestDTO) (*models.Order, error)
}

// orderService 是 OrderService 接口的具体实现。
// 它依赖于 db.Database 接口来处理数据持久化，并使用 logger 进行日志记录。
type orderService struct {
	db  db.Database
	log *logger.Logger
}

// NewOrderService 创建一个新的 orderService 实例。
// db: 数据库接口实例。
// log: 日志记录器实例。
func NewOrderService(db db.Database, log *logger.Logger) OrderService {
	return &orderService{
		db:  db,
		log: log,
	}
}

// CreateOrder 创建一个新订单。
// 该方法包含了订单创建的业务逻辑，包括构建订单模型、计算总金额，
// 并在一个数据库事务中完成订单的持久化和 Outbox 事件的创建。
func (s *orderService) CreateOrder(ctx context.Context, req *CreateOrderRequestDTO) (*models.Order, error) {
	// 1. 从 DTO 构建领域模型。
	order := &models.Order{
		UserID:     req.UserID,
		Status:     models.StatusPending, // 初始状态为待处理
		OrderItems: make([]models.OrderItem, len(req.OrderItems)),
	}

	// 2. 执行业务计算，例如计算订单总金额。
	var total float64
	for i, item := range req.OrderItems {
		order.OrderItems[i] = models.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
		total += float64(item.Quantity) * item.Price
	}
	order.TotalAmount = total

	// 3. 在一个数据库事务中执行订单创建和 Outbox 事件存储，确保原子性。
	err := s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		// 3a. 创建订单及其订单项。
		if err := tx.Insert(ctx, order); err != nil {
			s.log.ErrorWithContext(ctx, err, "在数据库中创建订单失败", serviceName)
			return fmt.Errorf("在数据库中创建订单失败: %w", err)
		}

		// 3b. 创建并存储 Outbox 事件。
		// 这确保了订单创建成功后，相应的事件也会被持久化，等待 Relay 服务发送。
		err := events.CreateAndStoreEvent(ctx, tx, constants.OrderTopic, constants.OrderCreatedEvent, order)
		if err != nil {
			s.log.ErrorWithContext(ctx, err, "创建并存储 Outbox 事件失败", serviceName)
			return fmt.Errorf("创建并存储 Outbox 事件失败: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.log.InfoWithContext(ctx, "成功创建订单和 Outbox 事件", serviceName, "order_id", order.ID)
	return order, nil
}
