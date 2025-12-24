package service

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/mq"
	"github.com/google/uuid"
	"time"
)

// RelayService 负责轮询发件箱表并将事件转发到消息队列
type RelayService struct {
	db        db.Database
	publisher mq.Publisher
	stopChan  chan struct{}
	ticker    *time.Ticker
}

// NewRelayService 创建一个新的 RelayService 实例
func NewRelayService(db db.Database, pub mq.Publisher, interval time.Duration) *RelayService {
	return &RelayService{
		db:        db,
		publisher: pub,
		stopChan:  make(chan struct{}),
		ticker:    time.NewTicker(interval),
	}
}

// Start 启动 RelayService 的后台轮询任务
func (s *RelayService) Start() {
	logger.Info("Starting Outbox Relay Service...", "order-svc")
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.processOutbox()
			case <-s.stopChan:
				s.ticker.Stop()
				return
			}
		}
	}()
}

// Stop 停止 RelayService
func (s *RelayService) Stop() {
	logger.Info("Stopping Outbox Relay Service...", "order-svc")
	close(s.stopChan)
}

// processOutbox 从数据库中获取一批事件，发布它们，然后删除它们
func (s *RelayService) processOutbox() {
	var events []model.OutboxEvent
	// 在一个事务中完成“捞取”和“删除”，防止被多个实例重复处理
	err := s.db.RunInTransaction(func(tx db.TxTransaction) error {
		// 使用 FOR UPDATE 来锁定行，防止并发问题
		if err := tx.GetDB().Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Limit(100).Order("created_at asc").Find(&events).Error; err != nil {
			return err
		}

		if len(events) == 0 {
			return nil
		}

		for _, event := range events {
			mqEvent := mq.Event{
				Exchange:   event.Exchange,
				RoutingKey: event.RoutingKey,
				Payload:    event.Payload,
			}
			// 发送到 RabbitMQ
			if err := s.publisher.Publish(context.Background(), mqEvent); err != nil {
				// 如果发送失败，由于我们在一个事务中，整个事务会回滚，
				// 这意味着事件不会被删除，将在下一次轮询中重试。
				logger.Error(err, "Failed to publish outbox event, rolling back...", "order-svc", "event_id", event.ID)
				return err
			}
		}

		// 所有事件都成功发布后，删除这些事件
		eventIDs := make([]uuid.UUID, len(events))
		for i, event := range events {
			eventIDs[i] = event.ID
		}
		err := tx.Delete(&model.OutboxEvent{}, "id IN ?", eventIDs)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Error processing outbox", "order-svc")
	}

	if len(events) > 0 {
		logger.Info("Processed and published events from outbox", "order-svc", "count", len(events))
	}
}
