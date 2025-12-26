package db

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// PostgreSQL数组类型映射表
// 定义了PostgreSQL数组类型与Go类型的映射关系，用于自动处理数组类型字段
var (
	// postgresArrayTypeMap 定义了PostgreSQL数组类型与Go类型的映射关系
	postgresArrayTypeMap = map[string]reflect.Type{
		"text[]":             reflect.TypeOf(PgStringArray{}),
		"varchar[]":          reflect.TypeOf(PgStringArray{}),
		"integer[]":          reflect.TypeOf(PgIntArray{}),
		"bigint[]":           reflect.TypeOf(PgIntArray{}),
		"real[]":             reflect.TypeOf(PgFloatArray{}),
		"double precision[]": reflect.TypeOf(PgFloatArray{}),
	}

	// typeCheckCache 用于缓存类型检查结果，提高性能
	// 使用sync.Map实现线程安全的缓存机制
	typeCheckCache = sync.Map{}
)

// ArrayTypeInfo 存储数组类型信息
type ArrayTypeInfo struct {
	GoType     reflect.Type
	PgTypeName string
	IsValid    bool
}

// getArrayTypeInfo 获取字段的数组类型信息
// 使用结构化键提高缓存效率
func getArrayTypeInfo(field reflect.StructField) *ArrayTypeInfo {
	cacheKey := field.Type.String() + ":" + field.Tag.Get("gorm")

	// 检查缓存
	if cached, ok := typeCheckCache.Load(cacheKey); ok {
		return cached.(*ArrayTypeInfo)
	}

	gormTag := strings.ToLower(field.Tag.Get("gorm"))
	typeInfo := &ArrayTypeInfo{IsValid: false}

	// 查找匹配的PostgreSQL数组类型
	for pgType, goType := range postgresArrayTypeMap {
		if strings.Contains(gormTag, pgType) {
			typeInfo.PgTypeName = pgType
			typeInfo.GoType = goType
			typeInfo.IsValid = (field.Type == goType) ||
				(field.Type.Kind() == reflect.Ptr && field.Type.Elem() == goType)
			// 一旦找到匹配项就立即返回，避免不必要的遍历
			return typeInfo
		}
	}

	// 存储到缓存
	typeCheckCache.Store(cacheKey, typeInfo)
	return typeInfo
}

// validateArrayField 验证数组字段并返回可能的警告信息
func validateArrayField(field reflect.StructField) string {
	typeInfo := getArrayTypeInfo(field)

	if !typeInfo.IsValid && typeInfo.GoType != nil {
		// 类型不匹配，返回警告
		return fmt.Sprintf("Field %s is marked as %s but is not %s",
			field.Name, typeInfo.PgTypeName, typeInfo.GoType.Name())
	}

	return "" // 有效或非数组字段，无警告
}

// convertSliceValue 将Go切片值转换为对应的PostgreSQL数组类型值
func convertSliceValue(value reflect.Value, gormTag string) (reflect.Value, error) {
	gormTagLower := strings.ToLower(gormTag)

	// 通过一次遍历来检查所有条件，减少字符串操作次数
	isArrayType := func(types ...string) bool {
		for _, t := range types {
			if strings.Contains(gormTagLower, t) {
				return true
			}
		}
		return false
	}

	isTextArray := isArrayType("text[]", "varchar[]")
	isIntArray := isArrayType("integer[]", "bigint[]")
	isFloatArray := isArrayType("real[]", "double precision[]")

	switch value.Type().Elem().Kind() {
	case reflect.String:
		if isTextArray {
			array := make(PgStringArray, value.Len())
			for j := 0; j < value.Len(); j++ {
				array[j] = value.Index(j).String()
			}
			return reflect.ValueOf(array), nil
		}
	case reflect.Int, reflect.Int64:
		if isIntArray {
			array := make(PgIntArray, value.Len())
			for j := 0; j < value.Len(); j++ {
				array[j] = value.Index(j).Int()
			}
			return reflect.ValueOf(array), nil
		}
	case reflect.Float32, reflect.Float64:
		if isFloatArray {
			array := make(PgFloatArray, value.Len())
			for j := 0; j < value.Len(); j++ {
				array[j] = value.Index(j).Float()
			}
			return reflect.ValueOf(array), nil
		}
	}

	return reflect.Value{}, fmt.Errorf("unsupported slice type conversion for gorm tag: %s", gormTag)
}

