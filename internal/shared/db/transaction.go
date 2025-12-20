package db

import (
	"gorm.io/gorm"
)

// TxTransaction 事务接口
// 定义了事务操作的基本方法
type TxTransaction interface {
	// Insert 插入数据
	Insert(value interface{}) error

	// Update 更新数据
	Update(model interface{}, updates map[string]interface{}) error

	// Delete 删除数据
	Delete(model interface{}, conds ...interface{}) error

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

	// Commit 提交事务
	Commit() error

	// Rollback 回滚事务
	Rollback() error

	// GetDB 获取底层的GORM数据库实例
	GetDB() *gorm.DB
}

// GormTransaction GORM事务实现
type GormTransaction struct {
	DB *gorm.DB
}

// Insert 插入数据
func (tx *GormTransaction) Insert(value interface{}) error {
	return tx.DB.Create(value).Error
}

// Update 更新数据
func (tx *GormTransaction) Update(model interface{}, updates map[string]interface{}) error {
	return tx.DB.Model(model).Updates(updates).Error
}

// Delete 删除数据
func (tx *GormTransaction) Delete(model interface{}, conds ...interface{}) error {
	return tx.DB.Delete(model, conds...).Error
}

// Query 查询数据
func (tx *GormTransaction) Query(dest interface{}, query string, args ...interface{}) error {
	return tx.DB.Raw(query, args...).Scan(dest).Error
}

// Count 统计记录数
func (tx *GormTransaction) Count(query string, args ...interface{}) (int64, error) {
	var count int64
	err := tx.DB.Raw(query, args...).Count(&count).Error
	return count, err
}

// Where 添加WHERE条件
func (tx *GormTransaction) Where(query string, args ...interface{}) *gorm.DB {
	return tx.DB.Where(query, args...)
}

// Order 添加排序条件
func (tx *GormTransaction) Order(query string) *gorm.DB {
	return tx.DB.Order(query)
}

// Limit 添加LIMIT限制
func (tx *GormTransaction) Limit(limit int) *gorm.DB {
	return tx.DB.Limit(limit)
}

// Commit 提交事务
func (tx *GormTransaction) Commit() error {
	return tx.DB.Commit().Error
}

// Rollback 回滚事务
func (tx *GormTransaction) Rollback() error {
	return tx.DB.Rollback().Error
}

// GetDB 获取底层的GORM数据库实例
func (tx *GormTransaction) GetDB() *gorm.DB {
	return tx.DB
}
