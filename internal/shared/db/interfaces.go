package db

import (
	"gorm.io/gorm"
)

// Database 数据库接口定义
// 定义了数据库操作的基本方法
type Database interface {
	// AutoMigrate 自动迁移数据库表结构
	AutoMigrate(models ...interface{}) error

	// Insert 插入数据
	Insert(value interface{}) error

	// Query 查询数据
	Query(dest interface{}, query string, args ...interface{}) error

	// Count 统计记录数
	Count(query string, args ...interface{}) (int64, error)

	// Where 添加WHERE条件
	Where(query string, args ...interface{}) *gorm.DB

	// Order 添加排序条件
	Order(query string) *gorm.DB

	// Limit 添加LIMIT限制
	Limit(limit int) *gorm.DB

	// Update 更新数据
	Update(model interface{}, updates map[string]interface{}) error

	// Delete 删除数据
	Delete(model interface{}, conds ...interface{}) error

	// GetDB 获取底层的GORM数据库实例
	GetDB() *gorm.DB

	// GetType 获取数据库类型
	GetType() string

	// Begin 开启事务
	Begin() (TxTransaction, error)

	// RunInTransaction 在事务中执行操作
	RunInTransaction(fn func(tx TxTransaction) error) error
}
