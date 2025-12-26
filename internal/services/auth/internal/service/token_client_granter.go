package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	. "easyms/internal/shared/models"
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
		return nil, consts.ErrNotSupportGrantType
	}

	// 根据客户端ID加载客户端详情
	clientDetails, err := tokenGranter.clientService.LoadClientByClientId(ctx, client.ClientId)
	if err != nil {
		return nil, consts.ErrInvalidClient
	}

	// 验证客户端密钥
	if clientDetails.ClientSecret != client.ClientSecret {
		return nil, consts.ErrInvalidClient
	}

	// 创建客户统一端访问令牌
	return tokenGranter.tokenService.CreateAccessToken(ctx, &OAuth2Details{
		Client: clientDetails,
		User:   nil,
	})
}
