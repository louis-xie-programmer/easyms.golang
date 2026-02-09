// Package handles 包含了用户服务的所有 HTTP 处理器 (Handler)。
// 这些处理器负责接收 HTTP 请求，调用服务层 (service layer) 的业务逻辑，
// 并将结果格式化为 HTTP 响应。
package handles

import (
	"easyms/internal/services/user/internal/service"
	"easyms/internal/shared/errors" // 引入共享的错误包，用于统一错误处理
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UserHandler 封装了所有与用户相关的 HTTP 处理器。
// 它依赖于 service.UserService 接口来执行实际的业务逻辑。
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler 创建一个新的 UserHandler 实例。
// userService: 一个实现了 service.UserService 接口的实例，通过依赖注入传入。
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// CreateUserRequest 定义了创建用户的 HTTP 请求体结构。
// 字段上的 `json` 标签用于 JSON 序列化/反序列化。
// `binding` 标签用于 Gin 的请求参数验证，例如 "required" 表示字段必填，
// "alphanum" 表示只能包含字母和数字，"min" 和 "max" 定义了长度范围。
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,alphanum,min=4,max=30"` // 用户名，必填，4-30个字符，只能是字母数字
	Password string `json:"password" binding:"required,min=8,max=100"`         // 密码，必填，8-100个字符
	Email    string `json:"email" binding:"required,email"`                    // 邮箱，必填，且必须是有效邮箱格式
}

// CreateUser 处理创建用户的 HTTP 请求。
// 它负责解析请求体，调用服务层创建用户，并根据业务逻辑或错误返回相应的 HTTP 响应。
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	// 绑定并验证请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		// 如果请求参数验证失败，返回 400 Bad Request
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	// 调用服务层创建用户
	user, err := h.userService.Create(c, req.Username, req.Password, req.Email)
	if err != nil {
		// 将服务层返回的业务错误映射到相应的 HTTP 状态码
		if errors.Is(err, service.ErrUsernameExists) || errors.Is(err, service.ErrEmailExists) {
			// 如果用户名或邮箱已存在，返回 409 Conflict
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": err.Error()})
		} else {
			// 对于其他未知错误，返回 500 Internal Server Error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": "创建用户失败"})
		}
		return
	}

	// 用户创建成功，返回 201 Created 状态码和用户对象
	c.JSON(http.StatusCreated, user)
}

// GetUserByID 处理根据用户 ID 获取用户详情的 HTTP 请求。
func (h *UserHandler) GetUserByID(c *gin.Context) {
	// 从 URL 参数中获取用户 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32) // 将字符串 ID 转换为无符号整数
	if err != nil {
		// 如果 ID 格式无效，返回 400 Bad Request
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id", "message": "用户 ID 必须是正整数"})
		return
	}

	// 调用服务层获取用户详情
	user, err := h.userService.GetByID(c, uint(id))
	if err != nil {
		// 将服务层返回的业务错误映射到相应的 HTTP 状态码
		if errors.Is(err, service.ErrUserNotFound) {
			// 如果用户未找到，返回 404 Not Found
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": err.Error()})
		} else {
			// 对于其他未知错误，返回 500 Internal Server Error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": "获取用户失败"})
		}
		return
	}

	// 成功获取用户，返回 200 OK 状态码和用户对象
	c.JSON(http.StatusOK, user)
}
