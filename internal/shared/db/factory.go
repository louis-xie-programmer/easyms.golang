// Package db 提供了数据库访问的抽象层。
package db

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing" // 引入 GORM OTel 插件
)

// DatabaseFactory 定义了创建数据库实例的工厂接口。
// 这种工厂模式允许根据不同的需求 (如数据库类型、连接池配置) 灵活地创建数据库连接。
type DatabaseFactory interface {
	// CreateDatabase 根据数据库类型和连接字符串创建一个数据库实例，使用默认连接池配置。
	CreateDatabase(dbType string, connStr string) (Database, error)
	// CreateDatabaseWithPool 根据数据库类型、连接字符串和自定义连接池配置创建一个数据库实例。
	CreateDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error)
	// CreateReadWriteSplitDatabase 创建一个支持读写分离的数据库实例。
	CreateReadWriteSplitDatabase(config ReadWriteSplitConfig) (Database, error)
}

// ReadWriteSplitConfig 读写分离配置，包含主库和从库的配置。
type ReadWriteSplitConfig struct {
	Master   DatabaseConfig   // 主库配置
	Replicas []DatabaseConfig // 从库配置列表
}

// DefaultDatabaseFactory 是 DatabaseFactory 接口的默认实现。
type DefaultDatabaseFactory struct{}

// NewDatabaseFactory 创建并返回一个 DefaultDatabaseFactory 实例。
func NewDatabaseFactory() DatabaseFactory {
	return &DefaultDatabaseFactory{}
}

// createGormDB 是一个辅助函数，用于创建和配置 GORM 的 *gorm.DB 实例。
// 它负责注册 GORM 插件，如指标插件和 OpenTelemetry 追踪插件。
func createGormDB(dialector gorm.Dialector, dbType string) (*gorm.DB, error) {
	config := &gorm.Config{
		SkipDefaultTransaction:                   true,  // 默认跳过事务，手动控制事务
		DisableForeignKeyConstraintWhenMigrating: true,  // 迁移时禁用外键约束
	}

	db, err := gorm.Open(dialector, config)
	if err != nil {
		return nil, err
	}

	// 注册自定义的指标插件，用于收集数据库操作的 Prometheus 指标
	if err := db.Use(&MetricsPlugin{DBType: dbType}); err != nil {
		return nil, err
	}

	// 注册 OpenTelemetry 插件，用于分布式追踪
	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, err
	}

	return db, nil
}

// CreateDatabase 根据数据库类型和连接字符串创建一个数据库实例。
// 它使用默认的连接池配置。
func (f *DefaultDatabaseFactory) CreateDatabase(dbType string, connStr string) (Database, error) {
	var dialector gorm.Dialector

	// 根据数据库类型选择对应的 GORM 驱动
	switch dbType {
	case "mysql":
		dialector = mysql.Open(connStr)
	case "postgres":
		dialector = postgres.Open(connStr)
	case "sqlserver":
		dialector = sqlserver.Open(connStr)
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", dbType)
	}

	// 创建 GORM 数据库连接并注册插件
	db, err := createGormDB(dialector, dbType)
	if err != nil {
		return nil, err
	}

	// 根据数据库类型返回相应的 Database 接口实现
	switch dbType {
	case "postgres":
		return NewPostgresDatabase(db), nil
	case "mysql":
		return NewMysqlDatabase(db), nil
	default:
		return &EasyDatabase{DB: db, DBType: dbType}, nil
	}
}

