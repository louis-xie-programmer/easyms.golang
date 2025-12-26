package service

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
	"fmt"
)

// UserService 定义了用户服务的接口
type UserService interface {
	Create(ctx context.Context, username, password, email string) (*models.User, error)
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// userServiceImpl 实现了 UserService 接口
type userServiceImpl struct {
	db db.Database
}

// NewUserService 创建一个新的用户服务实例
func NewUserService(db db.Database) UserService {
	return &userServiceImpl{
		db: db,
	}
}

// Create 创建一个新用户
func (s *userServiceImpl) Create(ctx context.Context, username, password, email string) (*models.User, error) {
	user := &models.User{
		Username: username,
		Email:    email,
	}
	if err := user.SetPassword(password); err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.db.Insert(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user in db: %w", err)
	}

	return user, nil
}

// GetByID 根据 ID 获取一个用户
func (s *userServiceImpl) GetByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	err := s.db.GetDB().WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
