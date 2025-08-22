package db

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
)

type Database interface {
	Insert(value interface{}) error
	Query(dest interface{}, query string, args ...interface{}) error
	Count(query string, args ...interface{}) (int64, error)
	Where(query string, args ...interface{}) *gorm.DB
	Order(query string) *gorm.DB
	Limit(limit int) *gorm.DB
	Update(model interface{}, updates map[string]interface{}) error
	Delete(model interface{}, conds ...interface{}) error
}

func NewEasyDatabase(dbType string, connStr string) (Database, error) {
	var dialector gorm.Dialector

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

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return &EasyDatabase{DB: db}, nil
}

type EasyDatabase struct {
	DB *gorm.DB
}

// Insert 插入数据
func (ed *EasyDatabase) Insert(value interface{}) error {
	return ed.DB.Create(value).Error
}

// Query 查询（可传 model + 条件）
func (ed *EasyDatabase) Query(dest interface{}, query string, args ...interface{}) error {
	return ed.DB.Raw(query, args...).Scan(dest).Error
}

func (ed *EasyDatabase) Count(query string, args ...interface{}) (int64, error) {
	var count int64
	err := ed.DB.Raw(query, args...).Count(&count).Error
	return count, err
}

func (ed *EasyDatabase) Where(query string, args ...interface{}) *gorm.DB {
	return ed.DB.Where(query, args...)
}

func (ed *EasyDatabase) Order(query string) *gorm.DB {
	return ed.DB.Order(query)
}

func (ed *EasyDatabase) Limit(limit int) *gorm.DB {
	return ed.DB.Limit(limit)
}

// Update 更新数据
func (ed *EasyDatabase) Update(model interface{}, updates map[string]interface{}) error {
	return ed.DB.Model(model).Updates(updates).Error
}

// Delete 删除数据
func (ed *EasyDatabase) Delete(model interface{}, conds ...interface{}) error {
	return ed.DB.Delete(model, conds...).Error
}
