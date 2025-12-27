package handles

import (
	"easyms/internal/services/user/internal/service"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UserHandler 封装了用户相关的 HTTP 处理器
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler 创建一个新的 UserHandler 实例
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// CreateUserRequest 定义了创建用户的请求体
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,alphanum,min=4,max=30"`
	Password string `json:"password" binding:"required,min=8,max=100"`
	Email    string `json:"email" binding:"required,email"`
}

// CreateUser 处理创建用户的请求
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	user, err := h.userService.Create(c, req.Username, req.Password, req.Email)
	if err != nil {
		// Map business errors to specific HTTP status codes
		if errors.Is(err, service.ErrUsernameExists) || errors.Is(err, service.ErrEmailExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": "Failed to create user"})
		}
		return
	}

	c.JSON(http.StatusCreated, user)
}

// GetUserByID 处理根据 ID 获取用户的请求
func (h *UserHandler) GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id", "message": "User ID must be a positive integer"})
		return
	}

	user, err := h.userService.GetByID(c, uint(id))
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": "Failed to retrieve user"})
		}
		return
	}

	c.JSON(http.StatusOK, user)
}
