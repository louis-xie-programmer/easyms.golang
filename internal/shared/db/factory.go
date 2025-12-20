package db

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

// DatabaseFactory 数据库工厂接口
type DatabaseFactory interface {
	CreateDatabase(dbType string, connStr string) (Database, error)
	CreateDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error)
}

// DefaultDatabaseFactory 默认数据库工厂实现
type DefaultDatabaseFactory struct{}

// NewDatabaseFactory 创建数据库工厂实例
func NewDatabaseFactory() DatabaseFactory {
	return &DefaultDatabaseFactory{}
}

// CreateDatabase 创建新的数据库实例
// 根据数据库类型创建相应的数据库连接
// 参数:
//   - dbType: 数据库类型（mysql/postgres/sqlserver）
//   - connStr: 数据库连接字符串
//
// 返回值:
//   - Database: 数据库实例
//   - error: 操作成功返回nil，失败返回具体错误
func (f *DefaultDatabaseFactory) CreateDatabase(dbType string, connStr string) (Database, error) {
	var dialector gorm.Dialector

	// 根据数据库类型选择对应的驱动
	switch dbType {
	case "mysql":
		dialector = mysql.Open(connStr)
	case "postgres":
		dialector = postgres.Open(connStr)
	case "sqlserver":
		dialector = sqlserver.Open(connStr)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	// 为 PostgreSQL 配置更好的数组支持
	config := &gorm.Config{
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	// 创建数据库连接
	db, err := gorm.Open(dialector, config)
	if err != nil {
		return nil, err
	}

	// 根据数据库类型返回相应的实现
	switch dbType {
	case "postgres":
		return NewPostgresDatabase(db), nil
	default:
		return &EasyDatabase{DB: db, DBType: dbType}, nil
	}
}

// CreateDatabaseWithPool 创建带连接池配置的数据库实例
// 根据数据库类型创建相应的数据库连接，并配置连接池参数
// 参数:
//   - dbType: 数据库类型（mysql/postgres/sqlserver）
//   - connStr: 数据库连接字符串
//   - cfg: 连接池配置
//
// 返回值:
//   - Database: 数据库实例
//   - error: 操作成功返回nil，失败返回具体错误
func (f *DefaultDatabaseFactory) CreateDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error) {
	var dialector gorm.Dialector

	// 根据数据库类型选择对应的驱动
	switch dbType {
	case "mysql":
		dialector = mysql.Open(connStr)
	case "postgres":
		dialector = postgres.Open(connStr)
	case "sqlserver":
		dialector = sqlserver.Open(connStr)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	// 为 PostgreSQL 配置更好的数组支持
	config := &gorm.Config{
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	// 创建数据库连接
	db, err := gorm.Open(dialector, config)
	if err != nil {
		return nil, err
	}

	// 配置连接池
	// 设置连接池相关参数以优化数据库性能
	if cfg != nil {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}

		// 根据不同的配置类型设置连接池参数
		switch v := cfg.(type) {
		case map[string]interface{}:
			// 设置最大空闲连接数
			// 控制连接池中空闲连接的最大数量
			if maxIdleConns, ok := v["max_idle_conns"].(int); ok && maxIdleConns > 0 {
				sqlDB.SetMaxIdleConns(maxIdleConns)
			}

			// 设置最大打开连接数
			// 控制数据库连接的最大数量
			if maxOpenConns, ok := v["max_open_conns"].(int); ok && maxOpenConns > 0 {
				sqlDB.SetMaxOpenConns(maxOpenConns)
			}

			// 设置连接最大生命周期
			// 控制连接可以被复用的最大时间
			if connMaxLifetime, ok := v["conn_max_lifetime"].(int); ok && connMaxLifetime > 0 {
				sqlDB.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)
			}

			// 设置连接最大空闲时间
			// 控制连接在池中保持空闲的最大时间
			if connMaxIdleTime, ok := v["conn_max_idle_time"].(int); ok && connMaxIdleTime > 0 {
				sqlDB.SetConnMaxIdleTime(time.Duration(connMaxIdleTime) * time.Second)
			}
		case *DatabaseConfig:
			// 如果cfg是DatabaseConfig结构体类型，直接使用其字段
			if v.MaxIdleConns > 0 {
				sqlDB.SetMaxIdleConns(v.MaxIdleConns)
			}
			if v.MaxOpenConns > 0 {
				sqlDB.SetMaxOpenConns(v.MaxOpenConns)
			}
			if v.ConnMaxLifetime > 0 {
				sqlDB.SetConnMaxLifetime(time.Duration(v.ConnMaxLifetime) * time.Second)
			}
			if v.ConnMaxIdleTime > 0 {
				sqlDB.SetConnMaxIdleTime(time.Duration(v.ConnMaxIdleTime) * time.Second)
			}
		}
	}

	// 根据数据库类型返回相应的实现
	switch dbType {
	case "postgres":
		return NewPostgresDatabase(db), nil
	default:
		return &EasyDatabase{DB: db, DBType: dbType}, nil
	}
}
