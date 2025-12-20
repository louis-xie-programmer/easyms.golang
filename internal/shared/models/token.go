package model

import "time"

type OAuth2Token struct {
	// 刷新令牌
	RefreshToken *OAuth2Token
	// 令牌类型
	TokenType string
	// 令牌
	TokenValue string
	// 过期时间
	ExpiresTime *time.Time
}

func (oauth2Token *OAuth2Token) IsExpired() bool {
	return oauth2Token.ExpiresTime != nil &&
		oauth2Token.ExpiresTime.Before(time.Now())
}

type OAuth2Details struct {
	Client *ClientDetails
	User   *UserDetails
	Scopes string
}

// RevokedToken 令牌撤销记录模型
// 用于存储已撤销的令牌信息
type RevokedToken struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	TokenValue string    `gorm:"column:token_value;uniqueIndex:idx_token_value;type:varchar(2048)"`
	Expiry     time.Time `gorm:"column:expiry;index"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

// TableName 设置RevokedToken模型对应的表名
func (RevokedToken) TableName() string {
	return "revoked_tokens"
}
