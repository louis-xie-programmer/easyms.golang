// token_store.go 令牌存储模块
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	. "easyms/internal/shared/models"

	"github.com/golang-jwt/jwt"
	"github.com/patrickmn/go-cache"
	"github.com/redis/go-redis/v9" // Import go-redis
	"gorm.io/gorm"
)

var (
	ErrNotSupportOperation = errors.New("no support operation")
	ErrTokenRevoked        = errors.New("token is revoked")
)

// TokenStore 令牌存储接口
type TokenStore interface {
	ReadAccessToken(tokenValue string) (*OAuth2Token, error)
	ReadOAuth2Details(tokenValue string) (*OAuth2Details, error)
	RemoveAccessToken(tokenValue string)
	RemoveRefreshToken(oauth2Token string)
	IsAccessTokenRevoked(tokenValue string) (bool, error)
}

// NewJwtTokenStore 创建新的JWT令牌存储实例
func NewJwtTokenStore(jwtTokenEnhancer *JwtTokenEnhancer, db db.Database, redisClient *db.EasyRedis) TokenStore {
	if db != nil {
		if err := db.AutoMigrate(&RevokedToken{}); err != nil {
			logger.Error(err, "Failed to auto-migrate RevokedToken table", "auth-svc")
		}
	}
	return &JwtTokenStore{
		jwtTokenEnhancer: jwtTokenEnhancer,
		db:               db,
		redis:            redisClient,
		revokedCache:     cache.New(5*time.Minute, 10*time.Minute),
	}
}

// JwtTokenStore JWT令牌存储实现
type JwtTokenStore struct {
	jwtTokenEnhancer *JwtTokenEnhancer
	db               db.Database
	redis            *db.EasyRedis
	revokedCache     *cache.Cache
}

// ReadAccessToken 根据令牌值获取访问令牌结构体
func (tokenStore *JwtTokenStore) ReadAccessToken(tokenValue string) (*OAuth2Token, error) {
	revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrTokenRevoked
	}
	oauth2Token, _, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Token, err
}

// ReadOAuth2Details 根据令牌值获取令牌对应的客户端和用户信息
func (tokenStore *JwtTokenStore) ReadOAuth2Details(tokenValue string) (*OAuth2Details, error) {
	revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrTokenRevoked
	}
	_, oauth2Details, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Details, err
}

// revokeToken 封装了通用的令牌撤销逻辑
func (tokenStore *JwtTokenStore) revokeToken(tokenValue string, prefix string) {
	oauth2Token, _, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	if err != nil {
		logger.Warn("Failed to extract token for revocation", "auth-svc", "error", err)
		return
	}
	expiresIn := time.Until(*oauth2Token.ExpiresTime)
	if expiresIn <= 0 {
		return
	}

	hash := getTokenHash(tokenValue)
	cacheKey := fmt.Sprintf("revoked:%s:%s", prefix, hash)

	tokenStore.revokedCache.Set(cacheKey, true, expiresIn)

	if tokenStore.redis != nil {
		err := tokenStore.redis.SetEx(cacheKey, "revoked", int(expiresIn.Seconds()))
		if err != nil {
			logger.Error(err, "redis setex failed for token revocation", "auth-svc")
		}
		return
	}

	if tokenStore.db != nil {
		revokedToken := &RevokedToken{
			TokenHash: hash, // Corrected: Store hash instead of full token
			Expiry:    *oauth2Token.ExpiresTime,
			CreatedAt: time.Now(),
		}
		err := tokenStore.db.Insert(context.Background(), revokedToken)
		if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
			logger.Error(err, "insert into revoked_tokens table failed", "auth-svc")
		}
	}
}

func (tokenStore *JwtTokenStore) RemoveAccessToken(tokenValue string) {
	tokenStore.revokeToken(tokenValue, "access")
}

func (tokenStore *JwtTokenStore) RemoveRefreshToken(tokenValue string) {
	tokenStore.revokeToken(tokenValue, "refresh")
}

// isTokenRevoked 封装了通用的检查令牌是否被撤销的逻辑
func (tokenStore *JwtTokenStore) isTokenRevoked(tokenValue, prefix string) (bool, error) {
	hash := getTokenHash(tokenValue)
	cacheKey := fmt.Sprintf("revoked:%s:%s", prefix, hash)

	if _, found := tokenStore.revokedCache.Get(cacheKey); found {
		return true, nil
	}

	if tokenStore.redis != nil {
		val, err := tokenStore.redis.GetCache(cacheKey)
		// Corrected: Check for redis.Nil error
		if err != nil && !errors.Is(err, redis.Nil) {
			logger.Error(err, "redis get failed for checking token revocation", "auth-svc")
		} else if err == nil && val != "" {
			tokenStore.revokedCache.Set(cacheKey, true, cache.DefaultExpiration)
			return true, nil
		}
	}

	if tokenStore.db != nil {
		var count int64
		// Corrected: Query by token_hash
		err := tokenStore.db.GetDB().Model(&RevokedToken{}).Where("token_hash = ?", hash).Count(&count).Error
		if err != nil {
			logger.Error(err, "db query failed for checking token revocation", "auth-svc")
			return false, err
		}
		if count > 0 {
			tokenStore.revokedCache.Set(cacheKey, true, cache.DefaultExpiration)
			return true, nil
		}
	}

	return false, nil
}

