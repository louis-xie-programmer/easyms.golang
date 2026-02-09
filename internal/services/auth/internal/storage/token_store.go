// Package storage 提供了认证服务的数据存储层。
// 它负责处理与令牌 (Token) 和用户凭证等相关的持久化逻辑。
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
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	ErrNotSupportOperation = errors.New("不支持的操作")
	ErrTokenRevoked        = errors.New("令牌已被撤销")
)

// TokenStore 定义了令牌存储的接口。
// 它抽象了令牌的读取和移除操作，使得上层服务不关心底层的存储实现 (例如 JWT, Redis, DB)。
type TokenStore interface {
	// ReadAccessToken 读取并解析一个访问令牌。
	ReadAccessToken(tokenValue string) (*OAuth2Token, error)
	// ReadOAuth2Details 读取并解析令牌，提取其中包含的认证详情。
	ReadOAuth2Details(tokenValue string) (*OAuth2Details, error)
	// RemoveAccessToken 撤销一个访问令牌。
	RemoveAccessToken(tokenValue string)
	// RemoveRefreshToken 撤销一个刷新令牌。
	RemoveRefreshToken(oauth2Token string)
	// IsAccessTokenRevoked 检查一个访问令牌是否已被撤销。
	IsAccessTokenRevoked(tokenValue string) (bool, error)
}

// NewJwtTokenStore 创建一个新的 JwtTokenStore 实例。
// jwtTokenEnhancer: 用于签名和解析 JWT。
// db: 数据库接口，用于持久化撤销的令牌。
// redisClient: Redis 客户端接口，用于缓存撤销的令牌。
func NewJwtTokenStore(jwtTokenEnhancer *JwtTokenEnhancer, db db.Database, redisClient db.RedisClient) TokenStore {
	if db != nil {
		// 自动迁移 RevokedToken 表结构
		if err := db.AutoMigrate(&RevokedToken{}); err != nil {
			logger.Error(err, "自动迁移 RevokedToken 表失败", "auth-svc")
		}
	}
	return &JwtTokenStore{
		jwtTokenEnhancer: jwtTokenEnhancer,
		db:               db,
		redis:            redisClient,
		revokedCache:     cache.New(5*time.Minute, 10*time.Minute), // 本地缓存，过期时间5分钟，每10分钟清理一次
	}
}

// JwtTokenStore 是 TokenStore 接口基于 JWT 的实现。
// 它不直接存储令牌，而是通过 JWT 的自包含特性来传递信息。
// 它只负责存储和检查“已撤销”的令牌列表。
type JwtTokenStore struct {
	jwtTokenEnhancer *JwtTokenEnhancer
	db               db.Database
	redis            db.RedisClient
	revokedCache     *cache.Cache // 用于快速检查的本地内存缓存
}

// ReadAccessToken 首先检查令牌是否被撤销，然后使用 jwtTokenEnhancer 解析令牌。
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

// ReadOAuth2Details 首先检查令牌是否被撤销，然后使用 jwtTokenEnhancer 解析并提取认证详情。
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

// revokeToken 封装了通用的令牌撤销逻辑。
// 它采用“双写”策略，将撤销记录同时写入持久化存储 (DB) 和分布式缓存 (Redis)，并更新本地缓存。
// 这是一个“尽力而为”的操作，即使缓存写入失败，也会尝试写入数据库。
func (tokenStore *JwtTokenStore) revokeToken(tokenValue string, prefix string) {
	oauth2Token, _, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	if err != nil {
		logger.Warn("撤销令牌失败：无法解析令牌", "auth-svc", "error", err)
		return
	}
	expiresIn := time.Until(*oauth2Token.ExpiresTime)
	if expiresIn <= 0 {
		return // 令牌已过期，无需撤销
	}

	hash := getTokenHash(tokenValue)
	cacheKey := fmt.Sprintf("revoked:%s:%s", prefix, hash)

	// 1. 更新本地缓存 (最快)
	tokenStore.revokedCache.Set(cacheKey, true, expiresIn)

	// 2. 更新 Redis (分布式缓存)
	if tokenStore.redis != nil {
		err := tokenStore.redis.SetEx(cacheKey, "revoked", int(expiresIn.Seconds()))
		if err != nil {
			logger.Error(err, "写入 Redis 撤销记录失败", "auth-svc")
			// 即使 Redis 失败，也继续写入数据库
		}
	}

	// 3. 更新数据库 (持久化存储，作为最终数据源)
	if tokenStore.db != nil {
		revokedToken := &RevokedToken{
			TokenHash: hash,
			Expiry:    *oauth2Token.ExpiresTime,
			CreatedAt: time.Now(),
		}
		err := tokenStore.db.Insert(context.Background(), revokedToken)
		if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
			logger.Error(err, "写入数据库撤销记录失败", "auth-svc")
		}
	}
}

// RemoveAccessToken 撤销一个访问令牌。
func (tokenStore *JwtTokenStore) RemoveAccessToken(tokenValue string) {
	tokenStore.revokeToken(tokenValue, "access")
}

// RemoveRefreshToken 撤销一个刷新令牌。
func (tokenStore *JwtTokenStore) RemoveRefreshToken(tokenValue string) {
	tokenStore.revokeToken(tokenValue, "refresh")
}

