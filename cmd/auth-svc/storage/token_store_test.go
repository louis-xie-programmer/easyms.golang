package storage

import (
	"testing"
	"time"

	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
)

func TestJwtTokenEnhancer_EnhanceAndExtract(t *testing.T) {
	// 创建令牌增强器
	enhancer := NewJwtTokenEnhancer("test-secret")

	// 创建测试数据
	futureTime := time.Now().Add(time.Hour)
	token := &model.OAuth2Token{
		TokenValue:  "test-token",
		ExpiresTime: &futureTime,
	}

	user := &model.UserDetails{
		Username: "testuser",
		Password: "testpass",
	}

	client := &model.ClientDetails{
		ClientId: "testclient",
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 测试增强令牌
	enhancedToken, err := enhancer.Enhance(token, details)
	if err != nil {
		t.Fatalf("Failed to enhance token: %v", err)
	}

	if enhancedToken.TokenValue == "" {
		t.Error("Enhanced token value should not be empty")
	}

	if enhancedToken.TokenType != "jwt" {
		t.Errorf("Expected token type to be 'jwt', got '%s'", enhancedToken.TokenType)
	}

	// 测试提取令牌
	extractedToken, extractedDetails, err := enhancer.Extract(enhancedToken.TokenValue)
	if err != nil {
		t.Fatalf("Failed to extract token: %v", err)
	}

	if extractedToken.TokenValue != enhancedToken.TokenValue {
		t.Error("Extracted token value does not match enhanced token value")
	}

	if extractedDetails.User.Username != user.Username {
		t.Errorf("Expected username to be '%s', got '%s'", user.Username, extractedDetails.User.Username)
	}

	if extractedDetails.Client.ClientId != client.ClientId {
		t.Errorf("Expected client ID to be '%s', got '%s'", client.ClientId, extractedDetails.Client.ClientId)
	}
}

func TestJwtTokenEnhancer_Extract_ExpiredToken(t *testing.T) {
	// 创建令牌增强器
	enhancer := NewJwtTokenEnhancer("test-secret")

	// 创建测试数据
	pastTime := time.Now().Add(-time.Hour)
	token := &model.OAuth2Token{
		TokenValue:  "test-token",
		ExpiresTime: &pastTime,
	}

	user := &model.UserDetails{
		Username: "testuser",
	}

	client := &model.ClientDetails{
		ClientId: "testclient",
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 测试增强令牌
	enhancedToken, err := enhancer.Enhance(token, details)
	if err != nil {
		t.Fatalf("Failed to enhance token: %v", err)
	}

	// 测试提取过期令牌
	_, _, err = enhancer.Extract(enhancedToken.TokenValue)
	if err == nil {
		t.Error("Expected error when extracting expired token")
	}
}

func TestJwtTokenStore_ReadAccessToken(t *testing.T) {
	// 创建令牌增强器
	enhancer := NewJwtTokenEnhancer("test-secret")
	
	// 创建JWT令牌存储
	store := NewJwtTokenStore(enhancer.(*JwtTokenEnhancer), nil)

	// 创建测试数据
	futureTime := time.Now().Add(time.Hour)
	token := &model.OAuth2Token{
		TokenValue:  "test-token",
		ExpiresTime: &futureTime,
	}

	user := &model.UserDetails{
		Username: "testuser",
	}

	client := &model.ClientDetails{
		ClientId: "testclient",
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 测试增强令牌
	enhancedToken, err := enhancer.Enhance(token, details)
	if err != nil {
		t.Fatalf("Failed to enhance token: %v", err)
	}

	// 测试读取访问令牌
	readToken, err := store.ReadAccessToken(enhancedToken.TokenValue)
	if err != nil {
		t.Fatalf("Failed to read access token: %v", err)
	}

	if readToken.TokenValue != enhancedToken.TokenValue {
		t.Error("Read token value does not match enhanced token value")
	}
}

func TestJwtTokenStore_ReadOAuth2Details(t *testing.T) {
	// 创建令牌增强器
	enhancer := NewJwtTokenEnhancer("test-secret")
	
	// 创建JWT令牌存储
	store := NewJwtTokenStore(enhancer.(*JwtTokenEnhancer), nil)

	// 创建测试数据
	futureTime := time.Now().Add(time.Hour)
	token := &model.OAuth2Token{
		TokenValue:  "test-token",
		ExpiresTime: &futureTime,
	}

	user := &model.UserDetails{
		Username: "testuser",
	}

	client := &model.ClientDetails{
		ClientId: "testclient",
	}

	details := &model.OAuth2Details{
		User:   user,
		Client: client,
	}

	// 测试增强令牌
	enhancedToken, err := enhancer.Enhance(token, details)
	if err != nil {
		t.Fatalf("Failed to enhance token: %v", err)
	}

	// 测试读取OAuth2详情
	readDetails, err := store.ReadOAuth2Details(enhancedToken.TokenValue)
	if err != nil {
		t.Fatalf("Failed to read OAuth2 details: %v", err)
	}

	if readDetails.User.Username != user.Username {
		t.Errorf("Expected username to be '%s', got '%s'", user.Username, readDetails.User.Username)
	}

	if readDetails.Client.ClientId != client.ClientId {
		t.Errorf("Expected client ID to be '%s', got '%s'", client.ClientId, readDetails.Client.ClientId)
	}
}