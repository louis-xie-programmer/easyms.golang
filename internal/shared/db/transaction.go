// Package db 提供了数据库访问的抽象层。
package db

import (
	"context"
	"gorm.io/gorm"
)

// TxTransaction 定义了数据库事务操作的接口。
// 这个接口的方法是 Database 接口的一个子集，确保在事务中可以执行相同的基本操作。
type TxTransaction interface {
	// Insert 在事务中插入一条新记录。
	Insert(ctx context.Context, value interface{}) error

	// Update 在事务中更新记录。
	Update(ctx context.Context, model interface{}, updates map[string]interface{}) error

	// Delete 在事务中删除记录。
	Delete(ctx context.Context, model interface{}, conds ...interface{}) error

	// Query 在事务中执行原生 SQL 查询。
	Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error

	// Count 在事务中执行原生 SQL 计数查询。
	Count(ctx context.Context, query string, args ...interface{}) (int64, error)

	// Where 在事务中开始一个 GORM 查询链，添加 WHERE 条件。
	Where(ctx context.Context, query string, args ...interface{}) *gorm.DB

	// Order 在事务中开始一个 GORM 查询链，添加 ORDER BY 条件。
	Order(ctx context.Context, query string) *gorm.DB

	// Limit 在事务中开始一个 GORM 查询链，添加 LIMIT 条件。
	Limit(ctx context.Context, limit int) *gorm.DB

	// Commit 手动提交事务。
	// 注意：当使用 `RunInTransaction` 时，不需要手动调用此方法。
	Commit() error

	// Rollback 手动回滚事务。
	// 注意：当使用 `RunInTransaction` 时，不需要手动调用此方法。
	Rollback() error

	// GetDB 返回事务使用的底层 *gorm.DB 实例。
	GetDB() *gorm.DB
}

// GormTransaction 是 TxTransaction 接口基于 GORM 的具体实现。
// 它包装了一个 *gorm.DB 事务对象。
type GormTransaction struct {
	DB *gorm.DB
}

// Insert 在事务中插入数据。
func (tx *GormTransaction) Insert(ctx context.Context, value interface{}) error {
	return tx.DB.WithContext(ctx).Create(value).Error
}

// Update 在事务中更新数据。
func (tx *GormTransaction) Update(ctx context.Context, model interface{}, updates map[string]interface{}) error {
	return tx.DB.WithContext(ctx).Model(model).Updates(updates).Error
}

// Delete 在事务中删除数据。
func (tx *GormTransaction) Delete(ctx context.Context, model interface{}, conds ...interface{}) error {
	return tx.DB.WithContext(ctx).Delete(model, conds...).Error
}

// Query 在事务中执行查询。
func (tx *GormTransaction) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return tx.DB.WithContext(ctx).Raw(query, args...).Scan(dest).Error
}

// Count 在事务中执行计数。
func (tx *GormTransaction) Count(ctx context.Context, query string, args ...interface{}) (int64, error) {
	var count int64
	err := tx.DB.WithContext(ctx).Raw(query, args...).Count(&count).Error
	return count, err
}

// Where 在事务中添加 WHERE 条件。
func (tx *GormTransaction) Where(ctx context.Context, query string, args ...interface{}) *gorm.DB {
	return tx.DB.WithContext(ctx).Where(query, args...)
}

// Order 在事务中添加 ORDER BY 条件。
func (tx *GormTransaction) Order(ctx context.Context, query string) *gorm.DB {
	return tx.DB.WithContext(ctx).Order(query)
}

// Limit 在事务中添加 LIMIT 条件。
func (tx *GormTransaction) Limit(ctx context.Context, limit int) *gorm.DB {
	return tx.DB.WithContext(ctx).Limit(limit)
}

// Commit 提交 GORM 事务。
func (tx *GormTransaction) Commit() error {
	return tx.DB.Commit().Error
}

// Rollback 回滚 GORM 事务。
func (tx *GormTransaction) Rollback() error {
	return tx.DB.Rollback().Error
}

// GetDB 返回底层的 GORM 事务数据库实例。
func (tx *GormTransaction) GetDB() *gorm.DB {
	return tx.DB
}
