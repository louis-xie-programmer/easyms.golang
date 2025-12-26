package events

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CreateAndStoreEvent 创建并存储一个发件箱事件到数据库事务中
func CreateAndStoreEvent(ctx context.Context, tx db.TxTransaction, exchange, routingKey string, payloadData interface{}) error {
	payload, err := json.Marshal(payloadData)
	if err != nil {
		return err
	}

	event := &models.OutboxEvent{
		ID:         uuid.New(),
		Exchange:   exchange,
		RoutingKey: routingKey,
		Payload:    payload,
		CreatedAt:  time.Now(),
	}

	return tx.Insert(ctx, event)
}