// CreateDatabaseWithPool 根据数据库类型、连接字符串和自定义连接池配置创建一个数据库实例。
func (f *DefaultDatabaseFactory) CreateDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error) {
	var dialector gorm.Dialector

	// 根据数据库类型选择对应的 GORM 驱动
	switch dbType {
	case "mysql":
		dialector = mysql.Open(connStr)
	case "postgres":
		dialector = postgres.Open(connStr)
	case "sqlserver":
		dialector = sqlserver.Open(connStr)
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", dbType)
	}

	// 创建 GORM 数据库连接并注册插件
	db, err := createGormDB(dialector, dbType)
	if err != nil {
		return nil, err
	}

	// 配置连接池
	if cfg != nil {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}

		// 根据不同的配置类型设置连接池参数
		switch v := cfg.(type) {
		case map[string]interface{}:
			if maxIdleConns, ok := v["max_idle_conns"].(int); ok && maxIdleConns > 0 {
				sqlDB.SetMaxIdleConns(maxIdleConns)
			}
			if maxOpenConns, ok := v["max_open_conns"].(int); ok && maxOpenConns > 0 {
				sqlDB.SetMaxOpenConns(maxOpenConns)
			}
			if connMaxLifetime, ok := v["conn_max_lifetime"].(int); ok && connMaxLifetime > 0 {
				sqlDB.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)
			}
			if connMaxIdleTime, ok := v["conn_max_idle_time"].(int); ok && connMaxIdleTime > 0 {
				sqlDB.SetConnMaxIdleTime(time.Duration(connMaxIdleTime) * time.Second)
			}
		case *DatabaseConfig: // 如果传入的是 DatabaseConfig 结构体指针
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

	// 根据数据库类型返回相应的 Database 接口实现
	switch dbType {
	case "postgres":
		return NewPostgresDatabase(db), nil
	case "mysql":
		return NewMysqlDatabase(db), nil
	default:
		return &EasyDatabase{DB: db, DBType: dbType}, nil
	}
}

// CreateReadWriteSplitDatabase 创建一个支持读写分离的数据库实例。
// 它会初始化一个主库连接和多个从库连接。
func (f *DefaultDatabaseFactory) CreateReadWriteSplitDatabase(config ReadWriteSplitConfig) (Database, error) {
	// 创建主库连接
	master, err := f.createSingleDatabase(config.Master)
	if err != nil {
		return nil, fmt.Errorf("创建主数据库失败: %w", err)
	}

	// 创建从库连接
	var replicas []*gorm.DB
	for i, replicaConfig := range config.Replicas {
		replica, err := f.createSingleDatabase(replicaConfig)
		if err != nil {
			return nil, fmt.Errorf("创建从数据库 #%d 失败: %w", i, err)
		}
		replicas = append(replicas, replica.GetDB())
	}

	return NewReadWriteSplitDatabase(master.GetDB(), replicas), nil
}

// createSingleDatabase 是一个内部辅助函数，用于根据 DatabaseConfig 创建单个数据库连接。
func (f *DefaultDatabaseFactory) createSingleDatabase(config DatabaseConfig) (Database, error) {
	var dialector gorm.Dialector
	// 构建数据库连接字符串
	connStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.UserName, config.Password, config.Host, config.Port, config.Database)

	// 根据数据库类型选择对应的 GORM 驱动
	switch config.Type {
	case "mysql":
		dialector = mysql.Open(connStr)
	case "postgres":
		dialector = postgres.Open(connStr)
	case "sqlserver":
		dialector = sqlserver.Open(connStr)
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", config.Type)
	}

	// 创建 GORM 数据库连接并注册插件
	gormDB, err := createGormDB(dialector, config.Type)
	if err != nil {
		return nil, err
	}

	// 配置连接池
	if config.MaxIdleConns > 0 || config.MaxOpenConns > 0 || config.ConnMaxLifetime > 0 || config.ConnMaxIdleTime > 0 {
		sqlDB, err := gormDB.DB()
		if err != nil {
			return nil, err
		}

		if config.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(config.MaxIdleConns)
		}
		if config.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(config.MaxOpenConns)
		}
		if config.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(time.Duration(config.ConnMaxLifetime) * time.Second)
		}
		if config.ConnMaxIdleTime > 0 {
			sqlDB.SetConnMaxIdleTime(time.Duration(config.ConnMaxIdleTime) * time.Second)
		}
	}

	// 根据数据库类型返回相应的 Database 接口实现
	switch config.Type {
	case "postgres":
		return NewPostgresDatabase(gormDB), nil
	case "mysql":
		return NewMysqlDatabase(gormDB), nil
	default:
		return &EasyDatabase{DB: gormDB, DBType: config.Type}, nil
	}
}
