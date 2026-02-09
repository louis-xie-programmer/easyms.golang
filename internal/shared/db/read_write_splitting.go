// Package db 提供了数据库访问的抽象层。
package db

import (
	"context"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// ReadWriteSplitDatabase 实现了 Database 接口，提供了数据库读写分离的功能。
// 它内部管理一个主数据库连接 (用于写操作) 和多个从数据库连接 (用于读操作)。
type ReadWriteSplitDatabase struct {
	master      *gorm.DB   // 主数据库连接 (写)
	replicas    []*gorm.DB // 从数据库连接列表 (读)
	nextReplica uint32     // 用于轮询负载均衡的原子计数器
}

// 确保 ReadWriteSplitDatabase 实现了 Database 接口。
var _ Database = &ReadWriteSplitDatabase{}

// NewReadWriteSplitDatabase 创建一个新的读写分离数据库实例。
func NewReadWriteSplitDatabase(master *gorm.DB, replicas []*gorm.DB) *ReadWriteSplitDatabase {
	// 初始化随机种子，用于随机负载均衡策略
	rand.Seed(time.Now().UnixNano())

	return &ReadWriteSplitDatabase{
		master:      master,
		replicas:    replicas,
		nextReplica: 0,
	}
}

// getNextReplica 使用轮询 (Round-Robin) 策略获取一个从库连接。
func (rw *ReadWriteSplitDatabase) getNextReplica() *gorm.DB {
	if len(rw.replicas) == 0 {
		return rw.master // 如果没有配置从库，则回退到主库进行读操作
	}
	// 使用原子操作确保在并发场景下轮询的线程安全
	next := atomic.AddUint32(&rw.nextReplica, 1)
	index := (int(next) - 1) % len(rw.replicas)
	return rw.replicas[index]
}

// getRandomReplica 使用随机策略获取一个从库连接。
func (rw *ReadWriteSplitDatabase) getRandomReplica() *gorm.DB {
	if len(rw.replicas) == 0 {
		return rw.master // 回退到主库
	}
	index := rand.Intn(len(rw.replicas))
	return rw.replicas[index]
}

// getReadDB 根据指定的负载均衡策略获取一个用于读操作的数据库连接。
func (rw *ReadWriteSplitDatabase) getReadDB(strategy LoadBalanceStrategy) *gorm.DB {
	switch strategy {
	case RandomLoadBalance:
		return rw.getRandomReplica()
	case RoundRobinLoadBalance:
		return rw.getNextReplica()
	default:
		return rw.getNextReplica() // 默认使用轮询策略
	}
}

// getWriteDB 获取用于写操作的数据库连接 (总是主库)。
func (rw *ReadWriteSplitDatabase) getWriteDB() *gorm.DB {
	return rw.master
}

// LoadBalanceStrategy 定义了读操作的负载均衡策略类型。
type LoadBalanceStrategy int

const (
	RoundRobinLoadBalance LoadBalanceStrategy = iota // 轮询负载均衡
	RandomLoadBalance                                // 随机负载均衡
)

// AutoMigrate 在主库上执行自动迁移。
func (rw *ReadWriteSplitDatabase) AutoMigrate(models ...interface{}) error {
	return rw.getWriteDB().AutoMigrate(models...)
}

// Insert 在主库上执行插入操作。
func (rw *ReadWriteSplitDatabase) Insert(ctx context.Context, value interface{}) error {
	return rw.getWriteDB().WithContext(ctx).Create(value).Error
}

// Query 在从库上执行原生 SQL 查询。
func (rw *ReadWriteSplitDatabase) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return rw.getReadDB(RoundRobinLoadBalance).WithContext(ctx).Raw(query, args...).Scan(dest).Error
}

// Count 在从库上执行原生 SQL 计数查询。
func (rw *ReadWriteSplitDatabase) Count(ctx context.Context, query string, args ...interface{}) (int64, error) {
	var count int64
	err := rw.getReadDB(RoundRobinLoadBalance).WithContext(ctx).Raw(query, args...).Count(&count).Error
	return count, err
}

// CountByField 在从库上根据指定字段统计记录数。
func (rw *ReadWriteSplitDatabase) CountByField(ctx context.Context, model interface{}, field string, value interface{}) (int64, error) {
	var count int64
	query := fmt.Sprintf("%s = ?", field)
	err := rw.getReadDB(RoundRobinLoadBalance).WithContext(ctx).Model(model).Where(query, value).Count(&count).Error
	return count, err
}

// Where 在从库上开始一个 GORM 查询链。
func (rw *ReadWriteSplitDatabase) Where(ctx context.Context, query string, args ...interface{}) *gorm.DB {
	return rw.getReadDB(RoundRobinLoadBalance).WithContext(ctx).Where(query, args...)
}

// Order 在从库上添加 ORDER BY 条件。
func (rw *ReadWriteSplitDatabase) Order(ctx context.Context, query string) *gorm.DB {
	return rw.getReadDB(RoundRobinLoadBalance).WithContext(ctx).Order(query)
}

// Limit 在从库上添加 LIMIT 条件。
func (rw *ReadWriteSplitDatabase) Limit(ctx context.Context, limit int) *gorm.DB {
	return rw.getReadDB(RoundRobinLoadBalance).WithContext(ctx).Limit(limit)
}

// Update 在主库上执行更新操作。
func (rw *ReadWriteSplitDatabase) Update(ctx context.Context, model interface{}, updates map[string]interface{}) error {
	return rw.getWriteDB().WithContext(ctx).Model(model).Updates(updates).Error
}

// Delete 在主库上执行删除操作。
func (rw *ReadWriteSplitDatabase) Delete(ctx context.Context, model interface{}, conds ...interface{}) error {
	return rw.getWriteDB().WithContext(ctx).Delete(model, conds...).Error
}

// GetDB 返回主库的 *gorm.DB 实例。
func (rw *ReadWriteSplitDatabase) GetDB() *gorm.DB {
	return rw.getWriteDB()
}

// GetType 返回数据库类型 (以主库为准)。
func (rw *ReadWriteSplitDatabase) GetType() string {
	// GORM v2 中没有 Name() 方法，需要通过 Driver() 获取
	return rw.getWriteDB().Dialector.Name()
}

// Begin 在主库上开启一个新事务。
func (rw *ReadWriteSplitDatabase) Begin(ctx context.Context) (TxTransaction, error) {
	tx := rw.getWriteDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &GormTransaction{DB: tx}, nil
}

// RunInTransaction 在主库上执行一个事务。
func (rw *ReadWriteSplitDatabase) RunInTransaction(ctx context.Context, fn func(tx TxTransaction) error) error {
	return rw.getWriteDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormTransaction{DB: tx})
	})
}

// Close 实现了 Database 接口的 Close 方法。
// 它会尝试关闭主库和所有从库的连接。
func (rw *ReadWriteSplitDatabase) Close() error {
	var firstErr error

	// 关闭主库
	if masterDB, err := rw.master.DB(); err == nil {
		if err := masterDB.Close(); err != nil {
			firstErr = fmt.Errorf("关闭主数据库失败: %w", err)
		}
	}

	// 关闭所有从库
	for i, replica := range rw.replicas {
		if replicaDB, err := replica.DB(); err == nil {
			if err := replicaDB.Close(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("关闭从数据库 #%d 失败: %w", i, err)
			}
		}
	}

	return firstErr
}
