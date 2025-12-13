// database.go 数据库访问模块
// 主要功能：
// 1. 支持多种数据库类型（MySQL、PostgreSQL、SQL Server）
// 2. 提供统一的数据库访问接口
// 3. 实现数据库连接池管理
// 4. 支持PostgreSQL数组类型处理
// 5. 提供常用的数据库操作方法
package db

import (
	"database/sql/driver"
	"fmt"
	"time"
	"github.com/lib/pq"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"reflect"
	"strings"
)

// PgStringArray PostgreSQL字符串数组类型
// 用于处理PostgreSQL的文本数组类型
// 实现了driver.Valuer和sql.Scanner接口以支持数据库读写
type PgStringArray []string

// Value 实现driver.Valuer接口
// 将Go数组转换为PostgreSQL数组格式
func (a PgStringArray) Value() (driver.Value, error) {
	return pq.Array(a).Value()
}

// Scan 实现sql.Scanner接口
// 将PostgreSQL数组转换为Go数组
func (a *PgStringArray) Scan(src interface{}) error {
	return pq.Array(a).Scan(src)
}

// Database 数据库访问接口
// 定义了数据库操作的标准方法
// 提供统一的数据库访问接口，屏蔽不同数据库之间的差异
type Database interface {
	AutoMigrate(models ...interface{}) error     // 自动迁移数据库表结构
	Insert(value interface{}) error              // 插入数据
	Query(dest interface{}, query string, args ...interface{}) error  // 查询数据
	Count(query string, args ...interface{}) (int64, error)          // 统计记录数
	Where(query string, args ...interface{}) *gorm.DB                // 添加WHERE条件
	Order(query string) *gorm.DB                                     // 添加排序条件
	Limit(limit int) *gorm.DB                                        // 添加LIMIT限制
	Update(model interface{}, updates map[string]interface{}) error   // 更新数据
	Delete(model interface{}, conds ...interface{}) error            // 删除数据
}

// NewEasyDatabase 创建新的数据库实例
// 根据数据库类型创建相应的数据库连接
// 参数:
//   - dbType: 数据库类型（mysql/postgres/sqlserver）
//   - connStr: 数据库连接字符串
// 返回值:
//   - Database: 数据库实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewEasyDatabase(dbType string, connStr string) (Database, error) {
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
	config := &gorm.Config{}
	if dbType == "postgres" {
		config.SkipDefaultTransaction = true
	}

	// 创建数据库连接
	db, err := gorm.Open(dialector, config)
	if err != nil {
		return nil, err
	}

	return &EasyDatabase{DB: db, DBType: dbType}, nil
}

// NewEasyDatabaseWithPool 创建带连接池配置的数据库实例
// 根据数据库类型创建相应的数据库连接，并配置连接池参数
// 参数:
//   - dbType: 数据库类型（mysql/postgres/sqlserver）
//   - connStr: 数据库连接字符串
//   - cfg: 连接池配置
// 返回值:
//   - Database: 数据库实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewEasyDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error) {
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
	config := &gorm.Config{}
	if dbType == "postgres" {
		config.SkipDefaultTransaction = true
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
		}
	}

	return &EasyDatabase{DB: db, DBType: dbType}, nil
}

// EasyDatabase 数据库实例结构体
type EasyDatabase struct {
	DB     *gorm.DB  // GORM数据库实例
	DBType string    // 数据库类型
}

// AutoMigrate 自动迁移数据库表结构
func (ed *EasyDatabase) AutoMigrate(models ...interface{}) error {
	return ed.DB.AutoMigrate(models...)
}

// convertArrays 自动将结构体中的[]string转换为PgStringArray
// 仅对PostgreSQL数据库生效
// 处理PostgreSQL数组类型的特殊需求
func (ed *EasyDatabase) convertArrays(value interface{}) interface{} {
	if ed.DBType != "postgres" {
		return value
	}

	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return value
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		// 检查字段是否为字符串切片类型
		if fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
			gormTag := field.Tag.Get("gorm")
			// 检查是否为PostgreSQL数组类型
			// 通过检查gorm标签判断是否为text[]或varchar[]类型
			if strings.Contains(strings.ToLower(gormTag), "text[]") ||
				strings.Contains(strings.ToLower(gormTag), "varchar[]") {
				// 转换为PgStringArray类型
				// 将Go的[]string转换为PostgreSQL兼容的PgStringArray
				array := PgStringArray{}
				for j := 0; j < fv.Len(); j++ {
					array = append(array, fv.Index(j).String())
				}
				fv.Set(reflect.ValueOf(array))
			}
		}
	}
	return value
}

// containsArrayTag 检查标签是否包含数组类型标识
func containsArrayTag(tag string) bool {
	tag = strings.ToLower(tag)
	return strings.Contains(tag, "text[]") || strings.Contains(tag, "varchar[]")
}

// Insert 插入数据
func (ed *EasyDatabase) Insert(value interface{}) error {
	v := ed.convertArrays(value)
	return ed.DB.Create(v).Error
}

// Query 查询数据（可传 model + 条件）
// 使用原生SQL查询并将结果扫描到目标结构体中
func (ed *EasyDatabase) Query(dest interface{}, query string, args ...interface{}) error {
	return ed.DB.Raw(query, args...).Scan(dest).Error
}

// Count 统计记录数
func (ed *EasyDatabase) Count(query string, args ...interface{}) (int64, error) {
	var count int64
	err := ed.DB.Raw(query, args...).Count(&count).Error
	return count, err
}

// Where 添加WHERE条件
func (ed *EasyDatabase) Where(query string, args ...interface{}) *gorm.DB {
	return ed.DB.Where(query, args...)
}

// Order 添加排序条件
func (ed *EasyDatabase) Order(query string) *gorm.DB {
	return ed.DB.Order(query)
}

// Limit 添加LIMIT限制
func (ed *EasyDatabase) Limit(limit int) *gorm.DB {
	return ed.DB.Limit(limit)
}

// Update 更新数据
func (ed *EasyDatabase) Update(model interface{}, updates map[string]interface{}) error {
	return ed.DB.Model(model).Updates(updates).Error
}

// Delete 删除数据
// 根据条件删除指定模型的数据
func (ed *EasyDatabase) Delete(model interface{}, conds ...interface{}) error {
	return ed.DB.Delete(model, conds...).Error
}