package db

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// ReadWriteSplitDatabase 支持读写分离的数据库实现
type ReadWriteSplitDatabase struct {
	master      *gorm.DB   // 主数据库（写操作）
	replicas    []*gorm.DB // 从数据库列表（读操作）
	nextReplica uint32     // 下一个从库索引（用于轮询）
}

// NewReadWriteSplitDatabase 创建支持读写分离的数据库实例
func NewReadWriteSplitDatabase(master *gorm.DB, replicas []*gorm.DB) *ReadWriteSplitDatabase {
	// 初始化随机种子
	rand.Seed(time.Now().UnixNano())

	return &ReadWriteSplitDatabase{
		master:      master,
		replicas:    replicas,
		nextReplica: 0,
	}
}

// getNextReplica 获取下一个从库连接（轮询方式）
func (rw *ReadWriteSplitDatabase) getNextReplica() *gorm.DB {
	if len(rw.replicas) == 0 {
		// 如果没有配置从库，则使用主库
		return rw.master
	}

	// 使用原子操作确保线程安全的轮询
	next := atomic.AddUint32(&rw.nextReplica, 1)
	index := (int(next) - 1) % len(rw.replicas)
	return rw.replicas[index]
}

// getRandomReplica 获取随机从库连接
func (rw *ReadWriteSplitDatabase) getRandomReplica() *gorm.DB {
	if len(rw.replicas) == 0 {
		// 如果没有配置从库，则使用主库
		return rw.master
	}

	// 随机选择一个从库
	index := rand.Intn(len(rw.replicas))
	return rw.replicas[index]
}

// getReadDB 获取用于读操作的数据库连接
// 支持多种负载均衡策略
func (rw *ReadWriteSplitDatabase) getReadDB(strategy LoadBalanceStrategy) *gorm.DB {
	switch strategy {
	case RandomLoadBalance:
		return rw.getRandomReplica()
	case RoundRobinLoadBalance:
		return rw.getNextReplica()
	default:
		return rw.getNextReplica()
	}
}

// getWriteDB 获取用于写操作的数据库连接
func (rw *ReadWriteSplitDatabase) getWriteDB() *gorm.DB {
	return rw.master
}

// LoadBalanceStrategy 负载均衡策略
type LoadBalanceStrategy int

const (
	// RoundRobinLoadBalance 轮询负载均衡
	RoundRobinLoadBalance LoadBalanceStrategy = iota

	// RandomLoadBalance 随机负载均衡
	RandomLoadBalance
)

// AutoMigrate 自动迁移数据库表结构（写操作）
func (rw *ReadWriteSplitDatabase) AutoMigrate(models ...interface{}) error {
	return rw.getWriteDB().AutoMigrate(models...)
}

// Insert 插入数据（写操作）
func (rw *ReadWriteSplitDatabase) Insert(value interface{}) error {
	session := rw.getWriteDB().Session(&gorm.Session{})
	return session.Create(value).Error
}

// Query 查询数据（读操作）
// 使用原生SQL查询并将结果扫描到目标结构体中
func (rw *ReadWriteSplitDatabase) Query(dest interface{}, query string, args ...interface{}) error {
	// 默认使用轮询负载均衡策略
	return rw.getReadDB(RoundRobinLoadBalance).Raw(query, args...).Scan(dest).Error
}

// QueryWithStrategy 使用指定负载均衡策略查询数据（读操作）
func (rw *ReadWriteSplitDatabase) QueryWithStrategy(strategy LoadBalanceStrategy, dest interface{}, query string, args ...interface{}) error {
	return rw.getReadDB(strategy).Raw(query, args...).Scan(dest).Error
}

// Count 统计记录数（读操作）
func (rw *ReadWriteSplitDatabase) Count(query string, args ...interface{}) (int64, error) {
	var count int64
	// 默认使用轮询负载均衡策略
	err := rw.getReadDB(RoundRobinLoadBalance).Raw(query, args...).Count(&count).Error
	return count, err
}

