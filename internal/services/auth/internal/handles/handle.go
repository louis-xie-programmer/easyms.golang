package handles

import (
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/services/auth/internal/service"
	"easyms/internal/shared/models"
	"github.com/gin-gonic/gin"
	"net/http"
)

// MakeTokenEndpoint 用于生成访问令牌(客户端公共令牌)
func MakeTokenEndpoint(svc service.TokenGranter, clientdetailsService service.ClientDetailsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &model.ClientTokenRequest{}
		if err := c.ShouldBindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// 检查授权类型是否支持
		if req.GrantType != "client_credentials" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "grant type is not supported"})
			return
		}

		if req.ClientId == "" || req.ClientSecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "client_id or client_secret is empty"})
			return
		}

		clientDetails, err := clientdetailsService.LoadClientByClientId(req.ClientId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if clientDetails == nil || clientDetails.ClientSecret != req.ClientSecret {
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidClient.Error()})
			return
		}

		// 验证以下
		token, err := svc.Grant(c, req.GrantType, clientDetails, &model.TokenRequest{
			GrantType: req.GrantType,
		})

		var errString = ""
		if err != nil {
			errString = err.Error()
		}

		response := model.TokenResponse{
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

// LoginEndPoint 处理登录请求, 需要带客户端共享令牌
func LoginEndPoint(userDetailsService service.UserDetailsService, tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &model.LoginRequest{}
		if err := c.ShouldBindJSON(req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Username == "" || req.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username or password is empty"})
			return
		}

		// 加载用户详情（而不是直接通过用户名和密码获取）
		userDetails, err := userDetailsService.LoadUserByUsername(req.Username)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 手动验证密码
		if !userDetails.CheckPassword(req.Password) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid username or password"})
			return
		}

		// 因为中间件已经验证过了客户端详情，这里就不需要再次验证了
		//clientDetails, ok := c.Value(consts.OAuth2ClientDetailsKey).(*model.ClientDetails)
		//if !ok {
		//	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidClient.Error()})
		//}

		token, err := tokenService.CreateAccessToken(&model.OAuth2Details{
			User:   userDetails,
			Client: c.Value(consts.OAuth2ClientDetailsKey).(*model.ClientDetails),
		})

		var errString = ""
		if err != nil {
			errString = err.Error()
		}

		response := model.TokenResponse{
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

// RefreshTokenEndpoint 处理刷新令牌请求(分用户令牌，客户端公共令牌，由中间件来区分)
func RefreshTokenEndpoint(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &model.RefreshTokenRequest{}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.RefreshToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
			return
		}

		// 中间件已经验证过了客户端详情，这里就不需要再次验证了
		//clientDetails, ok := c.Value(consts.OAuth2ClientDetailsKey).(*model.ClientDetails)
		//if !ok {
		//	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid client"})
		//}

		newAccessToken, err := tokenService.RefreshAccessToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		response := model.TokenResponse{
			AccessToken: newAccessToken,
		}

		// 如果新的访问令牌包含刷新令牌，也在响应中包含
		if newAccessToken != nil && newAccessToken.RefreshToken != nil {
			response.RefreshToken = newAccessToken.RefreshToken
		}

		c.JSON(http.StatusOK, response)
	}
}
