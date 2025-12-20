package db

import (
	"fmt"
	"log"
)

// 示例结构体，演示如何使用各种Pg数组类型
type Product struct {
	ID          int64         `gorm:"primaryKey"`
	Name        string        `gorm:"type:varchar(100)"`
	Tags        PgStringArray `gorm:"type:text[]"`    // 字符串数组
	Categories  PgIntArray    `gorm:"type:integer[]"` // 整数数组
	Ratings     PgFloatArray  `gorm:"type:real[]"`    // 浮点数组
	Description string        `gorm:"type:text"`
}

func ExamplePgStringArray() {
	// 创建一个包含标签的示例产品
	product := Product{
		Name:        "智能手表",
		Tags:        PgStringArray{"electronics", "wearable", "smart"},
		Categories:  PgIntArray{1, 5, 12},
		Ratings:     PgFloatArray{4.5, 4.2, 4.8},
		Description: "一款功能强大的智能手表",
	}

	fmt.Printf("产品名称: %s\n", product.Name)
	fmt.Printf("标签: %v\n", []string(product.Tags))
	fmt.Printf("分类ID: %v\n", []int64(product.Categories))
	fmt.Printf("评分: %v\n", []float64(product.Ratings))
	fmt.Printf("描述: %s\n", product.Description)

	// Output:
	// 产品名称: 智能手表
	// 标签: [electronics wearable smart]
	// 分类ID: [1 5 12]
	// 评分: [4.5 4.2 4.8]
	// 描述: 一款功能强大的智能手表
}

// 示例：展示如何扫描PostgreSQL数组
func ExamplePgArrayScan() {
	// 模拟从数据库中扫描出的字符串数组值
	var stringArray PgStringArray
	err := stringArray.Scan("{apple,banana,cherry}")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("扫描得到的字符串数组: %v\n", []string(stringArray))

	// 模拟从数据库中扫描出的整数数组值
	var intArray PgIntArray
	err = intArray.Scan("{1,2,3,4,5}")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("扫描得到的整数数组: %v\n", []int64(intArray))

	// 模拟从数据库中扫描出的浮点数组值
	var floatArray PgFloatArray
	err = floatArray.Scan("{1.1,2.2,3.3}")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("扫描得到的浮点数组: %v\n", []float64(floatArray))

	// Output:
	// 扫描得到的字符串数组: [apple banana cherry]
	// 扫描得到的整数数组: [1 2 3 4 5]
	// 扫描得到的浮点数组: [1.1 2.2 3.3]
}