// CountWithStrategy 使用指定负载均衡策略统计记录数（读操作）
func (rw *ReadWriteSplitDatabase) CountWithStrategy(strategy LoadBalanceStrategy, query string, args ...interface{}) (int64, error) {
	var count int64
	err := rw.getReadDB(strategy).Raw(query, args...).Count(&count).Error
	return count, err
}

// Where 添加WHERE条件（读操作优先使用从库，但也可以用于写操作）
func (rw *ReadWriteSplitDatabase) Where(query string, args ...interface{}) *gorm.DB {
	// WHERE操作既可以用于读也可以用于写，这里默认使用从库
	return rw.getReadDB(RoundRobinLoadBalance).Where(query, args...)
}

// WhereForWrite 添加WHERE条件（用于写操作）
func (rw *ReadWriteSplitDatabase) WhereForWrite(query string, args ...interface{}) *gorm.DB {
	return rw.getWriteDB().Where(query, args...)
}

// Order 添加排序条件（读操作）
func (rw *ReadWriteSplitDatabase) Order(query string) *gorm.DB {
	return rw.getReadDB(RoundRobinLoadBalance).Order(query)
}

// OrderForWrite 添加排序条件（用于写操作）
func (rw *ReadWriteSplitDatabase) OrderForWrite(query string) *gorm.DB {
	return rw.getWriteDB().Order(query)
}

// Limit 添加LIMIT限制（读操作）
func (rw *ReadWriteSplitDatabase) Limit(limit int) *gorm.DB {
	return rw.getReadDB(RoundRobinLoadBalance).Limit(limit)
}

// LimitForWrite 添加LIMIT限制（用于写操作）
func (rw *ReadWriteSplitDatabase) LimitForWrite(limit int) *gorm.DB {
	return rw.getWriteDB().Limit(limit)
}

// Update 更新数据（写操作）
func (rw *ReadWriteSplitDatabase) Update(model interface{}, updates map[string]interface{}) error {
	return rw.getWriteDB().Model(model).Updates(updates).Error
}

// Delete 删除数据（写操作）
func (rw *ReadWriteSplitDatabase) Delete(model interface{}, conds ...interface{}) error {
	return rw.getWriteDB().Delete(model, conds...).Error
}

// GetDB 获取底层的GORM数据库实例（默认返回主库）
func (rw *ReadWriteSplitDatabase) GetDB() *gorm.DB {
	return rw.getWriteDB()
}

// GetReadDB 获取用于读操作的数据库实例
func (rw *ReadWriteSplitDatabase) GetReadDB() *gorm.DB {
	return rw.getReadDB(RoundRobinLoadBalance)
}

// GetWriteDB 获取用于写操作的数据库实例
func (rw *ReadWriteSplitDatabase) GetWriteDB() *gorm.DB {
	return rw.getWriteDB()
}

// GetType 获取数据库类型
func (rw *ReadWriteSplitDatabase) GetType() string {
	return rw.getWriteDB().Name()
}

// Begin 开启事务（事务必须在主库上执行）
func (rw *ReadWriteSplitDatabase) Begin() (TxTransaction, error) {
	tx := rw.getWriteDB().Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &GormTransaction{DB: tx}, nil
}

// RunInTransaction 在事务中执行操作（事务必须在主库上执行）
func (rw *ReadWriteSplitDatabase) RunInTransaction(fn func(tx TxTransaction) error) error {
	return rw.getWriteDB().Transaction(func(tx *gorm.DB) error {
		return fn(&GormTransaction{DB: tx})
	})
}

// WithContext 在指定上下文中执行数据库操作
func (rw *ReadWriteSplitDatabase) WithContext(ctx context.Context) *ReadWriteSplitDatabase {
	// 为所有数据库连接设置上下文
	newReplicas := make([]*gorm.DB, len(rw.replicas))
	for i, replica := range rw.replicas {
		newReplicas[i] = replica.WithContext(ctx)
	}

	return &ReadWriteSplitDatabase{
		master:      rw.master.WithContext(ctx),
		replicas:    newReplicas,
		nextReplica: rw.nextReplica,
	}
}
