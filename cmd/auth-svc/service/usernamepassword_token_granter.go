package service

import (
	"context"
	. "easyms/cmd/auth-svc/model"
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
		return nil, ErrNotSupportGrantType
	}

	userDetailsService := tokenGranter.userDetailsService.(*PostgresUserDetailsService)

	// 验证用户名密码是否正确
	userDetails, err := userDetailsService.GetUserDetailByUsername(ctx, reader.Username, reader.Password)

	if err != nil {
		return nil, ErrInvalidUsernameAndPasswordRequest
	}

	// 根据用户信息和客户端信息生成访问令牌
	return tokenGranter.tokenService.CreateAccessToken(&OAuth2Details{
		Client: client,
		User:   userDetails,
	})

}
