// token_store.go 令牌存储模块
// 主要功能：
// 1. JWT令牌的生成和验证
// 2. 令牌撤销管理
// 3. 令牌存储接口实现
package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	. "easyms/internal/shared/models"
	"errors"
	"time"

	"github.com/golang-jwt/jwt"
	"gorm.io/gorm"
)

var (
	// ErrNotSupportOperation 操作不支持错误
	ErrNotSupportOperation = errors.New("no support operation")
	// ErrTokenRevoked 令牌被撤销错误
	ErrTokenRevoked = errors.New("token is revoked")
)

// TokenStore 令牌存储接口
// 定义了令牌管理的标准方法
type TokenStore interface {
	// ReadAccessToken 根据令牌值获取访问令牌结构体
	ReadAccessToken(tokenValue string) (*OAuth2Token, error)
	// ReadOAuth2Details 根据令牌值获取令牌对应的客户端和用户信息
	ReadOAuth2Details(tokenValue string) (*OAuth2Details, error)

	// RemoveAccessToken 移除存储的访问令牌
	RemoveAccessToken(tokenValue string)
	// RemoveRefreshToken 移除存储的刷新令牌
	RemoveRefreshToken(oauth2Token string)

	// IsAccessTokenRevoked 检查访问令牌是否被撤销
	IsAccessTokenRevoked(tokenValue string) (bool, error)
}

// NewJwtTokenStore 创建新的JWT令牌存储实例,
// 当前版本令牌存储在JWT令牌中，无需额外存储操作
// 参数:
//   - jwtTokenEnhancer: JWT令牌增强器
//   - db: 数据库实例（保留用于兼容）
//   - redisClient: Redis客户端（用于高性能令牌撤销）
//
// 返回值:
//   - TokenStore: 令牌存储实例
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
	}
}

// JwtTokenStore JWT令牌存储实现
type JwtTokenStore struct {
	jwtTokenEnhancer *JwtTokenEnhancer // JWT令牌增强器
	db               db.Database       // 数据库实例（保留用于兼容）
	redis            *db.EasyRedis     // Redis客户端，用于高性能令牌撤销
}

// ReadAccessToken 根据令牌值获取访问令牌结构体
// 参数:
//   - tokenValue: 令牌值
//
// 返回值:
//   - *OAuth2Token: 访问令牌结构体
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) ReadAccessToken(tokenValue string) (*OAuth2Token, error) {
	// 检查令牌是否被撤销
	revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrTokenRevoked
	}

	// 从JWT令牌中提取信息
	oauth2Token, _, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Token, err
}

// ReadOAuth2Details 根据令牌值获取令牌对应的客户端和用户信息
// 参数:
//   - tokenValue: 令牌值
//
// 返回值:
//   - *OAuth2Details: 客户端和用户信息
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) ReadOAuth2Details(tokenValue string) (*OAuth2Details, error) {
	// 检查令牌是否被撤销
	revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrTokenRevoked
	}

	// 从JWT令牌中提取信息
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
	// 计算剩余有效期
	expiresIn := time.Until(*oauth2Token.ExpiresTime)
	if expiresIn <= 0 {
		return // 令牌已过期，无需撤销
	}

	// 优先使用Redis
	if tokenStore.redis != nil {
		hash := getTokenHash(tokenValue)
		err := tokenStore.redis.SetEx(
			fmt.Sprintf("revoked:%s:%s", prefix, hash),
			"revoked",
			int(expiresIn.Seconds()),
		)
		if err != nil {
			logger.Error(err, "redis setex failed for token revocation", "auth-svc")
		}
		return
	}

	// 降级到数据库
	if tokenStore.db != nil {
		revokedToken := &RevokedToken{
			TokenValue: tokenValue,
			Expiry:     *oauth2Token.ExpiresTime,
			CreatedAt:  time.Now(),
		}
		// 直接插入，忽略唯一键冲突错误
		err := tokenStore.db.Insert(revokedToken)
		if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
			logger.Error(err, "insert into revoked_tokens table failed", "auth-svc")
		}
	}
}

// RemoveAccessToken 撤销访问令牌
func (tokenStore *JwtTokenStore) RemoveAccessToken(tokenValue string) {
	tokenStore.revokeToken(tokenValue, "access")
}

