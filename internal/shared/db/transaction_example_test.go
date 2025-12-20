package db

import (
	"errors"
	"fmt"
)

// User 用户模型
type User struct {
	ID    int64  `gorm:"primaryKey"`
	Name  string `gorm:"type:varchar(100)"`
	Email string `gorm:"type:varchar(100);uniqueIndex"`
}

// Account 账户模型
type Account struct {
	ID      int64 `gorm:"primaryKey"`
	UserID  int64
	Balance int64 // 余额，以分为单位
}

// TransferMoney 转账示例函数
func TransferMoney(db Database, fromUserID, toUserID int64, amount int64) error {
	// 开启事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}

	// 使用 defer 确保事务最终会被处理
	defer func() {
		if r := recover(); r != nil {
			// 发生 panic，回滚事务
			tx.Rollback()
			panic(r) // 重新抛出 panic
		}
	}()

	// 查询转出账户
	var fromAccount Account
	if err := tx.Query(&fromAccount, "SELECT * FROM accounts WHERE user_id = ? FOR UPDATE", fromUserID); err != nil {
		tx.Rollback()
		return fmt.Errorf("查询转出账户失败: %w", err)
	}

	// 检查余额是否充足
	if fromAccount.Balance < amount {
		tx.Rollback()
		return errors.New("余额不足")
	}

	// 查询转入账户
	var toAccount Account
	if err := tx.Query(&toAccount, "SELECT * FROM accounts WHERE user_id = ? FOR UPDATE", toUserID); err != nil {
		tx.Rollback()
		return fmt.Errorf("查询转入账户失败: %w", err)
	}

	// 执行转账操作
	fromAccount.Balance -= amount
	toAccount.Balance += amount

	// 更新转出账户余额
	if err := tx.Update(&fromAccount, map[string]interface{}{"balance": fromAccount.Balance}); err != nil {
		tx.Rollback()
		return fmt.Errorf("更新转出账户失败: %w", err)
	}

	// 更新转入账户余额
	if err := tx.Update(&toAccount, map[string]interface{}{"balance": toAccount.Balance}); err != nil {
		tx.Rollback()
		return fmt.Errorf("更新转入账户失败: %w", err)
	}

	// 记录转账日志
	transferLog := map[string]interface{}{
		"from_user_id": fromUserID,
		"to_user_id":   toUserID,
		"amount":       amount,
	}

	// 插入转账日志
	if err := tx.Insert(transferLog); err != nil {
		tx.Rollback()
		return fmt.Errorf("记录转账日志失败: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// CreateUserWithAccount 创建用户及其账户示例函数
func CreateUserWithAccount(db Database, name, email string) error {
	// 开启事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}

	// 使用 defer 确保事务最终会被处理
	defer func() {
		if r := recover(); r != nil {
			// 发生 panic，回滚事务
			tx.Rollback()
			panic(r) // 重新抛出 panic
		}
	}()

	// 创建用户
	user := User{
		Name:  name,
		Email: email,
	}

	if err := tx.Insert(&user); err != nil {
		tx.Rollback()
		return fmt.Errorf("创建用户失败: %w", err)
	}

	// 创建账户
	account := Account{
		UserID:  user.ID,
		Balance: 0,
	}

	if err := tx.Insert(&account); err != nil {
		tx.Rollback()
		return fmt.Errorf("创建账户失败: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	fmt.Printf("成功创建用户 %s (ID: %d) 及其账户\n", user.Name, user.ID)
	return nil
}

func ExampleTransferMoney() {
	// 这只是一个示例，不会真正执行转账操作
	fmt.Println("转账示例演示了如何使用事务确保数据一致性")

	// Output:
	// 转账示例演示了如何使用事务确保数据一致性
}

func ExampleCreateUserWithAccount() {
	// 这只是一个示例，不会真正创建用户
	fmt.Println("创建用户及账户示例演示了如何在事务中执行多个相关操作")

	// Output:
	// 创建用户及账户示例演示了如何在事务中执行多个相关操作
}
