// Package service 包含了认证服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	. "easyms/internal/shared/models"
)

// RefreshTokenGranter 是 TokenGranter 接口的一个具体实现，用于处理 "refresh_token" 授权模式。
// 它负责使用一个有效的刷新令牌来获取新的访问令牌。
type RefreshTokenGranter struct {
	supportGrantType string // 该 Granter 支持的授权类型 (例如 "refresh_token")
	tokenService     TokenService // 令牌服务，用于执行实际的令牌刷新操作
}

// NewRefreshGranter 创建一个新的 RefreshTokenGranter 实例。
// grantType: 该 Granter 将支持的授权类型字符串。
// tokenService: 令牌服务接口的实例。
func NewRefreshGranter(grantType string, tokenService TokenService) TokenGranter {
	return &RefreshTokenGranter{
		supportGrantType: grantType,
		tokenService:     tokenService,
	}
}

// Grant 实现了 TokenGranter 接口的 Grant 方法，处理刷新令牌的授权逻辑。
func (tokenGranter *RefreshTokenGranter) Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	// 1. 检查授权类型是否匹配
	if grantType != tokenGranter.supportGrantType {
		return nil, consts.ErrNotSupportGrantType
	}

	// 2. 从请求中提取刷新令牌的值
	refreshTokenValue := reader.RefreshToken

	// 3. 验证刷新令牌是否存在
	if refreshTokenValue == "" {
		return nil, consts.ErrInvalidTokenRequest
	}

	// 4. 调用 TokenService 执行刷新操作，获取新的访问令牌
	return tokenGranter.tokenService.RefreshAccessToken(ctx, refreshTokenValue)
}