// isTokenRevoked 封装了检查令牌是否被撤销的通用逻辑。
// 它采用“降级查询”策略：本地缓存 -> Redis -> 数据库。
// 这种多级缓存策略可以有效减少对数据库的直接访问压力。
func (tokenStore *JwtTokenStore) isTokenRevoked(tokenValue, prefix string) (bool, error) {
	hash := getTokenHash(tokenValue)
	cacheKey := fmt.Sprintf("revoked:%s:%s", prefix, hash)

	// 1. 检查本地缓存
	if _, found := tokenStore.revokedCache.Get(cacheKey); found {
		return true, nil
	}

	// 2. 检查 Redis
	if tokenStore.redis != nil {
		val, err := tokenStore.redis.GetCache(cacheKey)
		if err == nil {
			if val != "" {
				// 在 Redis 中找到，更新本地缓存并返回
				tokenStore.revokedCache.Set(cacheKey, true, cache.DefaultExpiration)
				return true, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			// 如果 Redis 出错 (例如连接失败)，记录日志并降级到数据库查询
			logger.Error(err, "从 Redis 检查令牌撤销状态失败", "auth-svc")
		}
		// 如果错误是 redis.Nil，说明 Redis 中不存在该键，将继续查询数据库
	}

	// 3. 检查数据库 (最终数据源)
	if tokenStore.db != nil {
		count, err := tokenStore.db.CountByField(context.Background(), &RevokedToken{}, "token_hash", hash)
		if err != nil {
			logger.Error(err, "从数据库检查令牌撤销状态失败", "auth-svc")
			return false, err
		}
		if count > 0 {
			// 在数据库中找到，更新本地缓存并返回
			tokenStore.revokedCache.Set(cacheKey, true, cache.DefaultExpiration)
			return true, nil
		}
	}

	return false, nil
}

// IsAccessTokenRevoked 检查一个访问令牌是否已被撤销。
func (tokenStore *JwtTokenStore) IsAccessTokenRevoked(tokenValue string) (bool, error) {
	return tokenStore.isTokenRevoked(tokenValue, "access")
}

// getTokenHash 计算令牌的 SHA256 哈希值。
// 存储哈希值而不是原始令牌，可以避免在数据库中暴露敏感信息。
func getTokenHash(tokenValue string) string {
	hash := sha256.Sum256([]byte(tokenValue))
	return hex.EncodeToString(hash[:])
}

// TokenEnhancer 定义了令牌增强器的接口。
// “增强”指的是将认证信息 (OAuth2Details) 编码到令牌中 (例如生成 JWT)，
// “提取”则是从令牌中解码出认证信息。
type TokenEnhancer interface {
	Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error)
	Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error)
}

// OAuth2TokenCustomClaims 是 JWT 的自定义声明结构体。
// 它包含了所有需要嵌入到 JWT 中的业务信息。
type OAuth2TokenCustomClaims struct {
	UserDetails   User          `json:"user_details"`
	ClientDetails ClientDetails `json:"client_details"`
	RefreshToken  OAuth2Token   `json:"refresh_token"`
	GrantType     string        `json:"grant_type"`
	jwt.StandardClaims
}

// JwtTokenEnhancer 是 TokenEnhancer 接口基于 JWT 的实现。
type JwtTokenEnhancer struct {
	secretKey []byte
}

// NewJwtTokenEnhancer 创建一个新的 JwtTokenEnhancer 实例。
func NewJwtTokenEnhancer(secretKey string) TokenEnhancer {
	return &JwtTokenEnhancer{
		secretKey: []byte(secretKey),
	}
}

// Enhance 将认证信息签名并生成一个 JWT 字符串，然后更新到 OAuth2Token 中。
func (enhancer *JwtTokenEnhancer) Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	return enhancer.sign(oauth2Token, oauth2Details)
}

// Extract 从一个 JWT 字符串中解析出声明，并还原为 OAuth2Token 和 OAuth2Details。
func (enhancer *JwtTokenEnhancer) Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error) {
	token, err := jwt.ParseWithClaims(tokenValue, &OAuth2TokenCustomClaims{}, func(token *jwt.Token) (i interface{}, e error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期的签名算法: %v", token.Header["alg"])
		}
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
			return nil, nil, errors.New("令牌已过期")
		}

		oauth2Details := &OAuth2Details{
			Client: &claims.ClientDetails,
			User:   &claims.UserDetails,
		}
		// 确保 User 为 nil 如果用户名为空
		if claims.UserDetails.Username == "" {
			oauth2Details.User = nil
		}

		return oauth2Token, oauth2Details, nil
	}

	return nil, nil, errors.New("无效的令牌")
}

// sign 是一个内部辅助函数，负责创建并签名 JWT。
func (enhancer *JwtTokenEnhancer) sign(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	expireTime := oauth2Token.ExpiresTime
	clientDetails := *oauth2Details.Client
	clientDetails.ClientSecret = "" // 不应将客户端密钥包含在 JWT 中

	var userDetails User
	if oauth2Details.User != nil {
		userDetails = *oauth2Details.User
	}

	grantType := "client_credentials"
	if oauth2Details.User != nil {
		grantType = "password" // 或其他用户授权类型
	}

	claims := OAuth2TokenCustomClaims{
		UserDetails:   userDetails,
		ClientDetails: clientDetails,
		GrantType:     grantType,
		StandardClaims: jwt.StandardClaims{
			Id:        oauth2Token.TokenValue, // JTI
			ExpiresAt: expireTime.Unix(),
			Issuer:    "easyms-auth-svc",
			IssuedAt:  time.Now().Unix(),
		},
	}

	if oauth2Token.RefreshToken != nil {
		claims.RefreshToken = *oauth2Token.RefreshToken
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedTokenValue, err := token.SignedString(enhancer.secretKey)

	if err == nil {
		oauth2Token.TokenValue = signedTokenValue
		oauth2Token.TokenType = "Bearer" // 通常 JWT 使用 Bearer 类型
		return oauth2Token, nil
	}

	return nil, err
}
