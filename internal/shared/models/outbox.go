package models

import (
	"time"

	"github.com/google/uuid"
)

// OutboxEvent 代表存储在发件箱表中的事件记录
type OutboxEvent struct {
	ID         uuid.UUID `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Exchange   string    `gorm:"type:varchar(255);not null"`
	RoutingKey string    `gorm:"type:varchar(255);not null"`
	Payload    []byte    `gorm:"type:bytea;not null"`
	CreatedAt  time.Time `gorm:"index"`
}

// TableName 为 OutboxEvent 模型指定表名
func (OutboxEvent) TableName() string {
	return "outbox_events"
}
