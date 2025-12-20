package model

import (
	"github.com/google/uuid"
	"time"
)

type UserAuthority struct {
	ID         uuid.UUID `json:"id" gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID     int64     `json:"user_id" gorm:"column:user_id;index"`
	ClientID   string    `json:"client_id" gorm:"column:client_id;type:varchar(64);index"`
	Scope      string    `json:"scope" gorm:"column:scope;type:varchar(256)"`
	CreateTime time.Time `json:"create_time" gorm:"column:create_time;default:now();type:timestamp"`
}

func (UserAuthority) TableName() string {
	return "user_authority"
}
