package middleware

import (
	"easyms/pkg/config"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

//JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
        if !strings.HasPrefix(authHeader, "Bearer ") {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing token"})
            return
        }
        tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
        cfg := config.GetAppConfig()
        keyFunc := func(token *jwt.Token) (interface{}, error) {
            return []byte(cfg.OAuth2.JWTSecret), nil // 或加载公钥
        }
        token, err := jwt.Parse(tokenStr, keyFunc)
        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
            return
        }
		c.Next()
	}
}
