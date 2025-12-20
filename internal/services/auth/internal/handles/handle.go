package handles

import (
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/services/auth/internal/service"
	model "easyms/internal/shared/models"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// respondWithToken 封装了统一的令牌响应逻辑
func respondWithToken(c *gin.Context, token *model.OAuth2Token, err error) {
	if err != nil {
		// 根据错误类型返回不同的HTTP状态码
		if errors.Is(err, consts.ErrInvalidClient) || errors.Is(err, consts.ErrInvalidUsernameAndPasswordRequest) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, model.TokenResponse{
		AccessToken:  token,
		RefreshToken: token.RefreshToken, // RefreshToken可能为nil，这没问题
	})
}

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
			c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrNotSupportGrantType.Error()})
			return
		}

		if req.ClientId == "" || req.ClientSecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "client_id and client_secret are required"})
			return
		}

		clientDetails, err := clientdetailsService.LoadClientByClientId(req.ClientId)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidClient.Error()})
			return
		}

		if clientDetails == nil || clientDetails.ClientSecret != req.ClientSecret {
			c.JSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidClient.Error()})
			return
		}

		token, err := svc.Grant(c, req.GrantType, clientDetails, &model.TokenRequest{
			GrantType: req.GrantType,
		})
		respondWithToken(c, token, err)
	}
}

func VerifyTokenEndpoint(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing token"})
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		// 使用tokenService验证令牌
		_, err := tokenService.GetOAuth2DetailsByAccessToken(tokenStr)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token", "detail": err.Error()})
			return
		}

		// 令牌有效，返回成功
		c.JSON(http.StatusOK, gin.H{"valid": true})
	}
}

// RegisterClientEndPoint 注册客户端端点
func RegisterClientEndPoint(service service.ClientDetailsService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		req := &model.RegisterClientRequest{}
		if err := ctx.ShouldBindJSON(req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "detail": err.Error()})
			return
		}

		// 验证客户端
		clientDetails, err := service.CreateClientDetails(req.ClientId)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Failed to register client", "detail": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, model.RegisterClientResponse{
			ClientId:     clientDetails.ClientId,
			ClientSecret: clientDetails.ClientSecret,
		})
	}
}

// RegisterUserEndPoint 注册用户端点
func RegisterUserEndPoint(service service.UserDetailsService, authorities []string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		req := &model.RegisterUserRequest{}
		if err := ctx.ShouldBindJSON(req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "detail": err.Error()})
			return
		}
		if req.Password == "" || req.Username == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
			return
		}
		//
		if clientDetail, ok := ctx.Value("OAuth2ClientDetailsKey").(model.ClientDetails); ok {
			_, err := service.CreateUserDetails(req.Username, req.Password, authorities, clientDetail.ClientId)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "Failed to register user", "detail": err.Error()})
				return
			}
			ctx.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
		} else {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		}
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
			return
		}

		// 加载用户详情（而不是直接通过用户名和密码获取）
		userDetails, err := userDetailsService.LoadUserByUsername(req.Username)
		if err != nil {
			// 避免暴露“用户不存在”的细节，统一返回认证失败
			c.JSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidUsernameAndPasswordRequest.Error()})
			return
		}

		// 手动验证密码
		if !userDetails.CheckPassword(req.Password) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": consts.ErrInvalidUsernameAndPasswordRequest.Error()})
			return
		}

		clientDetails, _ := c.Value(consts.OAuth2ClientDetailsKey).(*model.ClientDetails)

		token, err := tokenService.CreateAccessToken(&model.OAuth2Details{
			User:   userDetails,
			Client: clientDetails,
		})

		respondWithToken(c, token, err)
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

		newAccessToken, err := tokenService.RefreshAccessToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		respondWithToken(c, newAccessToken, err)
	}
}
