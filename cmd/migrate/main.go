// main.go 数据库迁移工具主程序
// 主要功能：
// 1. 解析命令行参数
// 2. 初始化数据库迁移实例
// 3. 执行向上或向下迁移操作
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// main 数据库迁移工具入口函数
// 根据命令行参数执行数据库迁移操作
func main() {
	// 定义命令行参数变量
	var migrationsPath, dbURL string
	var up, down bool

	// 定义命令行参数
	// -path: 迁移文件路径，默认为"./migrations"
	// -database: 数据库连接URL，必须提供
	// -up: 执行向上迁移操作
	// -down: 执行向下迁移操作
	flag.StringVar(&migrationsPath, "path", "./migrations", "migrations path")
	flag.StringVar(&dbURL, "database", "", "database url")
	flag.BoolVar(&up, "up", false, "apply all up migrations")
	flag.BoolVar(&down, "down", false, "apply all down migrations")
	flag.Parse()

	// 检查数据库URL是否提供，如果没有则终止程序
	if dbURL == "" {
		log.Fatal("database url is required")
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