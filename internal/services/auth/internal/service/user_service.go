package service

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	. "easyms/internal/shared/models"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var (
	ErrPassword     = errors.New("invalid password")
	ErrUserNotExist = errors.New("user not exist")
)

// UserDetailsService 用户详情服务接口
// 定义了用户信息管理的标准方法
type UserDetailsService interface {
	// LoadUserByUsername 根据用户名加载用户详情
	LoadUserByUsername(ctx context.Context, username string) (*User, error)
	// GetUserAllowedScopes 获取用户允许的scope列表
	GetUserAllowedScopes(ctx context.Context, userId int64) []string
	// CreateUser 创建 用户
	CreateUserDetails(ctx context.Context, username, password string, authorities []string, clientId string) (*User, error)
}

// PostgresUserDetailsService 基于PostgreSQL的用户详情服务实现
type PostgresUserDetailsService struct {
	db db.Database
}

// NewPostgresUserDetailsService 创建新的PostgreSQL用户详情服务实例
// 参数:
//   - db: 数据库实例
//
// 返回值:
//   - UserDetailsService: 用户详情服务实例
func NewPostgresUserDetailsService(db db.Database) UserDetailsService {
	return &PostgresUserDetailsService{db: db}
}

// LoadUserByUsername 根据用户名加载用户详情
// 参数:
//   - username: 用户名
//
// 返回值:
//   - *UserDetails: 用户详情
//   - error: 操作成功返回nil，失败返回具体错误
func (service *PostgresUserDetailsService) LoadUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := service.db.Where(ctx, "username = ?", username).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotExist
		}
		logger.Error(err, "Failed to query user details", "auth-svc", "username", username)
		return nil, err
	}
	return &user, nil
}

func (service *PostgresUserDetailsService) GetUserAllowedScopes(ctx context.Context, userId int64) []string {
	var user User
	err := service.db.Where(ctx, "user_id = ?", userId).Select("authorities").First(&user).Error
	if err != nil {
		logger.Error(err, "Failed to get user allowed scopes", "auth-svc", "userId", userId)
		return []string{}
	}
	return user.GetAuthorities()
}

func (service *PostgresUserDetailsService) CreateUserDetails(ctx context.Context, username, password string, authorities []string, clientId string) (*User, error) {
	var count int64
	err := service.db.GetDB().WithContext(ctx).Model(&User{}).Where("username = ?", username).Count(&count).Error
	if err != nil {
		logger.Error(err, "Failed to check if user exists", "auth-svc", "username", username)
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("user %s already exists", username)
	}

	userDetails := &User{
		Username:    username,
		Authorities: strings.Join(authorities, ","),
	}

	if err := userDetails.SetPassword(password); err != nil {
		logger.Error(err, "Failed to hash password", "auth-svc", "username", username)
		return nil, err
	}

	// 使用事务确保用户和权限关系的一致性
	err = service.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		if err := tx.Insert(ctx, userDetails); err != nil {
			return err
		}

		userAuthority := &UserAuthority{
			UserID:   userDetails.ID,
			ClientID: clientId,
			Scope:    userDetails.Authorities,
		}
		if err := tx.Insert(ctx, userAuthority); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to create user and authority in transaction", "auth-svc", "username", username)
		return nil, err
	}

	return userDetails, nil
}
