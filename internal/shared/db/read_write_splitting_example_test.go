package db

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID    int64  `gorm:"primaryKey" json:"id"`
	Name  string `gorm:"type:varchar(100);not null" json:"name"`
	Email string `gorm:"type:varchar(100);uniqueIndex" json:"email"`
}

// ExampleReadWriteSplitDatabase 演示读写分离数据库的使用
func ExampleReadWriteSplitDatabase() {
	// 注意：这只是示例代码，实际使用时需要连接真实的数据库

	// 模拟主库和从库连接
	var masterDB *gorm.DB
	var replicaDB1 *gorm.DB
	var replicaDB2 *gorm.DB

	// 创建读写分离数据库实例
	rwDB := NewReadWriteSplitDatabase(masterDB, []*gorm.DB{replicaDB1, replicaDB2})

	// 插入数据（写操作，会在主库执行）
	user := User{
		Name:  "张三",
		Email: "zhangsan@example.com",
	}

	if err := rwDB.Insert(&user); err != nil {
		log.Fatalf("插入用户失败: %v", err)
	}

	// 查询数据（读操作，会在从库执行）
	var resultUser User
	if err := rwDB.Query(&resultUser, "SELECT * FROM users WHERE id = ?", user.ID); err != nil {
		log.Fatalf("查询用户失败: %v", err)
	}

	// 使用指定负载均衡策略查询数据
	if err := rwDB.QueryWithStrategy(RandomLoadBalance, &resultUser, "SELECT * FROM users WHERE name = ?", "张三"); err != nil {
		log.Fatalf("查询用户失败: %v", err)
	}

	// 更新数据（写操作，会在主库执行）
	updates := map[string]interface{}{
		"name": "李四",
	}

	if err := rwDB.Update(&User{ID: user.ID}, updates); err != nil {
		log.Fatalf("更新用户失败: %v", err)
	}

	// 删除数据（写操作，会在主库执行）
	if err := rwDB.Delete(&User{ID: user.ID}); err != nil {
		log.Fatalf("删除用户失败: %v", err)
	}

	fmt.Println("读写分离数据库操作演示完成")

	// Output:
	// 读写分离数据库操作演示完成
}

// ExampleReadWriteSplitConfig 演示读写分离配置
func ExampleReadWriteSplitConfig() {
	// 创建读写分离配置
	config := ReadWriteSplitConfig{
		Master: DatabaseConfig{
			Type:            "mysql",
			Host:            "master.db.example.com",
			Port:            3306,
			UserName:        "root",
			Password:        "password",
			Database:        "myapp",
			MaxIdleConns:    10,
			MaxOpenConns:    100,
			ConnMaxLifetime: 3600,
			ConnMaxIdleTime: 1800,
		},
		Replicas: []DatabaseConfig{
			{
				Type:            "mysql",
				Host:            "replica1.db.example.com",
				Port:            3306,
				UserName:        "reader",
				Password:        "password",
				Database:        "myapp",
				MaxIdleConns:    5,
				MaxOpenConns:    50,
				ConnMaxLifetime: 3600,
				ConnMaxIdleTime: 1800,
			},
			{
				Type:            "mysql",
				Host:            "replica2.db.example.com",
				Port:            3306,
				UserName:        "reader",
				Password:        "password",
				Database:        "myapp",
				MaxIdleConns:    5,
				MaxOpenConns:    50,
				ConnMaxLifetime: 3600,
				ConnMaxIdleTime: 1800,
			},
		},
	}

	fmt.Printf("主库地址: %s:%d\n", config.Master.Host, config.Master.Port)
	fmt.Printf("从库数量: %d\n", len(config.Replicas))

	// Output:
	// 主库地址: master.db.example.com:3306
	// 从库数量: 2
}

func main() {
	// 这些示例仅用于演示目的
	log.Println("读写分离数据库功能示例")
}
