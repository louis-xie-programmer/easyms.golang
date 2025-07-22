// main.go 程序入口点
// 包含服务启动和初始化逻辑
package main

import (
	"os"
)

func main() {
	// 获取环境变量
	env := os.Getenv("ENV")
	if env == "" {
		env = "dev"
	}

}
