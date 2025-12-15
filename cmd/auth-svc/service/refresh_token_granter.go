package service

import (
	"context"
	"easyms/cmd/auth-svc/consts"
	. "easyms/cmd/auth-svc/model"
)

type RefreshTokenGranter struct {
	supportGrantType string
	tokenService     TokenService
}

func NewRefreshGranter(grantType string, tokenService TokenService) TokenGranter {
	return &RefreshTokenGranter{
		supportGrantType: grantType,
		tokenService:     tokenService,
	}
}

func (tokenGranter *RefreshTokenGranter) Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	if grantType != tokenGranter.supportGrantType {
		return nil, consts.ErrNotSupportGrantType
	}
	// 从请求中获取刷新令牌
	refreshTokenValue := reader.RefreshToken

	if refreshTokenValue == "" {
		return nil, consts.ErrInvalidTokenRequest
	}

	// 刷新令牌
	return tokenGranter.tokenService.RefreshAccessToken(refreshTokenValue)
}
