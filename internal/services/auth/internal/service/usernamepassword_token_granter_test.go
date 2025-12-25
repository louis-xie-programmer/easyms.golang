package service

import (
	"context"
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/shared/models"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockUserDetailsService 实现了 UserDetailsService 接口
type mockUserDetailsService struct {
	mock.Mock
}

func (m *mockUserDetailsService) LoadUserByUsername(username string) (*model.UserDetails, error) {
	args := m.Called(username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.UserDetails), args.Error(1)
}

func (m *mockUserDetailsService) GetUserAllowedScopes(userId int64) []string {
	args := m.Called(userId)
	return args.Get(0).([]string)
}

func (m *mockUserDetailsService) CreateUserDetails(username, password string, authorities []string, clientId string) (*model.UserDetails, error) {
	args := m.Called(username, password, authorities, clientId)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.UserDetails), args.Error(1)
}

// mockTokenService 实现了 TokenService 接口
type mockTokenService struct {
	mock.Mock
}

func (m *mockTokenService) CreateAccessToken(oauth2Details *model.OAuth2Details) (*model.OAuth2Token, error) {
	args := m.Called(oauth2Details)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OAuth2Token), args.Error(1)
}

// 其他 TokenService 方法的空实现
func (m *mockTokenService) GetOAuth2DetailsByAccessToken(tokenValue string) (*model.OAuth2Details, error) {
	return nil, nil
}
func (m *mockTokenService) RefreshAccessToken(refreshTokenValue string) (*model.OAuth2Token, error) {
	return nil, nil
}
func (m *mockTokenService) ReadAccessToken(tokenValue string) (*model.OAuth2Token, error) {
	return nil, nil
}
func (m *mockTokenService) ReadOAuth2Details(tokenValue string) (*model.OAuth2Details, error) {
	return nil, nil
}

func TestUsernamePasswordTokenGranter_Grant_Success(t *testing.T) {
	// 1. 准备
	mockUserSvc := new(mockUserDetailsService)
	mockTokenSvc := new(mockTokenService)
	granter := NewUsernamePasswordTokenGranter("password", mockUserSvc, mockTokenSvc)

	correctUser := &model.UserDetails{
		Username: "testuser",
		Password: "password",
	}
	err := correctUser.HashPassword()
	assert.NoError(t, err)

	clientDetails := &model.ClientDetails{ClientId: "test-client", AllowedAuthorities: "read,write"}
	tokenRequest := &model.TokenRequest{
		Username: "testuser",
		Password: "password",
	}
	expectedToken := &model.OAuth2Token{TokenValue: "success-token", ExpiresTime: func() *time.Time { t := time.Now().Add(time.Hour); return &t }()}

	// 设置 Mock 期望
	mockUserSvc.On("LoadUserByUsername", "testuser").Return(correctUser, nil)
	mockUserSvc.On("GetUserAllowedScopes", correctUser.UserId).Return([]string{"read", "write"})

	// 关键修正：使用 mock.MatchedBy 来自定义匹配逻辑
	mockTokenSvc.On("CreateAccessToken", mock.MatchedBy(func(details *model.OAuth2Details) bool {
		// 我们只关心 Scopes 是否被正确计算，其他字段在 Grant 方法中是直接透传的
		return details.Client.ClientId == "test-client" &&
			details.User.Username == "testuser" &&
			(details.Scopes == "read,write" || details.Scopes == "write,read") // 考虑顺序问题
	})).Return(expectedToken, nil)

	// 2. 执行
	token, err := granter.Grant(context.Background(), "password", clientDetails, tokenRequest)

	// 3. 断言
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, "success-token", token.TokenValue)

	// 验证 Mock 是否被按预期调用
	mockUserSvc.AssertExpectations(t)
	mockTokenSvc.AssertExpectations(t)
}

func TestUsernamePasswordTokenGranter_Grant_UserNotFound(t *testing.T) {
	// 1. 准备
	mockUserSvc := new(mockUserDetailsService)
	mockTokenSvc := new(mockTokenService)
	granter := NewUsernamePasswordTokenGranter("password", mockUserSvc, mockTokenSvc)

	clientDetails := &model.ClientDetails{ClientId: "test-client"}
	tokenRequest := &model.TokenRequest{Username: "unknownuser", Password: "password"}

	// 设置 Mock 期望：当加载用户时返回错误
	mockUserSvc.On("LoadUserByUsername", "unknownuser").Return(nil, errors.New("user not found"))

	// 2. 执行
	_, err := granter.Grant(context.Background(), "password", clientDetails, tokenRequest)

	// 3. 断言
	assert.Error(t, err)
	assert.Equal(t, consts.ErrInvalidUsernameAndPasswordRequest, err)

	// 验证 LoadUserByUsername 被调用，但 CreateAccessToken 不应该被调用
	mockUserSvc.AssertCalled(t, "LoadUserByUsername", "unknownuser")
	mockTokenSvc.AssertNotCalled(t, "CreateAccessToken", mock.Anything)
}

func TestUsernamePasswordTokenGranter_Grant_WrongPassword(t *testing.T) {
	// 1. 准备
	mockUserSvc := new(mockUserDetailsService)
	mockTokenSvc := new(mockTokenService)
	granter := NewUsernamePasswordTokenGranter("password", mockUserSvc, mockTokenSvc)

	correctUser := &model.UserDetails{Username: "testuser", Password: "password"}
	_ = correctUser.HashPassword() // 密码是 "password"

	clientDetails := &model.ClientDetails{ClientId: "test-client"}
	tokenRequest := &model.TokenRequest{Username: "testuser", Password: "wrongpassword"}

	// 设置 Mock 期望
	mockUserSvc.On("LoadUserByUsername", "testuser").Return(correctUser, nil)

	// 2. 执行
	_, err := granter.Grant(context.Background(), "password", clientDetails, tokenRequest)

	// 3. 断言
	assert.Error(t, err)
	assert.Equal(t, consts.ErrInvalidUsernameAndPasswordRequest, err)

	mockUserSvc.AssertCalled(t, "LoadUserByUsername", "testuser")
	mockTokenSvc.AssertNotCalled(t, "CreateAccessToken", mock.Anything)
}
