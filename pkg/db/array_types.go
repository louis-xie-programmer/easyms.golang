package db

import (
	"database/sql/driver"
	"strings"

	"github.com/lib/pq"
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
	if src == nil {
		*a = nil
		return nil
	}
	
	// 如果src是字符串类型，我们尝试解析它
	if str, ok := src.(string); ok {
		// 解析PostgreSQL数组格式 "{item1,item2,...}"
		if len(str) >= 2 && str[0] == '{' && str[len(str)-1] == '}' {
			content := str[1 : len(str)-1]
			if content == "" {
				*a = PgStringArray{}
				return nil
			}
			
			// 简单分割，实际PostgreSQL数组解析应该更复杂
			items := strings.Split(content, ",")
			result := make(PgStringArray, len(items))
			for i, item := range items {
				// 移除可能的引号
				if len(item) >= 2 && item[0] == '"' && item[len(item)-1] == '"' {
					item = item[1 : len(item)-1]
				}
				result[i] = item
			}
			*a = result
			return nil
		}
	}
	
	// 使用pq.Array进行标准解析
	return pq.Array(a).Scan(src)
}