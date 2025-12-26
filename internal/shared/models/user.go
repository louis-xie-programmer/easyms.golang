package models

import (
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User 定义了基础的用户 GORM 模型
type User struct {
	ID           int64  `json:"id" gorm:"column:user_id;primaryKey;autoIncrement"`
	Username     string `json:"username" gorm:"column:username;type:varchar(64);not null"`
	PasswordHash string `json:"passwordHash" gorm:"column:password_hash;type:varchar(128);not null"`
	Email        string `json:"email" gorm:"column:email;type:varchar(100);"`
	// 用户具有的权限scope，多个权限用逗号分隔
	Authorities string `json:"authorities" gorm:"column:authorities;type:varchar(255)"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName 为 User 模型指定表名
func (User) TableName() string {
	return "users"
}

// GetAuthorities 获取权限列表
func (u *User) GetAuthorities() []string {
	if u.Authorities == "" {
		return []string{}
	}
	return strings.Split(u.Authorities, ",")
}

// SetAuthorities 设置权限列表
func (u *User) SetAuthorities(authorities []string) {
	u.Authorities = strings.Join(authorities, ",")
}

// SetPassword 设置并哈希用户的密码
func (u *User) SetPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hashedPassword)
	return nil
}

// CheckPassword 验证密码是否正确
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}
