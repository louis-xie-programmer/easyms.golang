package middleware

import (
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/services/auth/internal/service"
	model "easyms/internal/shared/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// MakeClientOnlyAuthorizationMiddleware 用于校验访问令牌是否为客户端凭证授权生成
func MakeSimpleClientMiddleware(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessTokenValue := c.GetHeader("Authorization")
		if accessTokenValue != "" {
			// 获取令牌对应的详细信息
			oauth2Details, err := tokenService.GetOAuth2DetailsByAccessToken(accessTokenValue)
			if err != nil {
				c.Set(consts.OAuth2ErrorKey, err)
			}
			if oauth2Details == nil || oauth2Details.Client == nil {
				c.Set(consts.OAuth2ErrorKey, consts.ErrInvalidClient)
			} else {
				// 设置客户端详情到上下文中，以便后续使用
				c.Set(consts.OAuth2ClientDetailsKey, oauth2Details.Client)

				// 注意面向内部的公共接口是不需要用户的，但是如果用户存在，照样将添加到上下文中，以便后续使用（如日志）
				if oauth2Details.User != nil {
					c.Set(consts.OAuth2UserDetailsKey, oauth2Details.User)
				}
			}
		} else {
			c.Set(consts.OAuth2ErrorKey, consts.ErrNullToken)
		}

		//Todo: 权限验证逻辑，建议是单独封装一个权限验证中间件

		if err, ok := c.Value(consts.OAuth2ErrorKey).(error); ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		} else {
			c.Next()
		}
	}
}

// MakeAuthorityAuthorizationMiddleware 用于校验访问令牌是否包含指定权限
// 校验用户权限
func MakeAuthorityAuthorizationMiddleware(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessTokenValue := c.GetHeader("Authorization")
		if accessTokenValue != "" {
			// 获取令牌对应的用户信息和客户端信息
			oauth2Details, err := tokenService.GetOAuth2DetailsByAccessToken(accessTokenValue)
			if err != nil {
				c.Set(consts.OAuth2ErrorKey, err)
			}
			if oauth2Details == nil || oauth2Details.Client == nil {
				c.Set(consts.OAuth2ErrorKey, consts.ErrInvalidClient)
			} else {
				// 设置客户端详情到上下文中，以便后续使用
				c.Set(consts.OAuth2ClientDetailsKey, oauth2Details.Client)

				if oauth2Details.User == nil {
					c.Set(consts.OAuth2ErrorKey, consts.ErrInvalidUser)
				} else {
					c.Set(consts.OAuth2UserDetailsKey, oauth2Details.User)
				}
			}
		} else {
			c.Set(consts.OAuth2ErrorKey, consts.ErrNullToken)
		}

		if err, ok := c.Value(consts.OAuth2ErrorKey).(error); ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		} else {
			c.Next()
		}
	}
}

// MakeScopeHandler 用于校验访问令牌是否包含指定权限
func MakeScopeHandler(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从上一个基本中间中件获取用户详情
		userDetails, ok := c.Value(consts.OAuth2UserDetailsKey).(*model.UserDetails)
		if !ok || userDetails == nil {
			c.Set(consts.OAuth2ErrorKey, consts.ErrInvalidUser)
		} else {
			// 检查用户是否具有所需权限
			userScopes := userDetails.GetAuthorities()
			isAllow := false
			for _, s := range userScopes {
				if s == scope {
					isAllow = true
					break
				}
			}
			if !isAllow {
				c.Set(consts.OAuth2ErrorKey, consts.ErrInvalidScope)
			}
		}

		if err, ok := c.Value(consts.OAuth2ErrorKey).(error); ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		} else {
			c.Next()
		}
	}
}
