package model

import (
	"testing"
)

func TestUserDetails_HashPassword(t *testing.T) {
	user := &UserDetails{
		Username: "testuser",
		Password: "password123",
	}

	// 测试密码哈希
	err := user.HashPassword()
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// 验证密码哈希不为空
	if user.PasswordHash == "" {
		t.Error("PasswordHash should not be empty")
	}

	// 注意：根据当前实现，明文密码不会被自动清除
	// 如果需要清除明文密码，应在调用HashPassword后手动设置user.Password = ""
}

func TestUserDetails_CheckPassword(t *testing.T) {
	user := &UserDetails{
		Username: "testuser",
		Password: "password123",
	}

	// 测试密码哈希
	err := user.HashPassword()
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// 测试正确密码验证
	if !user.CheckPassword("password123") {
		t.Error("CheckPassword should return true for correct password")
	}

	// 测试错误密码验证
	if user.CheckPassword("wrongpassword") {
		t.Error("CheckPassword should return false for incorrect password")
	}
}

func TestUserDetails_CheckPassword_EmptyHash(t *testing.T) {
	user := &UserDetails{
		Username: "testuser",
	}

	// 测试空密码哈希验证
	if user.CheckPassword("password123") {
		t.Error("CheckPassword should return false for empty password hash")
	}
}