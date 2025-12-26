package db

import (
	"context"
	"gorm.io/gorm"
)

// TxTransaction 事务接口
// 定义了事务操作的基本方法
type TxTransaction interface {
	// Insert 插入数据
	Insert(ctx context.Context, value interface{}) error

	// Update 更新数据
	Update(ctx context.Context, model interface{}, updates map[string]interface{}) error

	// Delete 删除数据
	Delete(ctx context.Context, model interface{}, conds ...interface{}) error

	// Query 查询数据
	Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error

	// Count 统计记录数
	Count(ctx context.Context, query string, args ...interface{}) (int64, error)

	// Where 添加WHERE条件
	Where(ctx context.Context, query string, args ...interface{}) *gorm.DB

	// Order 添加排序条件
	Order(ctx context.Context, query string) *gorm.DB

	// Limit 添加LIMIT限制
	Limit(ctx context.Context, limit int) *gorm.DB

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
func (tx *GormTransaction) Insert(ctx context.Context, value interface{}) error {
	return tx.DB.WithContext(ctx).Create(value).Error
}

// Update 更新数据
func (tx *GormTransaction) Update(ctx context.Context, model interface{}, updates map[string]interface{}) error {
	return tx.DB.WithContext(ctx).Model(model).Updates(updates).Error
}

// Delete 删除数据
func (tx *GormTransaction) Delete(ctx context.Context, model interface{}, conds ...interface{}) error {
	return tx.DB.WithContext(ctx).Delete(model, conds...).Error
}

// Query 查询数据
func (tx *GormTransaction) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return tx.DB.WithContext(ctx).Raw(query, args...).Scan(dest).Error
}

// Count 统计记录数
func (tx *GormTransaction) Count(ctx context.Context, query string, args ...interface{}) (int64, error) {
	var count int64
	err := tx.DB.WithContext(ctx).Raw(query, args...).Count(&count).Error
	return count, err
}

// Where 添加WHERE条件
func (tx *GormTransaction) Where(ctx context.Context, query string, args ...interface{}) *gorm.DB {
	return tx.DB.WithContext(ctx).Where(query, args...)
}

// Order 添加排序条件
func (tx *GormTransaction) Order(ctx context.Context, query string) *gorm.DB {
	return tx.DB.WithContext(ctx).Order(query)
}

// Limit 添加LIMIT限制
func (tx *GormTransaction) Limit(ctx context.Context, limit int) *gorm.DB {
	return tx.DB.WithContext(ctx).Limit(limit)
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
