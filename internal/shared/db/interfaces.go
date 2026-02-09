// Package db 提供了数据库访问的抽象层。
// 它定义了通用的数据库操作接口，旨在解耦上层服务与底层数据库驱动 (如 GORM) 的具体实现。
package db

import (
	"context"
	"github.com/redis/go-redis/v9" // 引入 go-redis
	"gorm.io/gorm"
)

// RedisClient 定义了与 Redis 交互所需方法的最小接口集合。
// 这使得上层服务可以依赖此接口，而不是具体的 Redis 客户端实现，
// 从而极大地简化了在单元测试中对 Redis 进行模拟 (Mock) 的过程。
type RedisClient interface {
	// SetEx 设置一个带有过期时间的键值对。
	SetEx(key string, value interface{}, expireSeconds int) error
	// GetCache 获取一个键的值。
	GetCache(key string) (string, error)
	// RunScript 执行一个预加载的 Lua 脚本。
	// 它会自动处理 SCRIPT LOAD 和 EVALSHA 优化。
	RunScript(ctx context.Context, script *redis.Script, keys []string, args ...interface{}) *redis.Cmd
}

// Database 定义了数据库操作的统一接口。
// 任何具体的数据库实现 (如 EasyDatabase, PostgresDatabase) 都应实现此接口。
type Database interface {
	// AutoMigrate 自动迁移数据库表结构。
	// 它会根据传入的 GORM 模型自动创建或更新表。
	AutoMigrate(models ...interface{}) error

	// Insert 插入一条新的记录到数据库。
	Insert(ctx context.Context, value interface{}) error

	// Query 执行原生的 SQL 查询并将结果集扫描到 `dest` 中。
	Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error

	// Count 执行原生的 SQL 查询并返回匹配的记录总数。
	Count(ctx context.Context, query string, args ...interface{}) (int64, error)

	// CountByField 根据指定的字段和值统计记录数量。
	// 这是一个更抽象、更易于测试的计数方法。
	CountByField(ctx context.Context, model interface{}, field string, value interface{}) (int64, error)

	// Where 开始一个 GORM 查询链，添加 WHERE 条件。
	// 注意：直接返回 *gorm.DB 会使上层代码与 GORM 耦合，但有时为了灵活性是必要的。
	Where(ctx context.Context, query string, args ...interface{}) *gorm.DB

	// Order 开始一个 GORM 查询链，添加 ORDER BY 条件。
	Order(ctx context.Context, query string) *gorm.DB

	// Limit 开始一个 GORM 查询链，添加 LIMIT 条件。
	Limit(ctx context.Context, limit int) *gorm.DB

	// Update 更新数据库中的记录。
	Update(ctx context.Context, model interface{}, updates map[string]interface{}) error

	// Delete 删除数据库中的记录。
	Delete(ctx context.Context, model interface{}, conds ...interface{}) error

	// GetDB 返回底层的 *gorm.DB 实例。
	// 这个方法提供了访问 GORM 特定功能的能力，但应谨慎使用，以避免过度耦合。
	GetDB() *gorm.DB

	// GetType 返回当前数据库的类型 (例如, "mysql", "postgres")。
	GetType() string

	// Begin 手动开启一个新的事务。
	// 返回一个实现了 TxTransaction 接口的事务对象。
	Begin(ctx context.Context) (TxTransaction, error)

	// RunInTransaction 在一个事务中自动执行一个函数。
	// 如果函数返回错误，事务将自动回滚；否则将自动提交。
	// 这是推荐的事务使用方式。
	RunInTransaction(ctx context.Context, fn func(tx TxTransaction) error) error

	// Close 关闭数据库连接池。
	Close() error
}