func (tokenStore *JwtTokenStore) IsAccessTokenRevoked(tokenValue string) (bool, error) {
	return tokenStore.isTokenRevoked(tokenValue, "access")
}

// getTokenHash computes the full SHA256 hash of a token.
func getTokenHash(tokenValue string) string {
	hash := sha256.Sum256([]byte(tokenValue))
	return hex.EncodeToString(hash[:]) // Corrected: Use the full hash
}

// ... (The rest of the file remains the same)
// TokenEnhancer, OAuth2TokenCustomClaims, JwtTokenEnhancer, etc.
type TokenEnhancer interface {
	Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error)
	Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error)
}

type OAuth2TokenCustomClaims struct {
	UserDetails   User
	ClientDetails ClientDetails
	RefreshToken  OAuth2Token
	JTI           string
	IssuedAt      int64
	GrantType     string
	jwt.StandardClaims
}

type JwtTokenEnhancer struct {
	secretKey []byte
}

func NewJwtTokenEnhancer(secretKey string) TokenEnhancer {
	return &JwtTokenEnhancer{
		secretKey: []byte(secretKey),
	}
}

func (enhancer *JwtTokenEnhancer) Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	return enhancer.sign(oauth2Token, oauth2Details)
}

func (enhancer *JwtTokenEnhancer) Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error) {
	token, err := jwt.ParseWithClaims(tokenValue, &OAuth2TokenCustomClaims{}, func(token *jwt.Token) (i interface{}, e error) {
		return enhancer.secretKey, nil
	})

	if err != nil {
		return nil, nil, err
	}

	if claims, ok := token.Claims.(*OAuth2TokenCustomClaims); ok && token.Valid {
		expiresTime := time.Unix(claims.ExpiresAt, 0)
		oauth2Token := &OAuth2Token{
			RefreshToken: &claims.RefreshToken,
			TokenValue:   tokenValue,
			ExpiresTime:  &expiresTime,
		}

		if oauth2Token.IsExpired() {
			return nil, nil, errors.New("token is expired")
		}

		oauth2Details := &OAuth2Details{
			Client: &ClientDetails{
				ClientId:                    claims.ClientDetails.ClientId,
				AccessTokenValiditySeconds:  claims.ClientDetails.AccessTokenValiditySeconds,
				RefreshTokenValiditySeconds: claims.ClientDetails.RefreshTokenValiditySeconds,
				RegisteredRedirectUri:       claims.ClientDetails.RegisteredRedirectUri,
				AuthorizedGrantTypes:        claims.ClientDetails.AuthorizedGrantTypes,
				AllowedAuthorities:          claims.ClientDetails.AllowedAuthorities,
				DefaultAuthorities:          claims.ClientDetails.DefaultAuthorities,
			},
		}

		if claims.UserDetails.Username != "" || claims.UserDetails.Authorities != "" {
			oauth2Details.User = &User{
				Username:    claims.UserDetails.Username,
				Authorities: claims.UserDetails.Authorities,
			}
		}

		return oauth2Token, oauth2Details, nil
	}

	return nil, nil, errors.New("invalid token")
}

func (enhancer *JwtTokenEnhancer) sign(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	expireTime := oauth2Token.ExpiresTime
	clientDetails := *oauth2Details.Client
	clientDetails.ClientSecret = ""

	var userDetails User
	if oauth2Details.User != nil {
		userDetails = *oauth2Details.User
	}

	grantType := "client_credentials"
	if oauth2Details.User != nil {
		grantType = "user_grant"
	}

	claims := OAuth2TokenCustomClaims{
		UserDetails:   userDetails,
		ClientDetails: clientDetails,
		JTI:           oauth2Token.TokenValue,
		IssuedAt:      time.Now().Unix(),
		GrantType:     grantType,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireTime.Unix(),
			Issuer:    "System",
		},
	}

	if oauth2Token.RefreshToken != nil {
		claims.RefreshToken = *oauth2Token.RefreshToken
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenValue, err := token.SignedString(enhancer.secretKey)

	if err == nil {
		oauth2Token.TokenValue = tokenValue
		oauth2Token.TokenType = "jwt"
		return oauth2Token, nil
	}

	return nil, err
}
