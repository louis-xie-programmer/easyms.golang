// Package storage 提供了用户服务的数据存储层。
// 它封装了所有与用户数据持久化相关的数据库操作。
package storage

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
)

// UserStorage 定义了用户数据持久化的接口。
// 这种抽象使得上层的服务逻辑与底层的数据库实现解耦。
type UserStorage interface {
	// Create 在数据库中插入一条新的用户记录。
	Create(ctx context.Context, user *models.User) error
	// GetByID 根据用户 ID 从数据库中检索一条用户记录。
	GetByID(ctx context.Context, id uint) (*models.User, error)
	// RunInTransaction 在一个数据库事务中执行一个函数。
	// 它将一个事务性的 UserStorage 实例传递给该函数，
	// 确保函数内的所有操作都在同一个事务中完成。
	RunInTransaction(ctx context.Context, fn func(txStorage UserStorage) error) error
}

// userStorageImpl 是 UserStorage 接口的具体实现，使用 db.Database 接口进行数据库操作。
type userStorageImpl struct {
	db db.Database
}

// NewUserStorage 创建一个新的 UserStorage 实例。
// db: 一个实现了 db.Database 接口的实例，用于依赖注入。
func NewUserStorage(db db.Database) UserStorage {
	return &userStorageImpl{
		db: db,
	}
}

// Create 实现了在数据库中插入新用户的逻辑。
func (s *userStorageImpl) Create(ctx context.Context, user *models.User) error {
	return s.db.Insert(ctx, user)
}

// GetByID 实现了根据 ID 从数据库中检索用户的逻辑。
func (s *userStorageImpl) GetByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	err := s.db.GetDB().WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// RunInTransaction 实现了开启和管理数据库事务的逻辑。
func (s *userStorageImpl) RunInTransaction(ctx context.Context, fn func(txStorage UserStorage) error) error {
	// 使用底层的 db.Database 接口来执行事务
	return s.db.RunInTransaction(ctx, func(tx db.TxTransaction) error {
		// 将事务对象包装成一个事务性的 UserStorage 实例
		return fn(&txUserStorageImpl{tx: tx})
	})
}

// txUserStorageImpl 是一个用于事务操作的 UserStorage 实现。
// 它持有的是一个 db.TxTransaction 事务对象，而不是一个完整的 db.Database 连接。
type txUserStorageImpl struct {
	tx db.TxTransaction
}

// Create 在事务中插入新用户。
func (s *txUserStorageImpl) Create(ctx context.Context, user *models.User) error {
	return s.tx.Insert(ctx, user)
}

// GetByID 在事务中根据 ID 检索用户。
func (s *txUserStorageImpl) GetByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	// 使用事务对象的 GetDB() 方法来执行查询
	err := s.tx.GetDB().WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// RunInTransaction 允许在已有的事务中"嵌套"逻辑。
// 它直接执行函数，因为事务已经开启。
func (s *txUserStorageImpl) RunInTransaction(ctx context.Context, fn func(txStorage UserStorage) error) error {
	return fn(s)
}