// processStructFields 处理结构体字段的通用函数
func processStructFields(value interface{}, processFunc func(reflect.StructField, reflect.Value) error) error {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		if err := processFunc(field, fv); err != nil {
			return err
		}
	}
	return nil
}

// PostgresDatabase PostgreSQL数据库实现
type PostgresDatabase struct {
	*EasyDatabase
}

// NewPostgresDatabase 创建PostgreSQL数据库实例
// 确保PostgresDatabase实现了Database接口
var _ Database = &PostgresDatabase{}

func NewPostgresDatabase(db *gorm.DB) *PostgresDatabase {
	return &PostgresDatabase{
		EasyDatabase: &EasyDatabase{DB: db, DBType: "postgres"},
	}
}

// AutoMigrate 自动迁移数据库表结构（PostgreSQL特有实现）
// 处理包含数组类型的字段并执行自动迁移
func (pd *PostgresDatabase) AutoMigrate(models ...interface{}) error {
	// 对于PostgreSQL，我们需要特别处理包含数组的字段
	for _, model := range models {
		err := processStructFields(model, func(field reflect.StructField, _ reflect.Value) error {
			if warning := validateArrayField(field); warning != "" {
				fmt.Printf("Warning: %s\n", warning)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return pd.DB.AutoMigrate(models...)
}

// convertArrays 自动将结构体中的切片转换为对应的Pg数组类型
// 处理PostgreSQL数组类型的特殊需求
func (pd *PostgresDatabase) convertArrays(value interface{}) interface{} {
	processStructFields(value, func(field reflect.StructField, fv reflect.Value) error {
		// 处理切片类型字段
		if fv.Kind() == reflect.Slice {
			if convertedValue, err := convertSliceValue(fv, field.Tag.Get("gorm")); err == nil {
				fv.Set(convertedValue)
			}
		}
		return nil
	})
	return value
}

// Insert 插入数据（PostgreSQL特有实现）
// 处理数组类型字段并插入数据
func (pd *PostgresDatabase) Insert(ctx context.Context, value interface{}) error {
	// 先转换数组字段
	v := pd.convertArrays(value)

	// 执行插入操作
	return pd.DB.WithContext(ctx).Create(v).Error
}

// Query 查询数据（PostgreSQL特有实现）
// 使用原生SQL查询并将结果扫描到目标结构体中
// 处理数组类型字段的初始化
func (pd *PostgresDatabase) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	// 检查dest是否为指向结构体的指针
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() == reflect.Ptr && !destValue.IsNil() {
		elem := destValue.Elem()
		if elem.Kind() == reflect.Struct {
			// 处理结构体字段，初始化数组类型字段
			processStructFields(dest, func(field reflect.StructField, fv reflect.Value) error {
				typeInfo := getArrayTypeInfo(field)
				if !typeInfo.IsValid {
					return nil // 非数组字段，跳过
				}

				// 创建对应类型的实例
				newValue := reflect.New(typeInfo.GoType)
				if fv.Kind() == reflect.Ptr {
					fv.Set(newValue)
				} else {
					fv.Set(newValue.Elem())
				}
				return nil
			})
		}
	}

	return pd.DB.WithContext(ctx).Raw(query, args...).Scan(dest).Error
}

// Begin 开启事务
func (pd *PostgresDatabase) Begin(ctx context.Context) (TxTransaction, error) {
	tx := pd.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &GormTransaction{DB: tx}, nil
}

// RunInTransaction 在事务中执行操作
func (pd *PostgresDatabase) RunInTransaction(ctx context.Context, fn func(tx TxTransaction) error) error {
	return pd.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&GormTransaction{DB: tx})
	})
}

// GetDB 获取底层的GORM数据库实例
func (pd *PostgresDatabase) GetDB() *gorm.DB {
	return pd.DB
}

// GetType 获取数据库类型
func (pd *PostgresDatabase) GetType() string {
	return pd.DBType
}
