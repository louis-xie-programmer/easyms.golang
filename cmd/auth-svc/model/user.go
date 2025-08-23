package model

import (
	"golang.org/x/crypto/bcrypt"
)

type UserDetails struct {
	// 用户标识
	UserId int64
	// 用户名 唯一
	Username string
	// 用户密码（明文，不存储到数据库）
	Password string
	// 用户密码哈希值（存储到数据库）
	PasswordHash string
	// 用户具有的权限
	Authorities []string // 具备的权限
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