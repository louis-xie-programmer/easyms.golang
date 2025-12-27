package outbox_relay

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/models"
	"easyms/internal/shared/mq"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"
)

const componentName = "outbox_relay"

var (
	eventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_events_processed_total",
		Help: "The total number of events processed from the outbox.",
	})
	publishSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_publish_success_total",
		Help: "The total number of events successfully published.",
	})
	publishFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_publish_failed_total",
		Help: "The total number of events that failed to publish.",
	})
	batchProcessDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "outbox_relay_batch_duration_seconds",
		Help:    "The duration of processing a batch of outbox events.",
		Buckets: prometheus.DefBuckets,
	})
)

// RelayService is responsible for polling the outbox table and forwarding events.
type RelayService struct {
	db        db.Database
	publisher mq.Publisher
	config    *models.OutboxConfig
	stopChan  chan struct{}
	ticker    *time.Ticker
}

// NewRelayService creates a new instance of the RelayService.
func NewRelayService(db db.Database, pub mq.Publisher, cfg *models.OutboxConfig) *RelayService {
	if cfg == nil { // Provide default config
		cfg = &models.OutboxConfig{
			RelayInterval: 10 * time.Second,
			BatchSize:     100,
		}
	}
	return &RelayService{
		db:        db,
		publisher: pub,
		config:    cfg,
		stopChan:  make(chan struct{}),
		ticker:    time.NewTicker(cfg.RelayInterval),
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

func (s *RelayService) processOutbox(ctx context.Context) {
	timer := prometheus.NewTimer(batchProcessDuration)
	defer timer.ObserveDuration()

	var events []models.OutboxEvent
	err := s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		if err := tx.GetDB().WithContext(ctx).Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Limit(s.config.BatchSize).Order("created_at asc").Find(&events).Error; err != nil {
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
			if err := s.publisher.Publish(ctx, mqEvent); err != nil {
				logger.Error(err, "Failed to publish outbox event, rolling back", "component", componentName, "event_id", event.ID, "exchange", event.Exchange)
				publishFailed.Inc()
				return err
			}
			publishSuccess.Inc()
		}

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
		eventsProcessed.Add(float64(len(events)))
		logger.Info("Processed and published events from outbox", "component", componentName, "count", len(events))
	}
}
