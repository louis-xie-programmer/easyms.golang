package db

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

// PostgresDatabase PostgreSQL数据库实现
type PostgresDatabase struct {
	*EasyDatabase
}

// NewPostgresDatabase 创建PostgreSQL数据库实例
// 确保PostgresDatabase实现了DatabaseInterface接口
var _ Database = &PostgresDatabase{}

func NewPostgresDatabase(db *gorm.DB) *PostgresDatabase {
	return &PostgresDatabase{
		EasyDatabase: &EasyDatabase{DB: db, DBType: "postgres"},
	}
}

// AutoMigrate 自动迁移数据库表结构（PostgreSQL特有实现）
func (pd *PostgresDatabase) AutoMigrate(models ...interface{}) error {
	// 对于PostgreSQL，我们需要特别处理包含字符串数组的字段
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

			// 检查是否有text[]或varchar[]类型的字段
			if strings.Contains(strings.ToLower(gormTag), "text[]") ||
				strings.Contains(strings.ToLower(gormTag), "varchar[]") {
				// 如果是PgStringArray类型，我们不需要特殊处理
				if field.Type == reflect.TypeOf(PgStringArray{}) ||
					(field.Type.Kind() == reflect.Ptr && field.Type.Elem() == reflect.TypeOf(PgStringArray{})) {
					continue
				}

				// 如果不是PgStringArray类型但标记为数组，则打印警告
				fmt.Printf("Warning: Field %s is marked as array type but is not PgStringArray\n", field.Name)
			}
		}
	}

	return pd.DB.AutoMigrate(models...)
}

// convertArrays 自动将结构体中的[]string转换为PgStringArray
// 处理PostgreSQL数组类型的特殊需求
func (pd *PostgresDatabase) convertArrays(value interface{}) interface{} {
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
				if fv.Len() >= 0 {
					array := make(PgStringArray, fv.Len())
					for j := 0; j < fv.Len(); j++ {
						array[j] = fv.Index(j).String()
					}
					fv.Set(reflect.ValueOf(array))
				}
			}
		}

		// 检查字段是否为*PgStringArray类型
		if fv.Kind() == reflect.Ptr && fv.Type().Elem() == reflect.TypeOf(PgStringArray{}) {
			if !fv.IsNil() {
				pgArray := fv.Interface().(*PgStringArray)
				fv.Set(reflect.ValueOf(pgArray))
			}
		}
	}
	return value
}

// Insert 插入数据（PostgreSQL特有实现）
func (pd *PostgresDatabase) Insert(value interface{}) error {
	// 先转换数组字段
	v := pd.convertArrays(value)

	// 使用 Session 创建一个新会话
	session := pd.DB.Session(&gorm.Session{})

	// 执行插入操作
	return session.Create(v).Error
}

// Query 查询数据（PostgreSQL特有实现）
// 使用原生SQL查询并将结果扫描到目标结构体中
func (pd *PostgresDatabase) Query(dest interface{}, query string, args ...interface{}) error {
	// 检查dest是否为指向结构体的指针
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() == reflect.Ptr && !destValue.IsNil() {
		elem := destValue.Elem()
		if elem.Kind() == reflect.Struct {
			// 遍历结构体字段，查找需要特殊处理的数组字段
			t := elem.Type()
			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				fv := elem.Field(i)

				// 检查字段是否为*PgStringArray类型
				if fv.Kind() == reflect.Ptr && fv.Type().Elem() == reflect.TypeOf(PgStringArray{}) {
					gormTag := field.Tag.Get("gorm")
					// 检查是否为PostgreSQL数组类型
					if strings.Contains(strings.ToLower(gormTag), "text[]") ||
						strings.Contains(strings.ToLower(gormTag), "varchar[]") {
						// 创建PgStringArray实例并赋值给字段
						pgArray := &PgStringArray{}
						fv.Set(reflect.ValueOf(pgArray))
					}
				}

				// 检查字段是否为PgStringArray类型
				if fv.Type() == reflect.TypeOf(PgStringArray{}) {
					gormTag := field.Tag.Get("gorm")
					// 检查是否为PostgreSQL数组类型
					if strings.Contains(strings.ToLower(gormTag), "text[]") ||
						strings.Contains(strings.ToLower(gormTag), "varchar[]") {
						// 创建PgStringArray实例并赋值给字段
						pgArray := &PgStringArray{}
						fv.Set(reflect.ValueOf(pgArray))
					}
				}
			}
		}
	}

	return pd.DB.Raw(query, args...).Scan(dest).Error
}

// GetDB 获取底层的GORM数据库实例
func (pd *PostgresDatabase) GetDB() *gorm.DB {
	return pd.DB
}

// GetType 获取数据库类型
func (pd *PostgresDatabase) GetType() string {
	return pd.DBType
}
