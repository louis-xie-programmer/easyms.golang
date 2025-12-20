package service

import (
	"easyms/internal/shared/db"
	. "easyms/internal/shared/models"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrPassword     = errors.New("invalid password")
	ErrUserNotExist = errors.New("user not exist")
)

// UserDetailsService 用户详情服务接口
// 定义了用户信息管理的标准方法
type UserDetailsService interface {
	// LoadUserByUsername 根据用户名加载用户详情
	LoadUserByUsername(username string) (*UserDetails, error)
	// GetUserAllowedScopes 获取用户允许的scope列表
	GetUserAllowedScopes(userId int64) []string
	// CreateUser 创建 用户
	CreateUserDetails(username, password string, authorities []string, clientId string) (*UserDetails, error)
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

func (service *PostgresUserDetailsService) GetUserAllowedScopes(userId int64) []string {
	// 从数据库中获取用户的权限scope
	querySql := fmt.Sprintf("SELECT authorities FROM user_details WHERE user_id = %d", userId)

	var user UserDetails
	err := service.db.Query(&user, querySql)
	if err != nil {
		return []string{}
	}

	return user.GetAuthorities()
}

func (service *PostgresUserDetailsService) CreateUserDetails(username, password string, authorities []string, clientId string) (*UserDetails, error) {
	// 校验用户是否存在
	var count int64
	err := service.db.GetDB().Table("user_details").Count(&count).Error
	if err != nil {
		fmt.Printf("查询失败: %v\n", err)
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("用户已经存在")
	}

	userDetails := &UserDetails{
		Username:     username,
		PasswordHash: password,
		Authorities:  strings.Join(authorities, ","),
	}

	err = userDetails.HashPassword()
	if err != nil {
		return nil, err
	}
	userDetails.Password = ""

	err = service.db.Insert(userDetails)
	if err != nil {
		return nil, err
	}

	// 创建客户端与用户之间的关系
	err = service.db.Insert(&UserAuthority{
		UserID:   userDetails.UserId,
		ClientID: clientId,
		Scope:    userDetails.Authorities,
	})

	if err != nil {
		return nil, err
	}

	return userDetails, nil
}
