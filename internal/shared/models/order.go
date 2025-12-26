package models

import (
	"time"
)

// OrderStatus 定义了订单的状态
type OrderStatus string

const (
	StatusPending   OrderStatus = "PENDING"
	StatusPaid      OrderStatus = "PAID"
	StatusShipped   OrderStatus = "SHIPPED"
	StatusCompleted OrderStatus = "COMPLETED"
	StatusCancelled OrderStatus = "CANCELLED"
)

// Order 是订单的 GORM 模型
type Order struct {
	ID          uint        `gorm:"primaryKey"`
	UserID      uint        `gorm:"not null;index"`
	TotalAmount float64     `gorm:"not null"`
	Status      OrderStatus `gorm:"type:varchar(20);not null;default:'PENDING'"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	OrderItems  []OrderItem `gorm:"foreignKey:OrderID"`
}

// OrderItem 是订单项的 GORM 模型
type OrderItem struct {
	ID        uint    `gorm:"primaryKey"`
	OrderID   uint    `gorm:"not null;index"`
	ProductID uint    `gorm:"not null"`
	Quantity  int     `gorm:"not null"`
	Price     float64 `gorm:"not null"`
}

// TableName 为 Order 模型指定表名
func (Order) TableName() string {
	return "orders"
}

// TableName 为 OrderItem 模型指定表名
func (OrderItem) TableName() string {
	return "order_items"
}
