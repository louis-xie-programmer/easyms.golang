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

// mockUserDetailsService 实现了 UserDetailsService 接口，用于模拟用户服务的行为。
// 它嵌入了 testify/mock.Mock，这使得我们可以预设方法的返回值并断言方法是否被调用。
type mockUserDetailsService struct {
	mock.Mock
}

// LoadUserByUsername 是接口方法的模拟实现。
func (m *mockUserDetailsService) LoadUserByUsername(username string) (*model.UserDetails, error) {
	// m.Called 会记录这次调用，并返回我们在测试用例中用 .On() 和 .Return() 预设的结果。
	args := m.Called(username)
	// 如果预设的第一个返回值为 nil，说明我们想模拟一个错误场景。
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// 否则，返回预设的用户详情对象。
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

// mockTokenService 实现了 TokenService 接口，用于模拟令牌服务的行为。
type mockTokenService struct {
	mock.Mock
}

// CreateAccessToken 是接口方法的模拟实现。
func (m *mockTokenService) CreateAccessToken(oauth2Details *model.OAuth2Details) (*model.OAuth2Token, error) {
	args := m.Called(oauth2Details)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.OAuth2Token), args.Error(1)
}

// 其他我们在这个测试中不关心的 TokenService 方法，可以留空或返回 nil。
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

// TestUsernamePasswordTokenGranter_Grant_Success 测试密码模式授权成功的核心流程。
func TestUsernamePasswordTokenGranter_Grant_Success(t *testing.T) {
	// --- 1. 准备阶段 (Arrange) ---

	// 创建所有依赖的 Mock 实例。
	mockUserSvc := new(mockUserDetailsService)
	mockTokenSvc := new(mockTokenService)
	// 创建被测试的对象：UsernamePasswordTokenGranter。
	granter := NewUsernamePasswordTokenGranter("password", mockUserSvc, mockTokenSvc)

	// 准备一个用于测试的、正确的用户对象。
	correctUser := &model.UserDetails{
		Username: "testuser",
		Password: "password", // 设置明文密码，以便 HashPassword 能正确工作。
	}
	// 为该用户生成密码哈希。
	err := correctUser.HashPassword()
	assert.NoError(t, err) // 断言哈希过程没有出错。

	// 准备一个模拟的客户端信息。
	clientDetails := &model.ClientDetails{ClientId: "test-client", AllowedAuthorities: "read,write"}

	// 准备模拟的HTTP请求内容。
	tokenRequest := &model.TokenRequest{
		Username: "testuser",
		Password: "password",
	}

	// 准备我们期望 TokenService 最终返回的令牌。
	expectedToken := &model.OAuth2Token{TokenValue: "success-token", ExpiresTime: func() *time.Time { t := time.Now().Add(time.Hour); return &t }()}

	// --- 编写“剧本”：为 Mock 对象预设行为 ---

	// 预设：当 mockUserSvc 的 LoadUserByUsername 方法被以 "testuser" 为参数调用时，
	// 应该返回我们准备好的 correctUser 对象，并且不返回错误。
	mockUserSvc.On("LoadUserByUsername", "testuser").Return(correctUser, nil)

	// 预设：当 mockUserSvc 的 GetUserAllowedScopes 方法被调用时，返回一个权限列表。
	mockUserSvc.On("GetUserAllowedScopes", correctUser.UserId).Return([]string{"read", "write"})

	// 预设：当 mockTokenSvc 的 CreateAccessToken 方法被调用时，我们期望它返回预设的 expectedToken。
	// 关键点：我们使用 mock.MatchedBy 来自定义参数的匹配逻辑。
	// 这是因为 CreateAccessToken 的参数 oauth2Details 是在 Grant 方法内部动态构建的，我们无法提前预知其指针地址。
	// 但我们可以通过这个函数来检查其内容是否符合预期。
	mockTokenSvc.On("CreateAccessToken", mock.MatchedBy(func(details *model.OAuth2Details) bool {
		// 我们只关心 Scopes 是否被正确计算，以及 Client 和 User 信息是否被正确传递。
		return details.Client.ClientId == "test-client" &&
			details.User.Username == "testuser" &&
			(details.Scopes == "read,write" || details.Scopes == "write,read") // 权限交集的顺序可能不同，所以两种情况都考虑。
	})).Return(expectedToken, nil)

	// --- 2. 执行阶段 (Act) ---

	// 调用我们需要测试的核心方法。
	token, err := granter.Grant(context.Background(), "password", clientDetails, tokenRequest)

	// --- 3. 断言阶段 (Assert) ---

	// 断言 Grant 方法没有返回错误。
	assert.NoError(t, err)
	// 断言返回的 token 不是 nil。
	assert.NotNil(t, token)
	// 断言返回的 token 值是我们预设的 "success-token"。
	assert.Equal(t, "success-token", token.TokenValue)

	// 验证所有在 .On() 中预设的期望行为是否都已经被准确地调用过。
	// 如果有任何一个预设的调用没有发生，这里会使测试失败。
	mockUserSvc.AssertExpectations(t)
	mockTokenSvc.AssertExpectations(t)
}

