// Package errors 提供了统一的业务错误处理机制。
// 它定义了标准的业务错误结构体 BizError，并预定义了通用的错误码和错误实例，
// 旨在规范化整个应用的错误处理流程。
package errors

import (
	"fmt"
	"net/http"
)

// BizError 定义了标准的业务错误结构体。
// 它包含了错误码和错误信息，并可以包装一个底层的原始错误。
type BizError struct {
	Code    int    `json:"code"`    // 业务错误码，用于程序化地识别错误类型
	Message string `json:"message"` // 人类可读的错误信息
	Err     error  `json:"-"`       // 原始的底层错误，不会被序列化到 JSON 中
}

// Error 实现了 error 接口，使得 BizError 可以像普通 error 一样使用。
func (e *BizError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("业务错误: Code=%d, Message=%s, OriginalError=%v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("业务错误: Code=%d, Message=%s", e.Code, e.Message)
}

// Unwrap 提供了对原始错误的解包能力，以支持 Go 1.13+ 的错误包装链 (error wrapping)。
// 这使得可以使用 errors.Is 和 errors.As 来检查底层错误。
func (e *BizError) Unwrap() error {
	return e.Err
}

// New 创建一个新的 BizError。
func New(code int, message string) *BizError {
	return &BizError{
		Code:    code,
		Message: message,
	}
}

// Wrap 将一个现有的 error 包装成一个新的 BizError，并附加业务错误码和信息。
func Wrap(err error, code int, message string) *BizError {
	return &BizError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// 通用业务错误码定义
const (
	SuccessCode             = 0     // 成功
	ServerErrorCode         = 10000 // 通用服务器错误
	InvalidParamsCode       = 10001 // 无效参数
	NotFoundCode            = 10002 // 资源未找到
	UnauthorizedCode        = 10003 // 未授权 (通常是认证失败)
	ForbiddenCode           = 10004 // 禁止访问 (通常是权限不足)
	ConflictCode            = 10005 // 资源冲突 (例如，尝试创建已存在的资源)
	TooManyRequestsCode     = 10006 // 请求过于频繁
	InternalServerErrorCode = 10007 // 内部服务器错误 (更具体的服务器错误)
)

// 预定义的通用错误实例，方便在代码中直接使用
var (
	Success             = New(SuccessCode, "成功")
	ErrServer           = New(ServerErrorCode, "服务器内部错误")
	ErrInvalidParams    = New(InvalidParamsCode, "无效的参数")
	ErrNotFound         = New(NotFoundCode, "资源未找到")
	ErrUnauthorized     = New(UnauthorizedCode, "未授权")
	ErrForbidden        = New(ForbiddenCode, "禁止访问")
	ErrConflict         = New(ConflictCode, "资源冲突")
	ErrTooManyRequests  = New(TooManyRequestsCode, "请求过于频繁")
)

// HTTPStatusFromCode 将业务错误码映射到标准的 HTTP 状态码。
// 这在 API 网关或 HTTP 服务中非常有用，可以根据业务错误返回正确的 HTTP 响应。
func HTTPStatusFromCode(code int) int {
	switch code {
	case SuccessCode:
		return http.StatusOK
	case InvalidParamsCode:
		return http.StatusBadRequest
	case NotFoundCode:
		return http.StatusNotFound
	case UnauthorizedCode:
		return http.StatusUnauthorized
	case ForbiddenCode:
		return http.StatusForbidden
	case ConflictCode:
		return http.StatusConflict
	case TooManyRequestsCode:
		return http.StatusTooManyRequests
	case ServerErrorCode, InternalServerErrorCode:
		return http.StatusInternalServerError
	default:
		// 对于未知的业务错误码，默认返回 500 内部服务器错误
		return http.StatusInternalServerError
	}
}
