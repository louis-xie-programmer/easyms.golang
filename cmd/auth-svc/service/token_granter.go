package service

import (
	"context"
	"easyms/cmd/auth-svc/consts"
	. "easyms/cmd/auth-svc/model"
)

type TokenGranter interface {
	// Grant 用于生成令牌
	Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error)
}

type ComposeTokenGranter struct {
	TokenGrantDict map[string]TokenGranter
}

func NewComposeTokenGranter(tokenGrantDict map[string]TokenGranter) TokenGranter {
	return &ComposeTokenGranter{
		TokenGrantDict: tokenGrantDict,
	}
}

func (tokenGranter *ComposeTokenGranter) Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	// 查找对应的授权类型处理器
	dispatchGranter := tokenGranter.TokenGrantDict[grantType]

	if dispatchGranter == nil {
		return nil, consts.ErrNotSupportGrantType
	}

	return dispatchGranter.Grant(ctx, grantType, client, reader)
}
