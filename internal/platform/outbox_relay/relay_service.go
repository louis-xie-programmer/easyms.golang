// Package outbox_relay 实现了 Outbox（发件箱）模式的事件中继器。
// 它的核心职责是定期轮询数据库中的 `outbox_events` 表，
// 将待处理的事件发布到消息队列，然后从表中删除已成功发布的事件。
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

// Prometheus 指标，用于监控中继服务的运行状况。
var (
	// eventsProcessed 记录已处理的事件总数。
	eventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_events_processed_total",
		Help: "已处理的发件箱事件总数。",
	})
	// publishSuccess 记录成功发布的事件总数。
	publishSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_publish_success_total",
		Help: "成功发布到消息队列的事件总数。",
	})
	// publishFailed 记录发布失败的事件总数。
	publishFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "outbox_relay_publish_failed_total",
		Help: "发布到消息队列失败的事件总数。",
	})
	// batchProcessDuration 记录处理一个批次事件的耗时分布。
	batchProcessDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "outbox_relay_batch_duration_seconds",
		Help:    "处理一个发件箱事件批次的耗时。",
		Buckets: prometheus.DefBuckets,
	})
)

// RelayService 负责轮询发件箱表并将事件转发到消息队列。
type RelayService struct {
	db        db.Database
	publisher mq.Publisher
	config    *models.OutboxConfig
	stopChan  chan struct{} // 用于优雅地停止服务的通道
	ticker    *time.Ticker  // 定时器，用于触发轮询
}

// NewRelayService 创建一个新的 RelayService 实例。
func NewRelayService(db db.Database, pub mq.Publisher, cfg *models.OutboxConfig) *RelayService {
	if cfg == nil { // 如果没有提供配置，则使用默认配置
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

// Start 启动 RelayService 的后台轮询任务。
func (s *RelayService) Start() {
	logger.Info("启动 Outbox 中继服务...", "component", componentName)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.processOutbox(context.Background())
			case <-s.stopChan:
				s.ticker.Stop()
				logger.Info("Outbox 中继服务已停止。", "component", componentName)
				return
			}
		}
	}()
}

// Stop 优雅地停止 RelayService。
func (s *RelayService) Stop() {
	logger.Info("正在停止 Outbox 中继服务...", "component", componentName)
	close(s.stopChan)
}

// processOutbox 是处理发件箱的核心逻辑。
// 它在一个事务中获取并锁定一批事件，尝试将它们发布到消息队列，然后删除已成功发布的事件。
func (s *RelayService) processOutbox(ctx context.Context) {
	timer := prometheus.NewTimer(batchProcessDuration)
	defer timer.ObserveDuration()

	var events []models.OutboxEvent
	err := s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		// 1. 获取并锁定一批待处理的事件。
		// 使用 "FOR UPDATE SKIP LOCKED" 是一种高效的并发处理策略，
		// 它允许不同的中继服务实例同时处理不同的事件批次，而不会相互阻塞。
		if err := tx.GetDB().WithContext(ctx).Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").Limit(s.config.BatchSize).Order("created_at asc").Find(&events).Error; err != nil {
			return err
		}

		if len(events) == 0 {
			return nil // 没有待处理的事件
		}

		var successIDs []uuid.UUID

		// 2. 尝试发布每个事件，并收集成功发布的事件ID。
		for _, event := range events {
			mqEvent := mq.Event{
				Exchange:   event.Exchange,
				RoutingKey: event.RoutingKey,
				Payload:    event.Payload,
			}

			if err := s.publisher.Publish(ctx, mqEvent); err != nil {
				// 如果发布失败，记录错误并继续处理批次中的下一个事件。
				// 失败的事件不会被添加到 successIDs 中，因此它将保留在数据库中，
				// 等待下一次轮询重试。
				// TODO: 为了防止“毒丸消息”（永远无法成功的消息）阻塞队列，可以引入重试计数和死信队列机制。
				logger.Error(err, "发布 Outbox 事件失败", "component", componentName, "event_id", event.ID)
				publishFailed.Inc()
				continue
			}

			publishSuccess.Inc()
			successIDs = append(successIDs, event.ID)
		}

		// 3. 从数据库中删除已成功发布的事件。
		if len(successIDs) > 0 {
			err := tx.Delete(ctx, &models.OutboxEvent{}, "id IN ?", successIDs)
			if err != nil {
				// 如果删除失败，整个事务将回滚。
				// 这将导致已经发布到消息队列的事件重新出现在数据库中，可能会在下次轮询时被重复发送。
				// 这是为了保证“至少一次”的投递语义，确保数据不会丢失。
				return err
			}
			eventsProcessed.Add(float64(len(successIDs)))
			logger.Info("已处理并发布 Outbox 事件", "component", componentName, "count", len(successIDs))
		}

		return nil
	})

	if err != nil {
		logger.Error(err, "处理 Outbox 批次时发生错误", "component", componentName)
	}
}
