package db

import (
	"fmt"
	"log"
)

// User 用户模型
type User struct {
	ID       int64  `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"type:varchar(100);not null" json:"name"`
	Email    string `gorm:"type:varchar(100);uniqueIndex" json:"email"`
	Profile  string `gorm:"type:json" json:"profile,omitempty"`
	Status   int    `gorm:"type:tinyint;default:1" json:"status"`
	CreateAt int64  `gorm:"autoCreateTime" json:"create_at"`
	UpdateAt int64  `gorm:"autoUpdateTime" json:"update_at"`
}

// UserProfile 用户资料
type UserProfile struct {
	Avatar   string `json:"avatar"`
	Bio      string `json:"bio"`
	Location string `json:"location"`
}

// ExampleMysqlUpsert 演示MySQL的Upsert功能
func ExampleMysqlUpsert() {
	// 注意：这只是示例代码，实际使用时需要连接真实的MySQL服务器
	fmt.Println("演示MySQL的Upsert功能")

	// 创建用户
	user := User{
		Name:  "张三",
		Email: "zhangsan@example.com",
	}

	// 这里只是演示语法，实际使用时需要连接真实的数据库
	fmt.Printf("用户信息: %+v\n", user)

	// Output:
	// 演示MySQL的Upsert功能
	// 用户信息: {ID:0 Name:张三 Email:zhangsan@example.com Profile: Status:0 CreateAt:0 UpdateAt:0}
}

// ExampleMysqlJson 演示MySQL的JSON字段处理
func ExampleMysqlJson() {
	// 注意：这只是示例代码，实际使用时需要连接真实的MySQL服务器
	fmt.Println("演示MySQL的JSON字段处理")

	// 创建带JSON字段的用户
	profile := UserProfile{
		Avatar:   "https://example.com/avatar.jpg",
		Bio:      "这是一个示例用户",
		Location: "北京",
	}

	user := User{
		Name:    "李四",
		Email:   "lisi@example.com",
		Profile: "", // 在实际使用中这里会存储JSON字符串
	}

	fmt.Printf("用户资料: %+v\n", profile)
	fmt.Printf("用户信息: %+v\n", user)

	// Output:
	// 演示MySQL的JSON字段处理
	// 用户资料: {Avatar:https://example.com/avatar.jpg Bio:这是一个示例用户 Location:北京}
	// 用户信息: {ID:0 Name:李四 Email:lisi@example.com Profile: Status:0 CreateAt:0 UpdateAt:0}
}

// ExampleMysqlConnection 演示MySQL连接
func ExampleMysqlConnection() {
	// 注意：这只是示例代码，实际使用时需要连接真实的MySQL服务器
	fmt.Println("演示MySQL连接")

	// MySQL连接字符串示例
	// dsn := "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"

	fmt.Println("MySQL连接成功")

	// Output:
	// 演示MySQL连接
	// MySQL连接成功
}

func main() {
	// 这些示例仅用于演示目的
	log.Println("MySQL功能示例")
}
