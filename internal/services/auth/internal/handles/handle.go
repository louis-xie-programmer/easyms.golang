package handles

import (
	"easyms/internal/services/auth/internal/consts"
	"easyms/internal/services/auth/internal/service"
	. "easyms/internal/shared/models"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// handleServiceError maps service layer errors to appropriate HTTP status codes and responses.
func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrTokenExpired):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token_expired", "message": err.Error()})
	case errors.Is(err, service.ErrInvalidToken), errors.Is(err, consts.ErrInvalidClient), errors.Is(err, consts.ErrInvalidUsernameAndPasswordRequest):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": err.Error()})
	case errors.Is(err, service.ErrInternalServer):
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error", "message": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
	}
}

// respondWithToken encapsulates the logic for sending a token response.
func respondWithToken(c *gin.Context, token *OAuth2Token, err error) {
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  token,
		RefreshToken: token.RefreshToken,
	})
}

// MakeTokenEndpoint creates a handler for the client credentials token endpoint.
func MakeTokenEndpoint(svc service.TokenGranter, clientdetailsService service.ClientDetailsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ClientTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
			return
		}

		clientDetails, err := clientdetailsService.LoadClientByClientId(c, req.ClientId)
		if err != nil || clientDetails == nil || clientDetails.ClientSecret != req.ClientSecret {
			handleServiceError(c, consts.ErrInvalidClient)
			return
		}

		token, err := svc.Grant(c, req.GrantType, clientDetails, &TokenRequest{GrantType: req.GrantType})
		respondWithToken(c, token, err)
	}
}

func VerifyTokenEndpoint(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_token"})
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		_, err := tokenService.GetOAuth2DetailsByAccessToken(c, tokenStr)
		if err != nil {
			handleServiceError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"valid": true})
	}
}

func RegisterClientEndPoint(service service.ClientDetailsService) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req RegisterClientRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
			return
		}

		clientDetails, err := service.CreateClientDetails(ctx, req.ClientId)
		if err != nil {
			handleServiceError(ctx, err)
			return
		}

		ctx.JSON(http.StatusOK, RegisterClientResponse{
			ClientId:     clientDetails.ClientId,
			ClientSecret: clientDetails.ClientSecret,
		})
	}
}

func RegisterUserEndPoint(service service.UserDetailsService, authorities []string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req RegisterUserRequest
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
			return
		}

		val, ok := ctx.Get(consts.OAuth2ClientDetailsKey)
		if !ok {
			handleServiceError(ctx, errors.New("client details not found in context"))
			return
		}
		clientDetail, ok := val.(*ClientDetails)
		if !ok {
			handleServiceError(ctx, errors.New("client details in context have wrong type"))
			return
		}

		_, err := service.CreateUserDetails(ctx, req.Username, req.Password, authorities, clientDetail.ClientId)
		if err != nil {
			handleServiceError(ctx, err)
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
	}
}

func LoginEndPoint(userDetailsService service.UserDetailsService, tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
			return
		}

		userDetails, err := userDetailsService.LoadUserByUsername(c, req.Username)
		if err != nil || !userDetails.CheckPassword(req.Password) {
			handleServiceError(c, consts.ErrInvalidUsernameAndPasswordRequest)
			return
		}

		val, ok := c.Get(consts.OAuth2ClientDetailsKey)
		if !ok {
			handleServiceError(c, errors.New("client details not found in context"))
			return
		}
		clientDetails, ok := val.(*ClientDetails)
		if !ok {
			handleServiceError(c, errors.New("client details in context have wrong type"))
			return
		}

		token, err := tokenService.CreateAccessToken(c, &OAuth2Details{
			User:   userDetails,
			Client: clientDetails,
		})
		respondWithToken(c, token, err)
	}
}

func RefreshTokenEndpoint(tokenService service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RefreshTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "validation_failed", "message": err.Error()})
			return
		}

		newAccessToken, err := tokenService.RefreshAccessToken(c, req.RefreshToken)
		respondWithToken(c, newAccessToken, err)
	}
}
