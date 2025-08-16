package middleware

import (
	"context"
)

// OperationType 解决 Go 类型别名与第三方包类型不兼容问题
type OperationType func(context.Context) (interface{}, error)

// ToCBOperation 返回 func(context.Context) (interface{}, error) 直接作为 circuitbreaker.Operation
func ToCBOperation(op OperationType) func(context.Context) (interface{}, error) {
	return func(ctx context.Context) (interface{}, error) {
		return op(ctx)
	}
}
