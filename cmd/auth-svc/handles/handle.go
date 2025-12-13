package handles

import (
	"easyms/cmd/auth-svc/model"
	"easyms/cmd/auth-svc/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type CheckTokenRequest struct {
	Token         string
	ClientDetails model.ClientDetails
}

type CheckTokenResponse struct {
	OAuthDetails *model.OAuth2Details `json:"o_auth_details"`
	Error        string               `json:"error"`
}

// CheckTokenEndPoint 用于校验访问令牌是否有效
func CheckTokenEndPoint(svc service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &CheckTokenRequest{}
		if err := c.ShouldBindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		tokenDetails, err := svc.GetOAuth2DetailsByAccessToken(req.Token)

		var errString = ""
		if err != nil {
			errString = err.Error()
		}

		c.JSON(http.StatusOK, CheckTokenResponse{
			OAuthDetails: tokenDetails,
			Error:        errString,
		})
	}
}

const (
	OAuth2DetailsKey       = "OAuth2Details"
	OAuth2ClientDetailsKey = "OAuth2ClientDetails"
	OAuth2ErrorKey         = "OAuth2Error"
)

type TokenRequest struct {
	GrantType string
	Reader    *http.Request
}

type TokenResponse struct {
	AccessToken *model.OAuth2Token `json:"access_token"`
	Error       string             `json:"error"`
}

// MakeTokenEndpoint 用于生成访问令牌
func MakeTokenEndpoint(svc service.TokenGranter) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &TokenRequest{}
		if err := c.ShouldBindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		token, err := svc.Grant(c, req.GrantType, c.Value(OAuth2ClientDetailsKey).(*model.ClientDetails), req.Reader)

		var errString = ""
		if err != nil {
			errString = err.Error()
		}

		c.JSON(http.StatusOK, TokenResponse{
			AccessToken: token,
			Error:       errString,
		})
	}
}

func LoginEndPoint(userDetailsService service.UserDetailsService, tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetHeader("username")
		password := c.GetHeader("password")

		user, err := userDetailsService.GetUserDetailByUsername(c, username, password)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		token, err := tokenService.CreateAccessToken(&model.OAuth2Details{
			User:   user,
			Client: c.Value(OAuth2ClientDetailsKey).(*model.ClientDetails),
		})

		var errString = ""
		if err != nil {
			errString = err.Error()
		}

		//c.Set(OAuth2DetailsKey, &model.OAuth2Details{
		//	User:    user,
		//	Client:  c.Value(OAuth2ClientDetailsKey).(*model.ClientDetails),
		//})

		c.JSON(http.StatusOK, TokenResponse{
			AccessToken: token,
			Error:       errString,
		})
	}
}

// MakeClientAuthorizationMiddleware 用于校验客户端是否有效
func MakeClientAuthorizationMiddleware(clientDetailsService service.ClientDetailsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientId := c.GetHeader("client_id")
		clientSecret := c.GetHeader("client_secret")

		clientDetails, err := clientDetailsService.GetClientDetailByClientId(c, clientId, clientSecret)
		if err != nil {
			c.Set(OAuth2ErrorKey, err)
		} else {
			c.Set(OAuth2ClientDetailsKey, clientDetails)
		}

		if err, ok := c.Value(OAuth2ErrorKey).(error); ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		}
		if _, ok := c.Value(OAuth2ClientDetailsKey).(*model.ClientDetails); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid client"})
		}
		c.Next()
	}
}

// MakeAuthorityAuthorizationMiddleware 用于校验访问令牌是否包含指定权限
// 校验用户权限
func MakeAuthorityAuthorizationMiddleware(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessTokenValue := c.GetHeader("Authorization")
		var err error
		if accessTokenValue != "" {
			// 获取令牌对应的用户信息和客户端信息
			oauth2Details, err := tokenService.GetOAuth2DetailsByAccessToken(accessTokenValue)
			if err == nil {
				c.Set(OAuth2DetailsKey, oauth2Details)
			}
		} else {
			c.Set(OAuth2ErrorKey, err)
		}

		if err, ok := c.Value(OAuth2ErrorKey).(error); ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		}
		if details, ok := c.Value(OAuth2DetailsKey).(*model.OAuth2Details); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		} else {
			for _, value := range details.User.Authorities {
				if value == accessTokenValue {
					c.Next()
				}
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authority"})
		}
	}
}
