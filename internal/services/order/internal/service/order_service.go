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

// --- Data Transfer Objects (DTOs) ---

// CreateOrderRequestDTO is used to pass order creation data to the service layer.
type CreateOrderRequestDTO struct {
	UserID     uint
	OrderItems []CreateOrderItemDTO
}

// CreateOrderItemDTO holds data for a single item in the order.
type CreateOrderItemDTO struct {
	ProductID uint
	Quantity  int
	Price     float64
}

// --- Service Definition ---

// OrderService defines the interface for the order service.
type OrderService interface {
	CreateOrder(ctx context.Context, req *CreateOrderRequestDTO) (*models.Order, error)
}

// orderService implements the OrderService interface.
type orderService struct {
	db  db.Database
	log *logger.Logger
}

// NewOrderService creates a new order service instance.
func NewOrderService(db db.Database, log *logger.Logger) OrderService {
	return &orderService{
		db:  db,
		log: log,
	}
}

// CreateOrder creates a new order, calculates totals, and stores an "order.created" event.
func (s *orderService) CreateOrder(ctx context.Context, req *CreateOrderRequestDTO) (*models.Order, error) {
	// --- Business logic is now inside the service layer ---

	// 1. Build the domain model from the DTO.
	order := &models.Order{
		UserID:     req.UserID,
		Status:     models.StatusPending,
		OrderItems: make([]models.OrderItem, len(req.OrderItems)),
	}

	// 2. Perform calculations.
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

	// 3. Run the creation process in a transaction.
	err := s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		// 3a. Create the order and order items.
		if err := tx.Insert(ctx, order); err != nil {
			s.log.ErrorWithContext(ctx, err, "failed to create order in db", serviceName)
			return fmt.Errorf("failed to create order in db: %w", err)
		}

		// 3b. Create and store the outbox event.
		err := events.CreateAndStoreEvent(ctx, tx, constants.OrderTopic, constants.OrderCreatedEvent, order)
		if err != nil {
			s.log.ErrorWithContext(ctx, err, "failed to create and store outbox event", serviceName)
			return fmt.Errorf("failed to create and store outbox event: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.log.InfoWithContext(ctx, "successfully created order and outbox event", serviceName, "order_id", order.ID)
	return order, nil
}