// RemoveRefreshToken 撤销刷新令牌
func (tokenStore *JwtTokenStore) RemoveRefreshToken(tokenValue string) {
	tokenStore.revokeToken(tokenValue, "refresh")
}

// ReadOAuth2DetailsForRefreshToken 根据令牌值获取刷新令牌对应的客户端和用户信息
// 参数:
//   - tokenValue: 令牌值
//
// 返回值:
//   - *OAuth2Details: 客户端和用户信息
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) ReadOAuth2DetailsForRefreshToken(tokenValue string) (*OAuth2Details, error) {
	// 检查令牌是否被撤销
	revoked, err := tokenStore.IsRefreshTokenRevoked(tokenValue)
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrTokenRevoked
	}

	// 从JWT令牌中提取信息
	_, oauth2Details, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Details, err
}

// isTokenRevoked 封装了通用的检查令牌是否被撤销的逻辑
func (tokenStore *JwtTokenStore) isTokenRevoked(tokenValue, prefix string) (bool, error) {
	// 优先使用Redis
	if tokenStore.redis != nil {
		hash := getTokenHash(tokenValue)
		val, err := tokenStore.redis.GetCache(fmt.Sprintf("revoked:%s:%s", prefix, hash))
		if err != nil && !errors.Is(err, sql.ErrNoRows) { // 假设redis client在未找到时返回类似sql.ErrNoRows的错误
			logger.Error(err, "redis get failed for checking token revocation", "auth-svc")
			// Redis 故障，降级到数据库
		} else if err == nil && val != "" {
			return true, nil
		} else {
			return false, nil // Redis中不存在
		}
	}

	// 降级到数据库
	if tokenStore.db != nil {
		var count int64
		err := tokenStore.db.GetDB().Model(&RevokedToken{}).Where("token_value = ?", tokenValue).Count(&count).Error
		if err != nil {
			logger.Error(err, "db query failed for checking token revocation", "auth-svc")
			return false, err
		}
		return count > 0, nil
	}

	return false, nil
}

// IsAccessTokenRevoked 检查访问令牌是否被撤销
func (tokenStore *JwtTokenStore) IsAccessTokenRevoked(tokenValue string) (bool, error) {
	return tokenStore.isTokenRevoked(tokenValue, "access")
}

// IsRefreshTokenRevoked 检查刷新令牌是否被撤销
func (tokenStore *JwtTokenStore) IsRefreshTokenRevoked(tokenValue string) (bool, error) {
	return tokenStore.isTokenRevoked(tokenValue, "refresh")
}

// TokenEnhancer 令牌增强器接口
// 定义了令牌增强的标准方法
type TokenEnhancer interface {
	// Enhance 组装 Token 信息
	Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error)
	// Extract 从 Token 中还原信息
	Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error)
}

// OAuth2TokenCustomClaims JWT令牌自定义声明
// 包含OAuth2相关的令牌信息
type OAuth2TokenCustomClaims struct {
	UserDetails   UserDetails
	ClientDetails ClientDetails
	RefreshToken  OAuth2Token
	JTI           string // JWT ID，用于防重放攻击
	IssuedAt      int64  // 签发时间，用于防重放攻击
	GrantType     string // 授权类型，例如"password"、"refresh_token"等
	jwt.StandardClaims
}

// JwtTokenEnhancer JWT令牌增强器
type JwtTokenEnhancer struct {
	secretKey []byte // 签名密钥
}

// NewJwtTokenEnhancer 创建新的JWT令牌增强器
// 参数:
//   - secretKey: 签名密钥
//
// 返回值:
//   - TokenEnhancer: 令牌增强器实例
func NewJwtTokenEnhancer(secretKey string) TokenEnhancer {
	return &JwtTokenEnhancer{
		secretKey: []byte(secretKey),
	}
}

// Enhance 组装 Token 信息
// 参数:
//   - oauth2Token: OAuth2令牌
//   - oauth2Details: 客户端和用户信息
//
// 返回值:
//   - *OAuth2Token: 增强后的OAuth2令牌
//   - error: 操作成功返回nil，失败返回具体错误
func (enhancer *JwtTokenEnhancer) Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	return enhancer.sign(oauth2Token, oauth2Details)
}

