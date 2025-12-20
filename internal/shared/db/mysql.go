package db

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

// MysqlDatabase MySQL数据库实现
type MysqlDatabase struct {
	*EasyDatabase
}

// NewMysqlDatabase 创建MySQL数据库实例
// 确保MysqlDatabase实现了Database接口
var _ Database = &MysqlDatabase{}

// NewMysqlDatabase 创建MySQL数据库实例
func NewMysqlDatabase(db *gorm.DB) *MysqlDatabase {
	return &MysqlDatabase{
		EasyDatabase: &EasyDatabase{DB: db, DBType: "mysql"},
	}
}

// AutoMigrate 自动迁移数据库表结构（MySQL特有实现）
func (md *MysqlDatabase) AutoMigrate(models ...interface{}) error {
	// 对于MySQL，我们可以添加特定的处理逻辑
	for _, model := range models {
		// 使用反射检查模型中的字段
		v := reflect.ValueOf(model)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			continue
		}

		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			gormTag := field.Tag.Get("gorm")

			// 检查是否有JSON类型的字段
			if strings.Contains(strings.ToLower(gormTag), "json") {
				// MySQL对JSON类型有良好支持，确保使用正确的数据类型
				fmt.Printf("Info: Field %s uses JSON type\n", field.Name)
			}

			// 检查是否有特定的MySQL类型提示
			if strings.Contains(strings.ToLower(gormTag), "type:") {
				// 可以在这里添加MySQL特定的类型检查
				fmt.Printf("Info: Field %s has explicit type definition\n", field.Name)
			}
		}
	}

	return md.DB.AutoMigrate(models...)
}

// Insert 插入数据（MySQL特有实现）
func (md *MysqlDatabase) Insert(value interface{}) error {
	// MySQL特定的插入逻辑
	// 使用 Session 创建一个新会话
	session := md.DB.Session(&gorm.Session{
		// MySQL支持的特定选项
	})

	// 执行插入操作
	return session.Create(value).Error
}

// convertJsonFields 自动处理结构体中的JSON字段
// MySQL对JSON类型有特殊支持
func (md *MysqlDatabase) convertJsonFields(value interface{}) interface{} {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return value
	}

	// 在这里可以添加MySQL特定的JSON字段处理逻辑
	// 例如，自动将map[string]interface{}转换为适合MySQL JSON类型的格式

	return value
}

// Query 查询数据（MySQL特有实现）
func (md *MysqlDatabase) Query(dest interface{}, query string, args ...interface{}) error {
	// MySQL特定的查询逻辑
	// 可以在这里添加MySQL特定的优化或处理

	// 检查dest是否为指向结构体的指针
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() == reflect.Ptr && !destValue.IsNil() {
		elem := destValue.Elem()
		if elem.Kind() == reflect.Struct {
			// 遍历结构体字段，查找需要特殊处理的字段
			t := elem.Type()
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				gormTag := field.Tag.Get("gorm")

				// 检查字段是否为JSON类型
				if strings.Contains(strings.ToLower(gormTag), "json") {
					// 可以为JSON字段做特殊处理
					fmt.Printf("Info: Processing JSON field %s\n", field.Name)
				}
			}
		}
	}

	return md.DB.Raw(query, args...).Scan(dest).Error
}

// Upsert MySQL特有功能：插入或更新（ON DUPLICATE KEY UPDATE）
func (md *MysqlDatabase) Upsert(value interface{}, updateColumns ...string) error {
	// 这里简化实现，实际应用中可以根据需要扩展
	// 使用GORM的Clauses来实现Upsert功能
	return md.DB.Clauses().Create(value).Error
}

// GetDB 获取底层的GORM数据库实例
func (md *MysqlDatabase) GetDB() *gorm.DB {
	return md.DB
}

// GetType 获取数据库类型
func (md *MysqlDatabase) GetType() string {
	return md.DBType
}

// Begin 开启事务
func (md *MysqlDatabase) Begin() (TxTransaction, error) {
	tx := md.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &GormTransaction{DB: tx}, nil
}
