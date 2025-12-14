// main.go 数据库迁移工具主程序
// 主要功能：
// 1. 解析命令行参数
// 2. 初始化数据库迁移实例
// 3. 执行向上或向下迁移操作
// 4. 支持基于模型的自动迁移
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	. "easyms/cmd/auth-svc/model"
	"easyms/pkg/db"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// main 数据库迁移工具入口函数
// 根据命令行参数执行数据库迁移操作
func main() {
	// 定义命令行参数变量
	var migrationsPath, dbURL string
	var up, down, auto bool

	// 定义命令行参数
	// -path: 迁移文件路径，默认为"./migrations"
	// -database: 数据库连接URL，必须提供
	// -up: 执行向上迁移操作
	// -down: 执行向下迁移操作
	// -auto: 使用GORM AutoMigrate自动创建表结构
	flag.StringVar(&migrationsPath, "path", "./migrations", "migrations path")
	flag.StringVar(&dbURL, "database", "", "database url")
	flag.BoolVar(&up, "up", false, "apply all up migrations")
	flag.BoolVar(&down, "down", false, "apply all down migrations")
	flag.BoolVar(&auto, "auto", false, "use GORM AutoMigrate to create tables")
	flag.Parse()

	// 检查数据库URL是否提供，如果没有则终止程序
	if dbURL == "" {
		log.Fatal("database url is required")
	}

	// 如果启用自动迁移模式
	if auto {
		// 解析数据库连接URL以获取数据库类型和连接字符串
		// 这里简单处理，实际项目中可能需要更复杂的解析
		var dbType string
		var connStr = dbURL

		if len(dbURL) >= 5 && dbURL[0:5] == "postg" {
			dbType = "postgres"
		} else if len(dbURL) >= 5 && dbURL[0:5] == "mysql" {
			dbType = "mysql"
		} else {
			log.Fatal("Unsupported database type in URL")
		}

		// 创建数据库连接
		database, err := db.NewEasyDatabase(dbType, connStr)
		if err != nil {
			log.Fatal("Failed to connect to database:", err)
		}

		// 执行自动迁移
		// 迁移所有auth-svc相关的模型
		models := []interface{}{
			&ClientDetails{},
			&UserDetails{},
			&RevokedToken{},
		}

		err = database.AutoMigrate(models...)
		if err != nil {
			// 检查是否是约束不存在的错误，如果是则忽略
			if strings.Contains(err.Error(), "constraint") && strings.Contains(err.Error(), "does not exist") {
				log.Printf("Warning: Constraint error during auto migration (can be ignored): %v", err)
				// 继续执行而不是终止
			} else {
				log.Printf("Auto migration failed: %v", err)
			}
		} else {
			fmt.Println("Auto migration completed successfully")
		}

		// 创建测试用户和客户端(并保存到数据库)
		createTestUsers(database)
		createTestClients(database)

		return
	}

	// 创建迁移实例
	// 使用文件系统作为迁移源，数据库作为目标
	m, err := migrate.New(
		"file://"+migrationsPath,
		dbURL,
	)
	if err != nil {
		log.Fatal(err)
	}

	// 如果指定了-up参数，则执行向上迁移
	// 向上迁移会应用所有未执行的迁移脚本
	if up {
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		fmt.Println("Migrations applied successfully")
	}

	// 如果指定了-down参数，则执行向下迁移
	// 向下迁移会回滚所有已执行的迁移脚本
	if down {
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
		fmt.Println("Migrations rolled back successfully")
	}
}

// createTestUsers 创建测试用户
// 创建默认测试用户并保存到数据库
func createTestUsers(dbase db.Database) map[string]*UserDetails {
	users := map[string]*UserDetails{
		"user1": {
			Username:    "user1",
			Password:    "password1",
			Authorities: "USER",
		},
		"admin": {
			Username:    "admin",
			Password:    "password2",
			Authorities: "USER,ADMIN",
		},
	}

	// 为用户生成密码哈希
	for _, user := range users {
		err := user.HashPassword()
		if err != nil {
			fmt.Printf("Failed to hash password for user %s: %v\n", user.Username, err)
			os.Exit(-1)
		}
		user.Password = "" // 清除明文密码

		// 自动迁移用户表结构
		//err = dbase.AutoMigrate(user)
		//if err != nil {
		//	fmt.Printf("Failed to auto migrate user: %v\n", err)
		//}

		// 检查用户是否已存在
		var count int64
		count, err = dbase.Count("SELECT COUNT(*) FROM user_details WHERE username = ?", user.Username)

		if err == nil && count > 0 {
			continue // 已存在，跳过创建
		}

		// 用户不存在，插入新用户
		err = dbase.Insert(user)
		if err != nil {
			fmt.Printf("Failed to insert user %s: %v\n", user.Username, err)
		}

	}

	return users
}

// createTestClients 创建测试客户端
// 创建默认测试客户端
func createTestClients(dbase db.Database) map[string]*ClientDetails {
	clients := map[string]*ClientDetails{
		"user-svc": {
			ClientId:                    "user-svc",
			ClientSecret:                "user_secret",
			AccessTokenValiditySeconds:  3600,
			RefreshTokenValiditySeconds: 7200,
			RegisteredRedirectUri:       "http://localhost:10002/callback",
			AuthorizedGrantTypes:        "password,refresh_token",
			Authorities:                 "USER",
		},
	}

	for _, client := range clients {

		// 检查客户端是否已存在
		var count int64
		count, err := dbase.Count("SELECT COUNT(*) FROM client_details WHERE client_id = ?", client.ClientId)
		if err == nil && count > 0 {
			continue // 已存在，跳过创建
		}

		// 自动迁移客户端表结构
		err = dbase.Insert(client)
		if err != nil {
			fmt.Printf("Failed to auto migrate client: %v\n", err)
		}

	}

	return clients
}
