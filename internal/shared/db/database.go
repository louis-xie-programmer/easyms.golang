// Package db 提供了数据库访问的抽象层。
package db

import (
	"context"
	"fmt"
	"gorm.io/gorm"
)

// DatabaseConfig 定义了数据库连接的配置参数。
type DatabaseConfig struct {
	Type     string `yaml:"type"`     // 数据库类型 (例如 "mysql", "postgres")
	Host     string `yaml:"host"`     // 数据库主机地址
	Port     int    `yaml:"port"`     // 数据库端口
	UserName string `yaml:"username"` // 数据库用户名
	Password string `yaml:"password"` // 数据库密码
	Database string `yaml:"database"` // 数据库名称
	// 连接池配置
	MaxIdleConns    int `yaml:"max_idle_conns"`     // 连接池最大空闲连接数
	MaxOpenConns    int `yaml:"max_open_conns"`     // 连接池最大打开连接数
	ConnMaxLifetime int `yaml:"conn_max_lifetime"`  // 连接最大生命周期 (秒)
	ConnMaxIdleTime int `yaml:"conn_max_idle_time"` // 连接最大空闲时间 (秒)
}

// EasyDatabase 是 Database 接口的通用实现，它包装了 GORM 数据库实例。
type EasyDatabase struct {
	DB     *gorm.DB // GORM 数据库实例
	DBType string   // 数据库类型 (例如 "mysql", "postgres")
}

// 确保 EasyDatabase 实现了 Database 接口。
var _ Database = &EasyDatabase{}

// AutoMigrate 自动迁移数据库表结构。
func (ed *EasyDatabase) AutoMigrate(models ...interface{}) error {
	return ed.DB.AutoMigrate(models...)
}

// Insert 插入一条新的记录到数据库。
func (ed *EasyDatabase) Insert(ctx context.Context, value interface{}) error {
	return ed.DB.WithContext(ctx).Create(value).Error
}

// Query 执行原生的 SQL 查询并将结果集扫描到 `dest` 中。
func (ed *EasyDatabase) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return ed.DB.WithContext(ctx).Raw(query, args...).Scan(dest).Error
}

// Count 执行原生的 SQL 查询并返回匹配的记录总数。
func (ed *EasyDatabase) Count(ctx context.Context, query string, args ...interface{}) (int64, error) {
	var count int64
	err := ed.DB.WithContext(ctx).Raw(query, args...).Count(&count).Error
	return count, err
}

// CountByField 根据指定的模型、字段和值统计记录数量。
func (ed *EasyDatabase) CountByField(ctx context.Context, model interface{}, field string, value interface{}) (int64, error) {
	var count int64
	query := fmt.Sprintf("%s = ?", field)
	err := ed.DB.WithContext(ctx).Model(model).Where(query, value).Count(&count).Error
	return count, err
}

// Where 开始一个 GORM 查询链，添加 WHERE 条件。
func (ed *EasyDatabase) Where(ctx context.Context, query string, args ...interface{}) *gorm.DB {
	return ed.DB.WithContext(ctx).Where(query, args...)
}

// Order 开始一个 GORM 查询链，添加 ORDER BY 条件。
func (ed *EasyDatabase) Order(ctx context.Context, query string) *gorm.DB {
	return ed.DB.WithContext(ctx).Order(query)
}

// Limit 开始一个 GORM 查询链，添加 LIMIT 条件。
func (ed *EasyDatabase) Limit(ctx context.Context, limit int) *gorm.DB {
	return ed.DB.WithContext(ctx).Limit(limit)
}

// Update 更新数据库中的记录。
func (ed *EasyDatabase) Update(ctx context.Context, model interface{}, updates map[string]interface{}) error {
	return ed.DB.WithContext(ctx).Model(model).Updates(updates).Error
}

// Delete 删除数据库中的记录。
func (ed *EasyDatabase) Delete(ctx context.Context, model interface{}, conds ...interface{}) error {
	return ed.DB.WithContext(ctx).Delete(model, conds...).Error
}

// GetDB 返回底层的 *gorm.DB 实例。
func (ed *EasyDatabase) GetDB() *gorm.DB {
	return ed.DB
}

// GetType 返回当前数据库的类型。
func (ed *EasyDatabase) GetType() string {
	return ed.DBType
}

// Begin 手动开启一个新的事务。
func (ed *EasyDatabase) Begin(ctx context.Context) (TxTransaction, error) {
	tx := ed.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &GormTransaction{DB: tx}, nil
}

// RunInTransaction 在一个事务中自动执行一个函数。
func (ed *EasyDatabase) RunInTransaction(ctx context.Context, fn func(tx TxTransaction) error) error {
	return ed.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormTransaction{DB: tx})
	})
}

// Close 关闭数据库连接池。
func (ed *EasyDatabase) Close() error {
	sqlDB, err := ed.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// NewEasyDatabase 创建一个新的数据库实例，不带连接池配置。
func NewEasyDatabase(dbType string, connStr string) (Database, error) {
	factory := NewDatabaseFactory()
	return factory.CreateDatabase(dbType, connStr)
}

// NewEasyDatabaseWithPool 创建一个新的数据库实例，并应用连接池配置。
func NewEasyDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error) {
	factory := NewDatabaseFactory()
	return factory.CreateDatabaseWithPool(dbType, connStr, cfg)
}
