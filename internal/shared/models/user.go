package model

import (
	"golang.org/x/crypto/bcrypt"
	"strings"
)

// UserDetails 用户详情模型
type UserDetails struct {
	// 用户标识
	UserId int64 `json:"userId" gorm:"column:user_id;primaryKey;autoIncrement"`
	// 用户名 唯一
	Username string `json:"username" gorm:"column:username;type:varchar(64);not null"`
	// 用户密码（明文，不存储到数据库）
	Password string `json:"-" gorm:"-"`
	// 用户密码哈希值（存储到数据库）
	PasswordHash string `json:"passwordHash" gorm:"column:password_hash;type:varchar(128);not null"`
	// 用户具有的权限，多个权限用逗号分隔
	Authorities string `json:"authorities" gorm:"column:authorities;type:varchar(255)"`
}

// TableName 设置UserDetails模型对应的表名
func (UserDetails) TableName() string {
	return "user_details"
}

// GetAuthorities 获取权限列表
func (u *UserDetails) GetAuthorities() []string {
	if u.Authorities == "" {
		return []string{}
	}
	return strings.Split(u.Authorities, ",")
}

// SetAuthorities 设置权限列表
func (u *UserDetails) SetAuthorities(authorities []string) {
	u.Authorities = strings.Join(authorities, ",")
}

// HashPassword 使用bcrypt哈希密码
func (u *UserDetails) HashPassword() error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hashedPassword)
	return nil
}

// CheckPassword 验证密码是否匹配
func (u *UserDetails) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}