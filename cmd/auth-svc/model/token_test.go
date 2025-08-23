package model

import (
	"testing"
	"time"
)

func TestOAuth2Token_IsExpired(t *testing.T) {
	// 测试未过期令牌
	futureTime := time.Now().Add(time.Hour)
	token := &OAuth2Token{
		ExpiresTime: &futureTime,
	}
	
	if token.IsExpired() {
		t.Error("Token should not be expired")
	}

	// 测试已过期令牌
	pastTime := time.Now().Add(-time.Hour)
	token = &OAuth2Token{
		ExpiresTime: &pastTime,
	}
	
	if !token.IsExpired() {
		t.Error("Token should be expired")
	}

	// 测试空过期时间
	token = &OAuth2Token{
		ExpiresTime: nil,
	}
	
	if token.IsExpired() {
		t.Error("Token with nil expiration time should not be considered expired")
	}
}

func TestOAuth2Details(t *testing.T) {
	// 测试创建OAuth2Details
	user := &UserDetails{
		Username: "testuser",
	}
	
	client := &ClientDetails{
		ClientId: "testclient",
	}
	
	details := &OAuth2Details{
		User:   user,
		Client: client,
	}
	
	if details.User != user {
		t.Error("User details not set correctly")
	}
	
	if details.Client != client {
		t.Error("Client details not set correctly")
	}
}