// Extract 从 Token 中还原信息
// 参数:
//   - tokenValue: 令牌值
//
// 返回值:
//   - *OAuth2Token: OAuth2令牌
//   - *OAuth2Details: 客户端和用户信息
//   - error: 操作成功返回nil，失败返回具体错误
func (enhancer *JwtTokenEnhancer) Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error) {
	// 解析令牌并验证签名
	token, err := jwt.ParseWithClaims(tokenValue, &OAuth2TokenCustomClaims{}, func(token *jwt.Token) (i interface{}, e error) {
		return enhancer.secretKey, nil
	})

	if err != nil {
		return nil, nil, err
	}

	// 检查令牌是否有效
	if claims, ok := token.Claims.(*OAuth2TokenCustomClaims); ok && token.Valid {
		// 解析过期时间
		expiresTime := time.Unix(claims.ExpiresAt, 0)

		// 创建OAuth2令牌对象
		oauth2Token := &OAuth2Token{
			RefreshToken: &claims.RefreshToken,
			TokenValue:   tokenValue,
			ExpiresTime:  &expiresTime,
		}

		// 检查是否过期
		if oauth2Token.IsExpired() {
			return nil, nil, errors.New("token is expired")
		}

		// 创建OAuth2Details对象，注意不包含敏感信息
		oauth2Details := &OAuth2Details{
			Client: &ClientDetails{
				ClientId:                    claims.ClientDetails.ClientId,
				AccessTokenValiditySeconds:  claims.ClientDetails.AccessTokenValiditySeconds,
				RefreshTokenValiditySeconds: claims.ClientDetails.RefreshTokenValiditySeconds,
				RegisteredRedirectUri:       claims.ClientDetails.RegisteredRedirectUri,
				AuthorizedGrantTypes:        claims.ClientDetails.AuthorizedGrantTypes,
				AllowedAuthorities:          claims.ClientDetails.AllowedAuthorities,
				DefaultAuthorities:          claims.ClientDetails.DefaultAuthorities,
				// 不返回客户端密钥
			},
		}

		// 只有当用户信息存在且不为空时才设置UserDetails
		if claims.UserDetails.Username != "" || claims.UserDetails.Authorities != "" {
			oauth2Details.User = &UserDetails{
				Username:    claims.UserDetails.Username,
				Authorities: claims.UserDetails.Authorities,
				// 不返回密码等敏感信息
			}
		}

		return oauth2Token, oauth2Details, nil
	}

	return nil, nil, errors.New("invalid token")
}

// sign 签名JWT令牌
// 参数:
//   - oauth2Token: OAuth2令牌
//   - oauth2Details: 客户端和用户信息
//
// 返回值:
//   - *OAuth2Token: 签名后的OAuth2令牌
//   - error: 操作成功返回nil，失败返回具体错误
func getTokenHash(tokenValue string) string {
	hash := sha256.Sum256([]byte(tokenValue))
	return hex.EncodeToString(hash[:4])
}

func (enhancer *JwtTokenEnhancer) sign(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 获取过期时间
	expireTime := oauth2Token.ExpiresTime

	// 复制客户端和用户信息，避免修改原始数据
	clientDetails := *oauth2Details.Client

	// 清除敏感信息
	clientDetails.ClientSecret = ""

	// 处理用户信息，添加空值检查
	var userDetails UserDetails
	if oauth2Details.User != nil {
		userDetails = *oauth2Details.User
		// 清除用户敏感信息
		userDetails.Password = ""
	}

	// 如果用户为空，则为客户端凭证授权
	grantType := "client_credentials"
	if oauth2Details.User != nil {
		grantType = "user_grant" // 或其他适当的标识
	}

	// 构造JWT声明
	claims := OAuth2TokenCustomClaims{
		UserDetails:   userDetails,
		ClientDetails: clientDetails,
		JTI:           oauth2Token.TokenValue, // 使用TokenValue作为JWT ID
		IssuedAt:      time.Now().Unix(),
		GrantType:     grantType,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireTime.Unix(),
			Issuer:    "System",
		},
	}

	// 如果有刷新令牌，也包含在声明中
	if oauth2Token.RefreshToken != nil {
		claims.RefreshToken = *oauth2Token.RefreshToken
	}

	// 创建JWT令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名令牌
	tokenValue, err := token.SignedString(enhancer.secretKey)

	if err == nil {
		// 更新令牌值和类型
		oauth2Token.TokenValue = tokenValue
		oauth2Token.TokenType = "jwt"
		return oauth2Token, nil
	}

	return nil, err
}
