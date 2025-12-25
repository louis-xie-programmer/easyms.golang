package service

import (
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/services/auth/internal/storage"
	"easyms/internal/shared/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockTokenStore 实现了 storage.TokenStore 接口，用于测试
type mockTokenStore struct {
	mock.Mock
}

func (m *mockTokenStore) ReadAccessToken(tokenValue string) (*model.OAuth2Token, error) {
	args := m.Called(tokenValue)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OAuth2Token), args.Error(1)
}

func (m *mockTokenStore) ReadOAuth2Details(tokenValue string) (*model.OAuth2Details, error) {
	args := m.Called(tokenValue)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OAuth2Details), args.Error(1)
}

func (m *mockTokenStore) RemoveAccessToken(tokenValue string) {
	m.Called(tokenValue)
}

func (m *mockTokenStore) RemoveRefreshToken(oauth2Token string) {
	m.Called(oauth2Token)
}

func (m *mockTokenStore) IsAccessTokenRevoked(tokenValue string) (bool, error) {
	args := m.Called(tokenValue)
	return args.Bool(0), args.Error(1)
}

// TestDefaultTokenService_CreateAccessToken 测试成功创建访问令牌
func TestDefaultTokenService_CreateAccessToken(t *testing.T) {
	// 1. 准备
	mockStore := new(mockTokenStore)
	// 对于 Enhance，我们使用真实的 JWT 增强器
	tokenEnhancer := storage.NewJwtTokenEnhancer("test-secret")
	tokenService := NewTokenService(mockStore, tokenEnhancer)

	clientDetails := &model.ClientDetails{
		ClientId:                    "test-client",
		AccessTokenValiditySeconds:  3600,
		RefreshTokenValiditySeconds: 7200,
	}
	userDetails := &model.UserDetails{
		UserId:   1,
		Username: "test-user",
	}
	oauth2Details := &model.OAuth2Details{
		Client: clientDetails,
		User:   userDetails,
	}

	// 2. 执行
	token, err := tokenService.CreateAccessToken(oauth2Details)

	// 3. 断言
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.NotEmpty(t, token.TokenValue)
	assert.NotNil(t, token.RefreshToken)
	assert.NotEmpty(t, token.RefreshToken.TokenValue)
	assert.WithinDuration(t, time.Now().Add(time.Hour), *token.ExpiresTime, time.Second*5)

	// 验证从生成的 token 中可以提取出正确的信息
	extractedToken, extractedDetails, err := tokenEnhancer.Extract(token.TokenValue)
	assert.NoError(t, err)
	assert.NotNil(t, extractedToken)
	assert.NotNil(t, extractedDetails)
	assert.Equal(t, "test-client", extractedDetails.Client.ClientId)
	assert.Equal(t, "test-user", extractedDetails.User.Username)
}

// TestDefaultTokenService_RefreshAccessToken 测试刷新令牌
func TestDefaultTokenService_RefreshAccessToken(t *testing.T) {
	// 1. 准备
	mockStore := new(mockTokenStore)
	tokenEnhancer := storage.NewJwtTokenEnhancer("test-secret")
	tokenService := NewTokenService(mockStore, tokenEnhancer)

	// 模拟一个有效的旧刷新令牌
	oldRefreshToken := &model.OAuth2Token{
		TokenValue:  "valid-refresh-token",
		ExpiresTime: func() *time.Time { t := time.Now().Add(2 * time.Hour); return &t }(),
	}
	oauth2Details := &model.OAuth2Details{
		Client: &model.ClientDetails{ClientId: "test-client", AccessTokenValiditySeconds: 3600, RefreshTokenValiditySeconds: 7200},
		User:   &model.UserDetails{Username: "test-user"},
	}

	// 设置 Mock 期望
	mockStore.On("ReadAccessToken", "valid-refresh-token").Return(oldRefreshToken, nil)
	mockStore.On("ReadOAuth2Details", "valid-refresh-token").Return(oauth2Details, nil)
	mockStore.On("RemoveRefreshToken", "valid-refresh-token").Return()

	// 2. 执行
	newToken, err := tokenService.RefreshAccessToken("valid-refresh-token")

	// 3. 断言
	assert.NoError(t, err)
	assert.NotNil(t, newToken)
	assert.NotEmpty(t, newToken.TokenValue)
	assert.NotEqual(t, "valid-refresh-token", newToken.RefreshToken.TokenValue, "A new refresh token should be generated")

	// 验证 Mock 是否被按预期调用
	mockStore.AssertExpectations(t)
}

// TestDefaultTokenService_RefreshAccessToken_Expired 测试刷新一个已过期的令牌
func TestDefaultTokenService_RefreshAccessToken_Expired(t *testing.T) {
	// 1. 准备
	mockStore := new(mockTokenStore)
	tokenEnhancer := storage.NewJwtTokenEnhancer("test-secret")
	tokenService := NewTokenService(mockStore, tokenEnhancer)

	// 模拟一个已过期的刷新令牌
	expiredRefreshToken := &model.OAuth2Token{
		TokenValue:  "expired-refresh-token",
		ExpiresTime: func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
	}

	// 设置 Mock 期望
	mockStore.On("ReadAccessToken", "expired-refresh-token").Return(expiredRefreshToken, nil)

	// 2. 执行
	_, err := tokenService.RefreshAccessToken("expired-refresh-token")

	// 3. 断言
	assert.Error(t, err)
	assert.Equal(t, consts.ErrExpiredToken, err)

	mockStore.AssertExpectations(t)
}
