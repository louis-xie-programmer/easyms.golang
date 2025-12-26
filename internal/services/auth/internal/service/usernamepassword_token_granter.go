package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	. "easyms/internal/shared/models"
	"strings"
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

	// 1. 客户端维度校验（客户端注册时预设的权限范围）
	allowedClientScopes := client.AllowedAuthorities // 从数据库加载（如 "read write"）

	// 2. 用户维度校验（用户实际拥有的权限）
	allowedUserScopes := tokenGranter.userDetailsService.GetUserAllowedScopes(userDetails.ID)

	// 3. 交集运算生成最终有效scope
	finalScopes := intersect(allowedClientScopes, allowedUserScopes)

	// 根据用户信息和客户端信息生成访问令牌
	return tokenGranter.tokenService.CreateAccessToken(&OAuth2Details{
		Client: client,
		User:   userDetails,
		Scopes: finalScopes,
	})
}

func intersect(allowedClientScopes string, allowedUserScopes []string) string {
	clientScopeSet := make(map[string]struct{})
	for _, scope := range strings.Split(allowedClientScopes, ",") {
		clientScopeSet[scope] = struct{}{}
	}

	var finalScopes []string
	for _, scope := range allowedUserScopes {
		if _, exists := clientScopeSet[scope]; exists {
			finalScopes = append(finalScopes, scope)
		}
	}

	return strings.Join(finalScopes, ",")
}
