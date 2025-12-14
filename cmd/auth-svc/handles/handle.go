package handles

import (
	"easyms/cmd/auth-svc/model"
	"easyms/cmd/auth-svc/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type CheckTokenRequest struct {
	Token         string `json:"token"`
	ClientDetails model.ClientDetails
}

type CheckTokenResponse struct {
	OAuthDetails *model.OAuth2Details `json:"o_auth_details"`
	Error        string               `json:"error"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
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

type TokenResponse struct {
	AccessToken  *model.OAuth2Token `json:"access_token"`
	RefreshToken *model.OAuth2Token `json:"refresh_token,omitempty"`
	Error        string             `json:"error"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// MakeTokenEndpoint 用于生成访问令牌
func MakeTokenEndpoint(svc service.TokenGranter) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &service.TokenRequest{}
		if err := c.ShouldBindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		token, err := svc.Grant(c, req.GrantType, c.Value(OAuth2ClientDetailsKey).(*model.ClientDetails), req)

		var errString = ""
		if err != nil {
			errString = err.Error()
		}

		response := TokenResponse{
			AccessToken: token,
			Error:       errString,
		}

		// 如果令牌包含刷新令牌，也在响应中包含
		if token != nil && token.RefreshToken != nil {
			response.RefreshToken = token.RefreshToken
		}

		c.JSON(http.StatusOK, response)
	}
}

func LoginEndPoint(userDetailsService service.UserDetailsService, tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &LoginRequest{}
		if err := c.ShouldBindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		user, err := userDetailsService.GetUserDetailByUsername(c, req.Username, req.Password)
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

		response := TokenResponse{
			AccessToken: token,
			Error:       errString,
		}

		// 如果令牌包含刷新令牌，也在响应中包含
		if token != nil && token.RefreshToken != nil {
			response.RefreshToken = token.RefreshToken
		}

		c.JSON(http.StatusOK, response)
	}
}

// RefreshTokenEndpoint 处理刷新令牌请求
func RefreshTokenEndpoint(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &RefreshTokenRequest{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.RefreshToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
			return
		}

		//oAuth2Details, ok := c.Value(OAuth2DetailsKey).(*model.OAuth2Details)
		//
		//if !ok {
		//	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid client"})
		//}

		newAccessToken, err := tokenService.RefreshAccessToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		response := TokenResponse{
			AccessToken: newAccessToken,
		}

		// 如果新的访问令牌包含刷新令牌，也在响应中包含
		if newAccessToken != nil && newAccessToken.RefreshToken != nil {
			response.RefreshToken = newAccessToken.RefreshToken
		}

		c.JSON(http.StatusOK, response)
	}
}

// RefreshClientTokenEndpoint 处理客户端刷新令牌请求
func RefreshClientTokenEndpoint(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.RefreshToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
			return
		}

		// 使用刷新令牌获取新的访问令牌
		newAccessToken, err := tokenService.RefreshAccessToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		response := TokenResponse{
			AccessToken: newAccessToken,
		}

		// 如果新的访问令牌包含刷新令牌，也在响应中包含
		if newAccessToken != nil && newAccessToken.RefreshToken != nil {
			response.RefreshToken = newAccessToken.RefreshToken
		}

		c.JSON(http.StatusOK, response)
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

// MakeClientOnlyAuthorizationMiddleware 用于校验访问令牌是否为客户端凭证授权生成
func MakeClientOnlyAuthorizationMiddleware(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		accessTokenValue := c.GetHeader("Authorization")
		if accessTokenValue == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		// 获取令牌对应的详细信息
		oauth2Details, err := tokenService.GetOAuth2DetailsByAccessToken(accessTokenValue)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		// 设置客户端详情到上下文中，以便后续使用
		c.Set(OAuth2ClientDetailsKey, oauth2Details.Client)

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
		if _, ok := c.Value(OAuth2DetailsKey).(*model.OAuth2Details); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		} else {
			c.Next()
		}
	}
}
