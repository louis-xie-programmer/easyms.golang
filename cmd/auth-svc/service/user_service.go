package service

import (
	"context"
	"easyms/cmd/auth-svc/model"
	"easyms/pkg/db"
	"errors"
)

var (
	// ErrUserNotExist 用户不存在
	ErrUserNotExist = errors.New("username is not exist")
	// ErrPassword 密码错误
	ErrPassword = errors.New("invalid password")
)

type UserDetailsService interface {
	// GetUserDetailByUsername 根据用户名和密码获取用户详情
	GetUserDetailByUsername(ctx context.Context, username, password string) (*model.UserDetails, error)
}

// PostgresUserDetailsService postgres用户详情服务
type PostgresUserDetailsService struct {
	db *db.EasyDatabase
}

func NewPostgresUserDetailsService(db *db.EasyDatabase) UserDetailsService {
	return &PostgresUserDetailsService{
		db: db,
	}
}

func (service *PostgresUserDetailsService) GetUserDetailByUsername(ctx context.Context, username, password string) (*model.UserDetails, error) {
	var userDetails model.UserDetails
	err := service.db.Query(&userDetails, "select * from user_details where username = ?", username)
	if err != nil {
		return nil, err
	}

	// 使用bcrypt验证密码
	if !userDetails.CheckPassword(password) {
		return nil, ErrPassword
	}

	return &userDetails, nil
}

// InMemoryUserDetailsService 内存用户详情服务
type InMemoryUserDetailsService struct {
	userDetailsDict map[string]*model.UserDetails
}

func (service *InMemoryUserDetailsService) GetUserDetailByUsername(ctx context.Context, username, password string) (*model.UserDetails, error) {
	// 根据 username 获取用户信息
	userDetails, ok := service.userDetailsDict[username]
	if ok {
		// 比较 password 是否匹配
		if userDetails.CheckPassword(password) {
			return userDetails, nil
		} else {
			return nil, ErrPassword
		}
	} else {
		return nil, ErrUserNotExist
	}

}

func NewInMemoryUserDetailsService(userDetailsList []*model.UserDetails) UserDetailsService {
	userDetailsDict := make(map[string]*model.UserDetails)

	if userDetailsList != nil {
		for _, value := range userDetailsList {
			// 为内存中的用户生成密码哈希
			if value.Password != "" && value.PasswordHash == "" {
				value.HashPassword()
			}
			userDetailsDict[value.Username] = value
		}
	}

	return &InMemoryUserDetailsService{
		userDetailsDict: userDetailsDict,
	}
}
