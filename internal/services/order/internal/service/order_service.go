package service

import (
	"context"
	"easyms/internal/services/order/internal/constants"
	"easyms/internal/shared/db"
	"easyms/internal/shared/events"
	"easyms/internal/shared/models"
	"fmt"
)

// OrderService defines the interface for the order service.
type OrderService interface {
	CreateOrder(ctx context.Context, order *models.Order) error
}

// orderService implements the OrderService interface.
type orderService struct {
	db db.Database
}

// NewOrderService creates a new order service instance.
func NewOrderService(db db.Database) OrderService {
	return &orderService{
		db: db,
	}
}

// CreateOrder creates a new order and stores an "order.created" event in the outbox table.
func (s *orderService) CreateOrder(ctx context.Context, order *models.Order) error {
	return s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		// 1. Create the order and order items within the transaction.
		if err := tx.Insert(ctx, order); err != nil {
			return fmt.Errorf("failed to create order in db: %w", err)
		}

		// 2. Create and store the outbox event using the shared event publisher.
		err := events.CreateAndStoreEvent(ctx, tx, constants.OrderTopic, constants.OrderCreatedEvent, order)
		if err != nil {
			return fmt.Errorf("failed to create and store outbox event: %w", err)
		}

		return nil // The transaction will be committed automatically here.
	})
}