// TestUsernamePasswordTokenGranter_Grant_UserNotFound 测试当用户不存在时的失败场景。
func TestUsernamePasswordTokenGranter_Grant_UserNotFound(t *testing.T) {
	// 1. 准备
	mockUserSvc := new(mockUserDetailsService)
	mockTokenSvc := new(mockTokenService)
	granter := NewUsernamePasswordTokenGranter("password", mockUserSvc, mockTokenSvc)

	clientDetails := &model.ClientDetails{ClientId: "test-client"}
	tokenRequest := &model.TokenRequest{Username: "unknownuser", Password: "password"}

	// 预设“剧本”：当加载一个不存在的用户 "unknownuser" 时，返回一个错误。
	mockUserSvc.On("LoadUserByUsername", "unknownuser").Return(nil, errors.New("user not found"))

	// 2. 执行
	_, err := granter.Grant(context.Background(), "password", clientDetails, tokenRequest)

	// 3. 断言
	// 断言方法必须返回一个错误。
	assert.Error(t, err)
	// 断言返回的错误是我们预期的“无效用户名或密码”错误，而不是底层的“user not found”错误，这验证了错误被正确封装。
	assert.Equal(t, consts.ErrInvalidUsernameAndPasswordRequest, err)

	// 验证 mockUserSvc 的 LoadUserByUsername 方法确实被调用了。
	mockUserSvc.AssertCalled(t, "LoadUserByUsername", "unknownuser")
	// 关键断言：验证 mockTokenSvc 的 CreateAccessToken 方法在整个流程中 **从未** 被调用过。
	mockTokenSvc.AssertNotCalled(t, "CreateAccessToken", mock.Anything)
}

// TestUsernamePasswordTokenGranter_Grant_WrongPassword 测试当密码错误时的失败场景。
func TestUsernamePasswordTokenGranter_Grant_WrongPassword(t *testing.T) {
	// 1. 准备
	mockUserSvc := new(mockUserDetailsService)
	mockTokenSvc := new(mockTokenService)
	granter := NewUsernamePasswordTokenGranter("password", mockUserSvc, mockTokenSvc)

	// 准备一个密码为 "password" 的用户。
	correctUser := &model.UserDetails{Username: "testuser", Password: "password"}
	_ = correctUser.HashPassword()

	clientDetails := &model.ClientDetails{ClientId: "test-client"}
	// 准备一个密码错误的请求。
	tokenRequest := &model.TokenRequest{Username: "testuser", Password: "wrongpassword"}

	// 预设“剧本”：当加载用户 "testuser" 时，成功返回用户信息。
	mockUserSvc.On("LoadUserByUsername", "testuser").Return(correctUser, nil)

	// 2. 执行
	_, err := granter.Grant(context.Background(), "password", clientDetails, tokenRequest)

	// 3. 断言
	// 断言方法必须返回一个错误。
	assert.Error(t, err)
	// 断言返回的错误是“无效用户名或密码”错误。
	assert.Equal(t, consts.ErrInvalidUsernameAndPasswordRequest, err)

	// 验证 mockUserSvc 的 LoadUserByUsername 方法确实被调用了。
	mockUserSvc.AssertCalled(t, "LoadUserByUsername", "testuser")
	// 关键断言：验证 mockTokenSvc 的 CreateAccessToken 方法在密码错误时 **从未** 被调用过。
	mockTokenSvc.AssertNotCalled(t, "CreateAccessToken", mock.Anything)
}
