package service

import (
	"context"
	"errors"
	"fmt"
	. "easyms/cmd/auth-svc/model"
	"easyms/pkg/db"
)

var (
	ErrPassword    = errors.New("invalid password")
	ErrUserNotExist = errors.New("user not exist")
)

// UserDetailsService 用户详情服务接口
// 定义了用户信息管理的标准方法
type UserDetailsService interface {
	// LoadUserByUsername 根据用户名加载用户详情
	LoadUserByUsername(username string) (*UserDetails, error)
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
func (service *PostgresUserDetailsService) LoadUserByUsername(username string) (*UserDetails, error) {
	// 构造查询SQL
	querySql := fmt.Sprintf("SELECT username, password_hash, authorities FROM user_details WHERE username = '%s'", username)

	// 执行查询
	var user UserDetails
	err := service.db.Query(&user, querySql)
	if err != nil {
		return nil, err
	}

	// 构造用户详情对象
	userDetails := &UserDetails{
		Username:     user.Username,
		PasswordHash: user.PasswordHash,
		Authorities:  user.Authorities,
	}

	return userDetails, nil
}

// InMemoryUserDetailsService 内存用户详情服务
type InMemoryUserDetailsService struct {
	userDetailsDict map[string]*UserDetails
}

func (service *InMemoryUserDetailsService) GetUserDetailByUsername(ctx context.Context, username, password string) (*UserDetails, error) {
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

func (service *InMemoryUserDetailsService) LoadUserByUsername(username string) (*UserDetails, error) {
	// 根据 username 获取用户信息
	userDetails, ok := service.userDetailsDict[username]
	if ok {
		return userDetails, nil
	} else {
		return nil, ErrUserNotExist
	}
}

func NewInMemoryUserDetailsService(userDetailsList []*UserDetails) UserDetailsService {
	userDetailsDict := make(map[string]*UserDetails)

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