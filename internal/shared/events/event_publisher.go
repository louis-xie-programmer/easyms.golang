// Package events 提供了与事件发布相关的功能，特别是实现了 Outbox（发件箱）模式。
package events

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CreateAndStoreEvent 是实现 Outbox 模式的核心函数。
// 它在一个数据库事务中创建一个事件，并将其存储到 `outbox_events` 表中。
//
// 这种模式确保了业务操作和事件创建的原子性：
// 1. 业务数据被更新/创建。
// 2. 同一个事务中，一个描述该业务变化的事件被写入发件箱表。
// 3. 事务提交。如果任何一步失败，整个操作都会回滚，不会产生不一致的状态。
// 4. 一个独立的 "Relay" 服务会稍后读取发件箱表，并将事件真正地发布到消息队列。
//
// ctx: 上下文，用于传递请求范围的数据，如 trace ID。
// tx: 数据库事务对象，此函数的所有数据库操作都将在这个事务中执行。
// exchange: 消息队列的交换机名称。
// routingKey: 消息队列的路由键。
// payloadData: 事件的载荷数据，它将被序列化为 JSON 格式。
func CreateAndStoreEvent(ctx context.Context, tx db.TxTransaction, exchange, routingKey string, payloadData interface{}) error {
	// 将事件载荷序列化为 JSON 字节数组
	payload, err := json.Marshal(payloadData)
	if err != nil {
		return err
	}

	// 创建一个 OutboxEvent 模型实例
	event := &models.OutboxEvent{
		ID:         uuid.New(),      // 为事件生成一个唯一的 ID
		Exchange:   exchange,      // 目标交换机
		RoutingKey: routingKey,      // 目标路由键
		Payload:    payload,       // JSON 格式的事件载荷
		CreatedAt:  time.Now(),      // 事件创建时间
	}

	// 在传入的数据库事务中插入事件记录
	return tx.Insert(ctx, event)
}
