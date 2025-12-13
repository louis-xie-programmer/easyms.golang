package model

import (
	"golang.org/x/crypto/bcrypt"
)

type UserDetails struct {
	// 用户标识
	UserId int64 `json:"userId" gorm:"primaryKey;autoIncrement"`
	// 用户名 唯一
	Username string `json:"username" gorm:"varchar(64);not null"`
	// 用户密码（明文，不存储到数据库）
	Password string `json:"-" gorm:"-"`
	// 用户密码哈希值（存储到数据库）
	PasswordHash string `json:"passwordHash" gorm:"varchar(128);not null"`
	// 用户具有的权限
	Authorities []string `json:"authorities" gorm:"type:varchar[]"`
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
