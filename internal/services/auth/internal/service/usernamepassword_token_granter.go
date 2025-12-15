package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	. "easyms/internal/shared/models"
)

type UsernamePasswordTokenGranter struct {
	supportGrantType   string
	userDetailsService UserDetailsService
	tokenService       TokenService
}

func NewUsernamePasswordTokenGranter(grantType string, userDetailsService UserDetailsService, tokenService TokenService) TokenGranter {
	return &UsernamePasswordTokenGranter{
		supportGrantType:   grantType,
		userDetailsService: userDetailsService,
		tokenService:       tokenService,
	}
}

func (tokenGranter *UsernamePasswordTokenGranter) Grant(ctx context.Context,
	grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	if grantType != tokenGranter.supportGrantType {
		return nil, consts.ErrNotSupportGrantType
	}

	// 加载用户详情
	userDetails, err := tokenGranter.userDetailsService.LoadUserByUsername(reader.Username)
	if err != nil {
		return nil, consts.ErrInvalidUsernameAndPasswordRequest
	}

	// 验证密码
	if !userDetails.CheckPassword(reader.Password) {
		return nil, consts.ErrInvalidUsernameAndPasswordRequest
	}

	// 根据用户信息和客户端信息生成访问令牌
	return tokenGranter.tokenService.CreateAccessToken(&OAuth2Details{
		Client: client,
		User:   userDetails,
	})
}
