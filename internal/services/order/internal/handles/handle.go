package handles

import (
	"net/http"

	"easyms/internal/services/order/internal/service"
	"github.com/gin-gonic/gin"
)

// CreateOrderRequest defines the request body for creating an order (DTO for binding).
type CreateOrderRequest struct {
	UserID     uint                     `json:"user_id" binding:"required"`
	OrderItems []CreateOrderItemRequest `json:"order_items" binding:"required,min=1,dive"`
}

type CreateOrderItemRequest struct {
	ProductID uint    `json:"product_id" binding:"required"`
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	Price     float64 `json:"price" binding:"required,gt=0"`
}

// MakeCreateOrderEndpoint creates a Gin Handler for creating an order.
func MakeCreateOrderEndpoint(svc service.OrderService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateOrderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
			return
		}

		// Convert the request DTO to the service layer DTO.
		// In this case, they are very similar, but this provides a separation layer.
		serviceDTO := &service.CreateOrderRequestDTO{
			UserID:     req.UserID,
			OrderItems: make([]service.CreateOrderItemDTO, len(req.OrderItems)),
		}
		for i, item := range req.OrderItems {
			serviceDTO.OrderItems[i] = service.CreateOrderItemDTO{
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				Price:     item.Price,
			}
		}

		// Call the core business logic.
		// The handle layer is no longer responsible for building the model or calculating totals.
		createdOrder, err := svc.CreateOrder(c.Request.Context(), serviceDTO)
		if err != nil {
			// TODO: Implement a proper error handler to map business errors to status codes.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, createdOrder)
	}
}
