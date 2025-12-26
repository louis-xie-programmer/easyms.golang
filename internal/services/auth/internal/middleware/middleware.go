package middleware

import (
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/services/auth/internal/service"
	model "easyms/internal/shared/models"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// parseAndSetDetails 是一个辅助函数，负责解析令牌并填充上下文
func parseAndSetDetails(c *gin.Context, tokenService service.TokenService) error {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return consts.ErrNullToken
	}
	tokenValue := strings.TrimPrefix(authHeader, "Bearer ")

	oauth2Details, err := tokenService.GetOAuth2DetailsByAccessToken(c, tokenValue)
	if err != nil {
		return err
	}

	if oauth2Details != nil {
		if oauth2Details.Client != nil {
			c.Set(consts.OAuth2ClientDetailsKey, oauth2Details.Client)
		}
		if oauth2Details.User != nil {
			c.Set(consts.OAuth2UserDetailsKey, oauth2Details.User)
		}
	}
	return nil
}

// MakeSimpleClientMiddleware 用于校验客户端令牌
func MakeSimpleClientMiddleware(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := parseAndSetDetails(c, tokenService)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		// 验证客户端信息是否存在
		if _, exists := c.Get(consts.OAuth2ClientDetailsKey); !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidClient.Error()})
			return
		}

		c.Next()
	}
}

// MakeAuthorityAuthorizationMiddleware 用于校验用户令牌
func MakeAuthorityAuthorizationMiddleware(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := parseAndSetDetails(c, tokenService)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		// 验证客户端和用户信息是否存在
		if _, exists := c.Get(consts.OAuth2ClientDetailsKey); !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidClient.Error()})
			return
		}
		if _, exists := c.Get(consts.OAuth2UserDetailsKey); !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidUser.Error()})
			return
		}

		c.Next()
	}
}

// MakeScopeHandler 用于校验访问令牌是否包含指定权限
func MakeScopeHandler(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userDetails, ok := c.Get(consts.OAuth2UserDetailsKey)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": consts.ErrInvalidUser.Error()})
			return
		}

		user, ok := userDetails.(*model.User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": consts.ErrInvalidUser.Error()})
			return
		}

		// 检查用户是否具有所需权限
		userScopes := user.GetAuthorities()
		isAllow := false
		for _, s := range userScopes {
			if s == scope {
				isAllow = true
				break
			}
		}

		if !isAllow {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": consts.ErrInvalidScope.Error()})
			return
		}

		c.Next()
	}
}
