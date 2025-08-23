package service

import (
	"context"
	"testing"

	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
)

func TestInMemoryUserDetailsService_GetUserDetailByUsername(t *testing.T) {
	// 创建测试用户数据
	users := []*model.UserDetails{
		{
			Username: "testuser",
			Password: "testpass",
		},
		{
			Username: "admin",
			Password: "adminpass",
		},
	}

	// 为用户生成密码哈希
	for _, user := range users {
		err := user.HashPassword()
		if err != nil {
			t.Fatalf("Failed to hash password: %v", err)
		}
		// 清除明文密码
		user.Password = ""
	}

	// 创建内存用户详情服务
	service := NewInMemoryUserDetailsService(users)

	// 测试获取存在的用户
	user, err := service.GetUserDetailByUsername(context.Background(), "testuser", "testpass")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Expected username to be 'testuser', got '%s'", user.Username)
	}

	// 测试获取不存在的用户
	_, err = service.GetUserDetailByUsername(context.Background(), "nonexistent", "password")
	if err != ErrUserNotExist {
		t.Errorf("Expected ErrUserNotExist, got %v", err)
	}

	// 测试错误密码
	_, err = service.GetUserDetailByUsername(context.Background(), "testuser", "wrongpass")
	if err != ErrPassword {
		t.Errorf("Expected ErrPassword, got %v", err)
	}
}

func TestInMemoryUserDetailsService_GetUserDetailByUsername_EmptyList(t *testing.T) {
	// 创建空用户列表的内存用户详情服务
	service := NewInMemoryUserDetailsService([]*model.UserDetails{})

	// 测试获取任何用户都应返回不存在错误
	_, err := service.GetUserDetailByUsername(context.Background(), "testuser", "testpass")
	if err != ErrUserNotExist {
		t.Errorf("Expected ErrUserNotExist, got %v", err)
	}
}
