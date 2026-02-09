// Package service 包含了认证服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/services/auth/internal/storage"
	"easyms/internal/shared/errors" // 引入共享的错误包
	. "easyms/internal/shared/models"
	stdErrors "errors" // 为标准错误库设置别名，以避免与共享错误包冲突
	"github.com/satori/go.uuid"
	"strconv"
	"time"
)

// TokenService 定义了令牌管理服务的接口。
// 它抽象了所有与 OAuth2 令牌相关的操作，如创建、刷新和读取。
type TokenService interface {
	// GetOAuth2DetailsByAccessToken 通过访问令牌获取其关联的认证详情 (包括用户信息和客户端信息)。
	GetOAuth2DetailsByAccessToken(ctx context.Context, tokenValue string) (*OAuth2Details, error)
	// CreateAccessToken 为给定的认证详情创建一个新的访问令牌和刷新令牌。
	CreateAccessToken(ctx context.Context, oauth2Details *OAuth2Details) (*OAuth2Token, error)
	// RefreshAccessToken 使用一个刷新令牌来获取一个新的访问令牌。
	RefreshAccessToken(ctx context.Context, refreshTokenValue string) (*OAuth2Token, error)
	// ReadAccessToken 读取并验证一个访问令牌。
	ReadAccessToken(ctx context.Context, tokenValue string) (*OAuth2Token, error)
	// ReadOAuth2Details 读取与令牌关联的认证详情。
	ReadOAuth2Details(ctx context.Context, tokenValue string) (*OAuth2Details, error)
}

// 使用共享错误包预定义业务错误，以便在整个应用中统一处理。
var (
	ErrInvalidToken   = errors.New(errors.UnauthorizedCode, "无效的令牌")
	ErrTokenExpired   = errors.New(errors.UnauthorizedCode, "令牌已过期")
	ErrInternalServer = errors.New(errors.ServerErrorCode, "服务器内部错误")
)

// DefaultTokenService 是 TokenService 接口的默认实现。
type DefaultTokenService struct {
	tokenStore    storage.TokenStore    // 负责令牌的持久化和读取
	tokenEnhancer storage.TokenEnhancer // 负责令牌的生成 (签名) 和解析 (验证)
}

// NewTokenService 创建一个新的 DefaultTokenService 实例。
func NewTokenService(tokenStore storage.TokenStore, tokenEnhancer storage.TokenEnhancer) TokenService {
	return &DefaultTokenService{
		tokenStore:    tokenStore,
		tokenEnhancer: tokenEnhancer,
	}
}

// CreateAccessToken 为认证详情创建一个包含访问令牌和刷新令牌的完整 OAuth2Token。
func (s *DefaultTokenService) CreateAccessToken(ctx context.Context, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 1. 首先创建刷新令牌
	refreshToken, err := s.createRefreshToken(ctx, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}
	// 2. 然后创建访问令牌，访问令牌会包含对刷新令牌的引用
	accessToken, err := s.createAccessToken(ctx, refreshToken, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}
	return accessToken, nil
}

// createAccessToken 是一个内部辅助函数，用于创建访问令牌。
func (s *DefaultTokenService) createAccessToken(ctx context.Context, refreshToken *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	validitySeconds := oauth2Details.Client.AccessTokenValiditySeconds
	duration, _ := time.ParseDuration(strconv.Itoa(validitySeconds) + "s")
	expiredTime := time.Now().Add(duration)
	accessToken := &OAuth2Token{
		RefreshToken: refreshToken,
		ExpiresTime:  &expiredTime,
		TokenValue:   uuid.NewV4().String(), // 生成一个唯一的 JTI (JWT ID)
	}

	// 使用 TokenEnhancer (例如 JWT) 来签名并生成最终的令牌字符串
	if s.tokenEnhancer != nil {
		return s.tokenEnhancer.Enhance(accessToken, oauth2Details)
	}
	return accessToken, nil
}

// createRefreshToken 是一个内部辅助函数，用于创建刷新令牌。
func (s *DefaultTokenService) createRefreshToken(ctx context.Context, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	validitySeconds := oauth2Details.Client.RefreshTokenValiditySeconds
	duration, _ := time.ParseDuration(strconv.Itoa(validitySeconds) + "s")
	expiredTime := time.Now().Add(duration)
	refreshToken := &OAuth2Token{
		ExpiresTime: &expiredTime,
		TokenValue:  uuid.NewV4().String(), // 生成一个唯一的 JTI (JWT ID)
	}

	// 刷新令牌同样需要被签名，以包含必要的信息
	if s.tokenEnhancer != nil {
		return s.tokenEnhancer.Enhance(refreshToken, oauth2Details)
	}
	return refreshToken, nil
}

// RefreshAccessToken 使用一个有效的刷新令牌来获取一个新的访问令牌和刷新令牌。
func (s *DefaultTokenService) RefreshAccessToken(ctx context.Context, refreshTokenValue string) (*OAuth2Token, error) {
	// 1. 读取旧的刷新令牌，这一步会检查它是否已被撤销
	refreshToken, err := s.tokenStore.ReadAccessToken(refreshTokenValue)
	if err != nil {
		if stdErrors.Is(err, storage.ErrTokenRevoked) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	// 2. 检查令牌是否已过期
	if refreshToken.IsExpired() {
		s.tokenStore.RemoveRefreshToken(refreshTokenValue) // 从存储中移除过期的令牌
		return nil, ErrTokenExpired
	}

	// 3. 读取与该刷新令牌关联的原始认证详情
	oauth2Details, err := s.tokenStore.ReadOAuth2Details(refreshTokenValue)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// 4. 创建新的刷新令牌和访问令牌
	newRefreshToken, err := s.createRefreshToken(ctx, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}
	newAccessToken, err := s.createAccessToken(ctx, newRefreshToken, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}

	// 5. 安全地移除旧的刷新令牌，以防止重放攻击 (Refresh Token Rotation)
	s.tokenStore.RemoveRefreshToken(refreshTokenValue)

	return newAccessToken, nil
}

// ReadAccessToken 读取并验证一个访问令牌。
func (s *DefaultTokenService) ReadAccessToken(ctx context.Context, tokenValue string) (*OAuth2Token, error) {
	return s.tokenStore.ReadAccessToken(tokenValue)
}

// ReadOAuth2Details 读取与令牌关联的认证详情。
func (s *DefaultTokenService) ReadOAuth2Details(ctx context.Context, tokenValue string) (*OAuth2Details, error) {
	return s.tokenStore.ReadOAuth2Details(tokenValue)
}

// GetOAuth2DetailsByAccessToken 是一个便捷方法，用于通过访问令牌直接获取认证详情。
func (s *DefaultTokenService) GetOAuth2DetailsByAccessToken(ctx context.Context, tokenValue string) (*OAuth2Details, error) {
	// 1. 读取并验证访问令牌
	accessToken, err := s.tokenStore.ReadAccessToken(tokenValue)
	if err != nil {
		if stdErrors.Is(err, storage.ErrTokenRevoked) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	// 2. 检查令牌是否过期
	if accessToken.IsExpired() {
		return nil, ErrTokenExpired
	}

	// 3. 读取完整的认证详情
	return s.tokenStore.ReadOAuth2Details(tokenValue)
}
