package db

import (
	"database/sql/driver"
	"strconv"

	"github.com/lib/pq"
)

// PgStringArray PostgreSQL字符串数组类型
// 用于处理PostgreSQL的文本数组类型
type PgStringArray []string

// Value 实现driver.Valuer接口
func (a PgStringArray) Value() (driver.Value, error) {
	return pq.Array(a).Value()
}

// Scan 实现sql.Scanner接口
func (a *PgStringArray) Scan(src interface{}) error {
	return scanPgArray(src, a, parseString)
}

// PgIntArray PostgreSQL整数数组类型
type PgIntArray []int64

// Value 实现driver.Valuer接口
func (a PgIntArray) Value() (driver.Value, error) {
	return pq.Array(a).Value()
}

// Scan 实现sql.Scanner接口
func (a *PgIntArray) Scan(src interface{}) error {
	return scanPgArray(src, a, parseInt)
}

// PgFloatArray PostgreSQL浮点数组类型
type PgFloatArray []float64

// Value 实现driver.Valuer接口
func (a PgFloatArray) Value() (driver.Value, error) {
	return pq.Array(a).Value()
}

// Scan 实现sql.Scanner接口
func (a *PgFloatArray) Scan(src interface{}) error {
	return scanPgArray(src, a, parseFloat)
}

// PgGenericArray 通用数组接口
type PgGenericArray interface {
	SetValues(values []string) error
}

// Scan 通用扫描方法
func scanPgArray(src interface{}, target PgGenericArray, parser func(string) (interface{}, error)) error {
	if src == nil {
		return target.SetValues(nil)
	}

	// 使用pq.Array进行标准解析
	var temp []string
	if err := pq.Array(&temp).Scan(src); err != nil {
		return err
	}

	// 解析每个元素
	values := make([]string, len(temp))
	for i, v := range temp {
		if parsed, err := parser(v); err == nil {
			values[i] = formatValue(parsed)
		} else {
			values[i] = v
		}
	}

	// 设置值到目标数组
	return target.SetValues(values)
}

// 解析函数类型
type parserFunc func(string) (interface{}, error)

// 字符串解析函数
func parseString(s string) (interface{}, error) {
	return s, nil
}

// 整数解析函数
func parseInt(s string) (interface{}, error) {
	return strconv.ParseInt(s, 10, 64)
}

// 浮点数解析函数
func parseFloat(s string) (interface{}, error) {
	return strconv.ParseFloat(s, 64)
}

// 格式化值为字符串
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	default:
		return ""
	}
}

// SetValues 为PgStringArray设置值
func (a *PgStringArray) SetValues(values []string) error {
	*a = PgStringArray(values)
	return nil
}

// SetValues 为PgIntArray设置值
func (a *PgIntArray) SetValues(values []string) error {
	result := make(PgIntArray, len(values))
	for i, v := range values {
		if val, err := strconv.ParseInt(v, 10, 64); err == nil {
			result[i] = val
		} else {
			return err
		}
	}
	*a = result
	return nil
}

// SetValues 为PgFloatArray设置值
func (a *PgFloatArray) SetValues(values []string) error {
	result := make(PgFloatArray, len(values))
	for i, v := range values {
		if val, err := strconv.ParseFloat(v, 64); err == nil {
			result[i] = val
		} else {
			return err
		}
	}
	*a = result
	return nil
}
