// database.go 数据库访问模块
// 主要功能：
// 1. 支持多种数据库类型（MySQL、PostgreSQL、SQL Server）
// 2. 提供统一的数据库访问接口
// 3. 实现数据库连接池管理
// 4. 提供常用的数据库操作方法
package db

import (
	"gorm.io/gorm"
)

// DatabaseConfig 定义数据库配置
type DatabaseConfig struct {
	Type     string `yaml:"type"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	UserName string `yaml:"username"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	// 连接池配置
	MaxIdleConns    int `yaml:"max_idle_conns"`     // 最大空闲连接数
	MaxOpenConns    int `yaml:"max_open_conns"`     // 最大打开连接数
	ConnMaxLifetime int `yaml:"conn_max_lifetime"`  // 连接最大生命周期(秒)
	ConnMaxIdleTime int `yaml:"conn_max_idle_time"` // 连接最大空闲时间(秒)
}

// EasyDatabase 数据库实例结构体
// 实现DatabaseInterface接口
type EasyDatabase struct {
	DB     *gorm.DB // GORM数据库实例
	DBType string   // 数据库类型
}

// 确保EasyDatabase实现了DatabaseInterface接口
var _ Database = &EasyDatabase{}

// AutoMigrate 自动迁移数据库表结构
func (ed *EasyDatabase) AutoMigrate(models ...interface{}) error {
	return ed.DB.AutoMigrate(models...)
}

// Insert 插入数据
func (ed *EasyDatabase) Insert(value interface{}) error {
	// 使用 Session 创建一个新会话
	session := ed.DB.Session(&gorm.Session{})

	// 执行插入操作
	return session.Create(value).Error
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

// GetDB 获取底层的GORM数据库实例
func (ed *EasyDatabase) GetDB() *gorm.DB {
	return ed.DB
}

// GetType 获取数据库类型
func (ed *EasyDatabase) GetType() string {
	return ed.DBType
}

// NewEasyDatabase 创建新的数据库实例
// 根据数据库类型创建相应的数据库连接
// 参数:
//   - dbType: 数据库类型（mysql/postgres/sqlserver）
//   - connStr: 数据库连接字符串
//
// 返回值:
//   - Database: 数据库实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewEasyDatabase(dbType string, connStr string) (Database, error) {
	factory := NewDatabaseFactory()
	return factory.CreateDatabase(dbType, connStr)
}

// NewEasyDatabaseWithPool 创建带连接池配置的数据库实例
// 根据数据库类型创建相应的数据库连接，并配置连接池参数
// 参数:
//   - dbType: 数据库类型（mysql/postgres/sqlserver）
//   - connStr: 数据库连接字符串
//   - cfg: 连接池配置
//
// 返回值:
//   - Database: 数据库实例
//   - error: 操作成功返回nil，失败返回具体错误
func NewEasyDatabaseWithPool(dbType string, connStr string, cfg interface{}) (Database, error) {
	factory := NewDatabaseFactory()
	return factory.CreateDatabaseWithPool(dbType, connStr, cfg)
}
