// Package service 包含了用户服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/services/user/internal/storage" // 引入 storage 包
	"easyms/internal/shared/errors"                   // 引入共享的错误包
	"easyms/internal/shared/models"
	stdErrors "errors" // 为标准错误库设置别名
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// --- 业务错误定义 ---
var (
	ErrUserNotFound   = errors.New(errors.NotFoundCode, "用户未找到")
	ErrUsernameExists = errors.New(errors.ConflictCode, "用户名已存在")
	ErrEmailExists    = errors.New(errors.ConflictCode, "邮箱已存在")
)

// --- 服务接口定义 ---

// UserService 定义了用户服务的标准接口。
// 它抽象了所有与用户相关的业务操作。
type UserService interface {
	// Create 创建一个新用户。
	Create(ctx context.Context, username, password, email string) (*models.User, error)
	// GetByID 根据用户 ID 获取用户详情。
	GetByID(ctx context.Context, id uint) (*models.User, error)
}

// userServiceImpl 是 UserService 接口的具体实现。
// 它依赖于 UserStorage 接口来处理数据持久化。
type userServiceImpl struct {
	storage storage.UserStorage
}

// NewUserService 创建一个新的 UserService 实例。
// storage: 一个实现了 UserStorage 接口的实例，用于依赖注入。
func NewUserService(storage storage.UserStorage) UserService {
	return &userServiceImpl{
		storage: storage,
	}
}

// Create 在一个事务中创建一个新用户。
// 它首先构建用户模型，设置密码，然后委托给 storage 层进行持久化。
func (s *userServiceImpl) Create(ctx context.Context, username, password, email string) (*models.User, error) {
	user := &models.User{
		Username: username,
		Email:    email,
	}
	// 对密码进行哈希处理
	if err := user.SetPassword(password); err != nil {
		return nil, fmt.Errorf("密码哈希失败: %w", err)
	}

	// 使用 storage 层的事务来确保原子性
	err := s.storage.RunInTransaction(ctx, func(txStorage storage.UserStorage) error {
		if err := txStorage.Create(ctx, user); err != nil {
			// 检查是否为唯一约束冲突错误
			if strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "unique constraint") {
				if strings.Contains(err.Error(), "username") {
					return ErrUsernameExists
				}
				if strings.Contains(err.Error(), "email") {
					return ErrEmailExists
				}
			}
			return fmt.Errorf("创建用户失败: %w", err)
		}
		// 在这里可以扩展其他需要在同一事务中完成的操作，例如创建用户的初始配置等
		return nil
	})

	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetByID 根据 ID 获取用户。
// 它直接调用 storage 层的方法，并处理未找到记录的特定错误。
func (s *userServiceImpl) GetByID(ctx context.Context, id uint) (*models.User, error) {
	user, err := s.storage.GetByID(ctx, id)
	if err != nil {
		// 如果底层错误是 gorm.ErrRecordNotFound，则将其转换为我们定义的业务错误
		if stdErrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}
