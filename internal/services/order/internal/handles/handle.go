package handles

import (
	"net/http"

	"easyms/internal/services/order/internal/service"
	"easyms/internal/shared/models"
	"github.com/gin-gonic/gin"
)

// CreateOrderRequest 定义了创建订单的请求体
type CreateOrderRequest struct {
	UserID     uint                     `json:"user_id" binding:"required"`
	OrderItems []CreateOrderItemRequest `json:"order_items" binding:"required,dive"`
}

type CreateOrderItemRequest struct {
	ProductID uint    `json:"product_id" binding:"required"`
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	Price     float64 `json:"price" binding:"required,gt=0"`
}

// MakeCreateOrderEndpoint 创建一个处理订单创建请求的 Gin Handler
func MakeCreateOrderEndpoint(svc service.OrderService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 将请求体转换为 GORM 模型
		order := &model.Order{
			UserID:      req.UserID,
			Status:      model.StatusPending,
			OrderItems:  make([]model.OrderItem, len(req.OrderItems)),
			TotalAmount: 0,
		}

		var total float64
		for i, item := range req.OrderItems {
			order.OrderItems[i] = model.OrderItem{
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				Price:     item.Price,
			}
			total += float64(item.Quantity) * item.Price
		}
		order.TotalAmount = total

		// 调用核心业务逻辑
		if err := svc.CreateOrder(c.Request.Context(), order); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, order)
	}
}
