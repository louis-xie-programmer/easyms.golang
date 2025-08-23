package service

import (
	"testing"
	"time"

	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/storage"
)

func TestDefaultTokenService_CreateAccessToken(t *testing.T) {
	// 创建令牌增强器和存储
	enhancer := storage.NewJwtTokenEnhancer("test-secret")
	store := storage.NewJwtTokenStore(enhancer.(*storage.JwtTokenEnhancer), nil)

	// 创建默认令牌服务
	tokenService := NewTokenService(store, enhancer)

	// 创建测试数据
	user := &model.UserDetails{
		Username: "testuser",
		Password: "testpass",
	}

	err := user.HashPassword()
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	user.Password = ""

	client := &model.ClientDetails{
		ClientId:                    "testclient",
		AccessTokenValiditySeconds:  3600,
		RefreshTokenValiditySeconds: 7200,
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 测试创建访问令牌
	token, err := tokenService.CreateAccessToken(details)
	if err != nil {
		t.Fatalf("Failed to create access token: %v", err)
	}

	if token.TokenValue == "" {
		t.Error("Token value should not be empty")
	}

	if token.ExpiresTime == nil {
		t.Error("Token expiration time should not be nil")
	}

	if token.IsExpired() {
		t.Error("Token should not be expired")
	}
}

func TestDefaultTokenService_GetOAuth2DetailsByAccessToken(t *testing.T) {
	// 创建令牌增强器和存储
	enhancer := storage.NewJwtTokenEnhancer("test-secret")
	store := storage.NewJwtTokenStore(enhancer.(*storage.JwtTokenEnhancer), nil)

	// 创建默认令牌服务
	tokenService := NewTokenService(store, enhancer)

	// 创建测试数据
	user := &model.UserDetails{
		Username: "testuser",
		Password: "testpass",
	}

	err := user.HashPassword()
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	user.Password = ""

	client := &model.ClientDetails{
		ClientId:                    "testclient",
		AccessTokenValiditySeconds:  3600,
		RefreshTokenValiditySeconds: 7200,
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 创建访问令牌
	token, err := tokenService.CreateAccessToken(details)
	if err != nil {
		t.Fatalf("Failed to create access token: %v", err)
	}

	// 测试通过访问令牌获取OAuth2详情
	retrievedDetails, err := tokenService.GetOAuth2DetailsByAccessToken(token.TokenValue)
	if err != nil {
		t.Fatalf("Failed to get OAuth2 details by access token: %v", err)
	}

	if retrievedDetails.User.Username != user.Username {
		t.Errorf("Expected username to be '%s', got '%s'", user.Username, retrievedDetails.User.Username)
	}

	if retrievedDetails.Client.ClientId != client.ClientId {
		t.Errorf("Expected client ID to be '%s', got '%s'", client.ClientId, retrievedDetails.Client.ClientId)
	}
}

func TestDefaultTokenService_ReadAccessToken(t *testing.T) {
	// 创建令牌增强器和存储
	enhancer := storage.NewJwtTokenEnhancer("test-secret")
	store := storage.NewJwtTokenStore(enhancer.(*storage.JwtTokenEnhancer), nil)

	// 创建默认令牌服务
	tokenService := NewTokenService(store, enhancer)

	// 创建测试数据
	futureTime := time.Now().Add(time.Hour)
	expectedToken := &model.OAuth2Token{
		TokenValue:  "test-token",
		ExpiresTime: &futureTime,
	}

	user := &model.UserDetails{
		Username: "testuser",
		Password: "testpass",
	}

	err := user.HashPassword()
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	user.Password = ""

	client := &model.ClientDetails{
		ClientId: "testclient",
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 增强令牌
	enhancedToken, err := enhancer.Enhance(expectedToken, details)
	if err != nil {
		t.Fatalf("Failed to enhance token: %v", err)
	}

	// 测试读取访问令牌
	readToken, err := tokenService.ReadAccessToken(enhancedToken.TokenValue)
	if err != nil {
		t.Fatalf("Failed to read access token: %v", err)
	}

	if readToken.TokenValue != enhancedToken.TokenValue {
		t.Error("Read token value does not match enhanced token value")
	}
}