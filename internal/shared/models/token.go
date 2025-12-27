package models

import "time"

type OAuth2Token struct {
	RefreshToken *OAuth2Token
	TokenType    string
	TokenValue   string
	ExpiresTime  *time.Time
}

func (oauth2Token *OAuth2Token) IsExpired() bool {
	return oauth2Token.ExpiresTime != nil &&
		oauth2Token.ExpiresTime.Before(time.Now())
}

type OAuth2Details struct {
	Client *ClientDetails
	User   *User
	Scopes string
}

// RevokedToken stores revoked token information.
// It stores a hash of the token to improve performance and security.
type RevokedToken struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	TokenHash string    `gorm:"column:token_hash;uniqueIndex:idx_token_hash;type:varchar(64)"`
	Expiry    time.Time `gorm:"column:expiry;index"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName sets the table name for the RevokedToken model.
func (RevokedToken) TableName() string {
	return "revoked_tokens"
}
