// Package service 包含了认证服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	. "easyms/internal/shared/models"
	"strings"
)

// UsernamePasswordTokenGranter 是 TokenGranter 接口的一个具体实现，用于处理 "password" 授权模式。
// 它负责验证用户的用户名和密码，并在验证成功后授予令牌。
type UsernamePasswordTokenGranter struct {
	supportGrantType   string             // 该 Granter 支持的授权类型 (例如 "password")
	userDetailsService UserDetailsService // 用户详情服务，用于加载和验证用户信息
	tokenService       TokenService       // 令牌服务，用于创建最终的访问令牌
}

// NewUsernamePasswordTokenGranter 创建一个新的 UsernamePasswordTokenGranter 实例。
func NewUsernamePasswordTokenGranter(grantType string, userDetailsService UserDetailsService, tokenService TokenService) TokenGranter {
	return &UsernamePasswordTokenGranter{
		supportGrantType:   grantType,
		userDetailsService: userDetailsService,
		tokenService:       tokenService,
	}
}

// Grant 实现了 TokenGranter 接口的 Grant 方法，处理用户名密码的授权逻辑。
func (tokenGranter *UsernamePasswordTokenGranter) Grant(ctx context.Context,
	grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	// 1. 检查授权类型是否匹配
	if grantType != tokenGranter.supportGrantType {
		return nil, consts.ErrNotSupportGrantType
	}

	// 2. 加载用户详情
	userDetails, err := tokenGranter.userDetailsService.LoadUserByUsername(ctx, reader.Username)
	if err != nil {
		// 如果用户不存在，返回统一的错误信息，避免泄露用户信息
		return nil, consts.ErrInvalidUsernameAndPasswordRequest
	}

	// 3. 验证密码
	if !userDetails.CheckPassword(reader.Password) {
		return nil, consts.ErrInvalidUsernameAndPasswordRequest
	}

	// 4. 权限范围 (Scope) 计算
	// 最终授予令牌的权限是客户端允许的权限和用户拥有的权限的交集。
	allowedClientScopes := client.AllowedAuthorities // 客户端被允许申请的权限范围
	allowedUserScopes := tokenGranter.userDetailsService.GetUserAllowedScopes(ctx, userDetails.ID) // 用户实际拥有的权限
	finalScopes := intersect(allowedClientScopes, allowedUserScopes) // 计算交集

	// 5. 根据用户信息和客户端信息生成访问令牌
	return tokenGranter.tokenService.CreateAccessToken(ctx, &OAuth2Details{
		Client: client,
		User:   userDetails,
		Scopes: finalScopes,
	})
}

// intersect 计算两个权限集合的交集。
// allowedClientScopes: 以逗号分隔的字符串，代表客户端允许的权限。
// allowedUserScopes: 字符串切片，代表用户拥有的权限。
func intersect(allowedClientScopes string, allowedUserScopes []string) string {
	clientScopeSet := make(map[string]struct{})
	// 将客户端权限字符串转换为一个 set，便于快速查找
	for _, scope := range strings.Split(allowedClientScopes, ",") {
		clientScopeSet[strings.TrimSpace(scope)] = struct{}{}
	}

	var finalScopes []string
	// 遍历用户权限，如果某个权限也存在于客户端权限集合中，则将其加入最终结果
	for _, scope := range allowedUserScopes {
		if _, exists := clientScopeSet[strings.TrimSpace(scope)]; exists {
			finalScopes = append(finalScopes, scope)
		}
	}

	return strings.Join(finalScopes, ",")
}
