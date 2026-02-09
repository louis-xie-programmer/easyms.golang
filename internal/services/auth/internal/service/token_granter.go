// Package service 包含了认证服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	. "easyms/internal/shared/models"
)

// TokenGranter 定义了令牌授予者的标准接口。
// 在 OAuth2 中，不同的授权模式 (grant_type) 有不同的令牌授予逻辑。
// 例如，"password" 模式需要验证用户名和密码，而 "refresh_token" 模式需要验证刷新令牌。
// 这个接口就是对这些不同逻辑的抽象。
type TokenGranter interface {
	// Grant 是授予令牌的核心方法。
	// 它根据传入的 grantType 和其他参数，执行相应的授权逻辑并返回一个 OAuth2Token。
	Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error)
}

// ComposeTokenGranter 是一个组合的 TokenGranter 实现。
// 它内部维护一个从 grant_type 字符串到具体 TokenGranter 实现的映射。
// 当它的 Grant 方法被调用时，它会根据传入的 grant_type 查找并调用相应的具体实现。
// 这是一种策略模式 (Strategy Pattern) 的应用。
type ComposeTokenGranter struct {
	TokenGrantDict map[string]TokenGranter // 存储 grant_type 到具体 TokenGranter 的映射
}

// NewComposeTokenGranter 创建一个新的 ComposeTokenGranter 实例。
// tokenGrantDict: 一个预先配置好的、包含了所有支持的授权模式及其处理器的 map。
func NewComposeTokenGranter(tokenGrantDict map[string]TokenGranter) TokenGranter {
	return &ComposeTokenGranter{
		TokenGrantDict: tokenGrantDict,
	}
}

// Grant 实现了 TokenGranter 接口的 Grant 方法。
// 它作为授权逻辑的分发器 (Dispatcher)。
func (tokenGranter *ComposeTokenGranter) Grant(ctx context.Context, grantType string, client *ClientDetails, reader *TokenRequest) (*OAuth2Token, error) {
	// 1. 根据 grantType 从字典中查找对应的授权处理器
	dispatchGranter := tokenGranter.TokenGrantDict[grantType]

	// 2. 如果找不到对应的处理器，说明不支持该授权模式
	if dispatchGranter == nil {
		return nil, consts.ErrNotSupportGrantType
	}

	// 3. 调用找到的具体处理器来执行授权逻辑
	return dispatchGranter.Grant(ctx, grantType, client, reader)
}
