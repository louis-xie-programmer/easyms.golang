package outbox_relay

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/mq"
	"time"

	"github.com/google/uuid"
)

const componentName = "outbox_relay"

// RelayService is responsible for polling the outbox table and forwarding events to a message queue.
type RelayService struct {
	db        db.Database
	publisher mq.Publisher
	stopChan  chan struct{}
	ticker    *time.Ticker
}

// NewRelayService creates a new instance of the RelayService.
func NewRelayService(db db.Database, pub mq.Publisher, interval time.Duration) *RelayService {
	return &RelayService{
		db:        db,
		publisher: pub,
		stopChan:  make(chan struct{}),
		ticker:    time.NewTicker(interval),
	}
}

// Start begins the background polling task of the RelayService.
func (s *RelayService) Start() {
	logger.Info("Starting Outbox Relay Service...", "component", componentName)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.processOutbox(context.Background())
			case <-s.stopChan:
				s.ticker.Stop()
				return
			}
		}
	}()
}

// Stop halts the RelayService.
func (s *RelayService) Stop() {
	logger.Info("Stopping Outbox Relay Service...", "component", componentName)
	close(s.stopChan)
}

// processOutbox fetches a batch of events from the database, publishes them, and then deletes them.
func (s *RelayService) processOutbox(ctx context.Context) {
	var events []models.OutboxEvent
	// Complete the "fetch" and "delete" in a single transaction to prevent duplicate processing by multiple instances.
	err := s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		// Use FOR UPDATE to lock rows, preventing concurrency issues.
		if err := tx.GetDB().WithContext(ctx).Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Limit(100).Order("created_at asc").Find(&events).Error; err != nil {
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
			// Publish to RabbitMQ.
			// If publishing fails, the entire transaction is rolled back,
			// meaning the event won't be deleted and will be retried on the next poll.
			if err := s.publisher.Publish(ctx, mqEvent); err != nil {
				logger.Error(err, "Failed to publish outbox event, rolling back", "component", componentName, "event_id", event.ID)
				return err
			}
		}

		// After all events are successfully published, delete them.
		eventIDs := make([]uuid.UUID, len(events))
		for i, event := range events {
			eventIDs[i] = event.ID
		}
		err := tx.Delete(ctx, &models.OutboxEvent{}, "id IN ?", eventIDs)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Error processing outbox", "component", componentName)
	}

	if len(events) > 0 {
		logger.Info("Processed and published events from outbox", "component", componentName, "count", len(events))
	}
}
