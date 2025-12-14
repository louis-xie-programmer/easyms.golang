package service

import (
	"context"
	. "easyms/cmd/auth-svc/model"
)

// ClientCredentialsTokenGranter 客户端凭证授权类型处理器
type ClientCredentialsTokenGranter struct {
	supportGrantType string
	clientService    ClientDetailsService
	tokenService     TokenService
}

func NewClientCredentialsTokenGranter(grantType string, clientService ClientDetailsService, tokenService TokenService) TokenGranter {
	return &ClientCredentialsTokenGranter{
		supportGrantType: grantType,
		clientService:    clientService,
		tokenService:     tokenService,
	}
}

func (tokenGranter *ClientCredentialsTokenGranter) Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	if grantType != tokenGranter.supportGrantType {
		return nil, ErrNotSupportGrantType
	}

	_, err := tokenGranter.clientService.GetClientDetailByClientId(ctx, client.ClientId, client.ClientSecret)

	if err != nil {
		return nil, ErrInvalidClient
	}

	// 创建客户统一端访问令牌
	return tokenGranter.tokenService.CreateAccessToken(&OAuth2Details{
		Client: client,
		User:   nil,
	})
}
