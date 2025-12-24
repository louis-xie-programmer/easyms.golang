package service

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

// mockTx 实现了 db.TxTransaction 接口，用于测试
type mockTx struct {
	db *mockDatabase
}

func (m *mockTx) Insert(value interface{}) error {
	if m.db.shouldFail {
		return errors.New("mock db insert error")
	}
	// 根据类型将插入的对象存入 mockDatabase 的切片中，以便断言
	switch v := value.(type) {
	case *model.Order:
		m.db.insertedOrders = append(m.db.insertedOrders, v)
	case *model.OutboxEvent:
		m.db.insertedEvents = append(m.db.insertedEvents, v)
	}
	return nil
}

// 其他 TxTransaction 方法的空实现
func (m *mockTx) Update(model interface{}, updates map[string]interface{}) error  { return nil }
func (m *mockTx) Delete(model interface{}, conds ...interface{}) error            { return nil }
func (m *mockTx) Query(dest interface{}, query string, args ...interface{}) error { return nil }
func (m *mockTx) Count(query string, args ...interface{}) (int64, error)          { return 0, nil }
func (m *mockTx) Where(query string, args ...interface{}) *gorm.DB                { return nil }
func (m *mockTx) Order(query string) *gorm.DB                                     { return nil }
func (m *mockTx) Limit(limit int) *gorm.DB                                        { return nil }
func (m *mockTx) Commit() error                                                   { return nil }
func (m *mockTx) Rollback() error                                                 { return nil }
func (m *mockTx) GetDB() *gorm.DB                                                 { return nil }

// mockDatabase 实现了 db.Database 接口，用于测试
type mockDatabase struct {
	shouldFail     bool
	insertedOrders []*model.Order
	insertedEvents []*model.OutboxEvent
}

func (m *mockDatabase) RunInTransaction(fn func(tx db.TxTransaction) error) error {
	if m.shouldFail {
		// 模拟事务开始就失败的场景
		return errors.New("mock db transaction error")
	}
	// 直接执行传入的函数，模拟事务块
	// 如果 fn 返回错误，这里的 RunInTransaction 也会返回错误，模拟了事务回滚
	return fn(&mockTx{db: m})
}

// 其他 Database 方法的空实现
func (m *mockDatabase) AutoMigrate(models ...interface{}) error                         { return nil }
func (m *mockDatabase) Insert(value interface{}) error                                  { return nil }
func (m *mockDatabase) Query(dest interface{}, query string, args ...interface{}) error { return nil }
func (m *mockDatabase) Count(query string, args ...interface{}) (int64, error)          { return 0, nil }
func (m *mockDatabase) Where(query string, args ...interface{}) *gorm.DB                { return nil }
func (m *mockDatabase) Order(query string) *gorm.DB                                     { return nil }
func (m *mockDatabase) Limit(limit int) *gorm.DB                                        { return nil }
func (m *mockDatabase) Update(model interface{}, updates map[string]interface{}) error  { return nil }
func (m *mockDatabase) Delete(model interface{}, conds ...interface{}) error            { return nil }
func (m *mockDatabase) GetDB() *gorm.DB                                                 { return nil }
func (m *mockDatabase) GetType() string                                                 { return "mock" }
func (m *mockDatabase) Begin() (db.TxTransaction, error)                                { return nil, nil }

// TestCreateOrder_Success 测试成功创建订单的场景
func TestCreateOrder_Success(t *testing.T) {
	// 1. 准备
	mockDB := &mockDatabase{}
	orderService := NewOrderService(mockDB)
	sampleOrder := &model.Order{
		UserID:      1,
		TotalAmount: 199.99,
		OrderItems: []model.OrderItem{
			{ProductID: 101, Quantity: 1, Price: 199.99},
		},
	}

	// 2. 执行
	err := orderService.CreateOrder(context.Background(), sampleOrder)

	// 3. 断言
	if err != nil {
		t.Fatalf("Expected no error, but got: %v", err)
	}

	// 验证 Order 是否被“插入”
	if len(mockDB.insertedOrders) != 1 {
		t.Fatalf("Expected 1 order to be inserted, but got %d", len(mockDB.insertedOrders))
	}
	if !reflect.DeepEqual(mockDB.insertedOrders[0], sampleOrder) {
		t.Errorf("Inserted order does not match the sample order")
	}

	// 验证 OutboxEvent 是否被“插入”
	if len(mockDB.insertedEvents) != 1 {
		t.Fatalf("Expected 1 outbox event to be inserted, but got %d", len(mockDB.insertedEvents))
	}
	event := mockDB.insertedEvents[0]
	if event.Exchange != "orders.topic" {
		t.Errorf("Expected event exchange to be 'orders.topic', but got '%s'", event.Exchange)
	}
	if event.RoutingKey != "order.created" {
		t.Errorf("Expected event routing key to be 'order.created', but got '%s'", event.RoutingKey)
	}

	// 验证事件的 Payload 内容是否正确
	var payloadOrder model.Order
	if err := json.Unmarshal(event.Payload, &payloadOrder); err != nil {
		t.Fatalf("Failed to unmarshal event payload: %v", err)
	}
	if !reflect.DeepEqual(&payloadOrder, sampleOrder) {
		t.Errorf("Event payload does not match the sample order")
	}
}

// TestCreateOrder_DBError 测试数据库插入失败的场景
func TestCreateOrder_DBError(t *testing.T) {
	// 1. 准备
	mockDB := &mockDatabase{shouldFail: true} // 设置 mockDB 为失败模式
	orderService := NewOrderService(mockDB)
	sampleOrder := &model.Order{UserID: 1}

	// 2. 执行
	err := orderService.CreateOrder(context.Background(), sampleOrder)

	// 3. 断言
	if err == nil {
		t.Fatal("Expected an error, but got nil")
	}

	// 验证在事务失败时，没有任何东西被“插入”
	if len(mockDB.insertedOrders) > 0 {
		t.Errorf("Expected 0 orders to be inserted on failure, but got %d", len(mockDB.insertedOrders))
	}
	if len(mockDB.insertedEvents) > 0 {
		t.Errorf("Expected 0 outbox events to be inserted on failure, but got %d", len(mockDB.insertedEvents))
	}
}
