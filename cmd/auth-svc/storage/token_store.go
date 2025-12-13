// token_store.go 令牌存储模块
// 主要功能：
// 1. JWT令牌的生成和验证
// 2. 令牌撤销管理
// 3. 令牌存储接口实现
package storage

import (
	. "easyms/cmd/auth-svc/model"
	"easyms/pkg/db"
	"errors"
	"github.com/golang-jwt/jwt"
	"time"
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
	// StoreAccessToken 存储访问令牌
	StoreAccessToken(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details)
	// ReadAccessToken 根据令牌值获取访问令牌结构体
	ReadAccessToken(tokenValue string) (*OAuth2Token, error)
	// ReadOAuth2Details 根据令牌值获取令牌对应的客户端和用户信息
	ReadOAuth2Details(tokenValue string) (*OAuth2Details, error)
	// GetAccessToken 根据客户端信息和用户信息获取访问令牌
	GetAccessToken(oauth2Details *OAuth2Details) (*OAuth2Token, error)
	// RemoveAccessToken 移除存储的访问令牌
	RemoveAccessToken(tokenValue string)
	// StoreRefreshToken 存储刷新令牌
	StoreRefreshToken(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details)
	// RemoveRefreshToken 移除存储的刷新令牌
	RemoveRefreshToken(oauth2Token string)
	// ReadRefreshToken 根据令牌值获取刷新令牌
	ReadRefreshToken(tokenValue string) (*OAuth2Token, error)
	// ReadOAuth2DetailsForRefreshToken 根据令牌值获取刷新令牌对应的客户端和用户信息
	ReadOAuth2DetailsForRefreshToken(tokenValue string) (*OAuth2Details, error)
	// IsAccessTokenRevoked 检查访问令牌是否被撤销
	IsAccessTokenRevoked(tokenValue string) (bool, error)
	// IsRefreshTokenRevoked 检查刷新令牌是否被撤销
	IsRefreshTokenRevoked(tokenValue string) (bool, error)
}

// RevokedToken 令牌撤销记录模型
// 用于存储已撤销的令牌信息
type RevokedToken struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	TokenValue string    `gorm:"uniqueIndex;type:varchar(512)"`
	Expiry     time.Time `gorm:"index"`
	CreatedAt  time.Time
}

// NewJwtTokenStore 创建新的JWT令牌存储实例
// 参数:
//   - jwtTokenEnhancer: JWT令牌增强器
//   - db: 数据库实例
// 返回值:
//   - TokenStore: 令牌存储实例
func NewJwtTokenStore(jwtTokenEnhancer *JwtTokenEnhancer, db *db.EasyDatabase) TokenStore {
	// 自动迁移创建撤销令牌表
	if db != nil {
		err := db.DB.AutoMigrate(&RevokedToken{})
		if err != nil {
			// 如果迁移失败，记录日志但继续执行
			// 在实际应用中应该有更好的错误处理机制
		}
	}

	return &JwtTokenStore{
		jwtTokenEnhancer: jwtTokenEnhancer,
		db:               db,
	}
}

// JwtTokenStore JWT令牌存储实现
type JwtTokenStore struct {
	jwtTokenEnhancer *JwtTokenEnhancer  // JWT令牌增强器
	db               *db.EasyDatabase   // 数据库实例
}

// StoreAccessToken JWT令牌信息存储在令牌本身中，无需额外存储操作
func (tokenStore *JwtTokenStore) StoreAccessToken(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) {
	// JWT令牌是自包含的，不需要额外存储
	// 此方法留空以满足接口要求
}

// ReadAccessToken 根据令牌值获取访问令牌结构体
// 参数:
//   - tokenValue: 令牌值
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

// GetAccessToken 根据客户端信息和用户信息获取访问令牌
// 参数:
//   - oauth2Details: 客户端和用户信息
// 返回值:
//   - *OAuth2Token: 访问令牌结构体
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) GetAccessToken(oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 对于JWT令牌，无法通过客户端和用户信息直接获取访问令牌
	// 因为每个令牌都是唯一的，且存储在客户端
	return nil, ErrNotSupportOperation
}

// RemoveAccessToken 移除存储的访问令牌
// 实现JWT令牌的撤销功能
// 参数:
//   - tokenValue: 令牌值
func (tokenStore *JwtTokenStore) RemoveAccessToken(tokenValue string) {
	// JWT令牌是自包含的，撤销操作需要维护黑名单
	if tokenStore.db != nil {
		// 检查令牌是否已经在黑名单中
		var count int64
		err := tokenStore.db.DB.Model(&RevokedToken{}).Where("token_value = ?", tokenValue).Count(&count).Error

		if err == nil && count == 0 {
			// 令牌不在黑名单中，才进行解析和插入操作
			revokedToken := &RevokedToken{
				TokenValue: tokenValue,
				Expiry:     time.Now().Add(24 * time.Hour), // 默认保留24小时
				CreatedAt:  time.Now(),
			}
			// 插入撤销记录
			tokenStore.db.Insert(revokedToken)
		}
	}
}

// StoreRefreshToken 存储刷新令牌
// 参数:
//   - oauth2Token: OAuth2令牌
//   - oauth2Details: 客户端和用户信息
func (tokenStore *JwtTokenStore) StoreRefreshToken(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) {
	// JWT刷新令牌是自包含的，不需要额外存储
	// 但可以保存一些基本信息用于后续验证
	if tokenStore.db != nil {
		revokedToken := &RevokedToken{
			TokenValue: oauth2Token.TokenValue,
			Expiry:     *oauth2Token.ExpiresTime,
			CreatedAt:  time.Now(),
		}
		// 将刷新令牌信息保存到数据库
		tokenStore.db.Insert(revokedToken)
	}
}

// RemoveRefreshToken 移除存储的刷新令牌
// 参数:
//   - tokenValue: 令牌值
func (tokenStore *JwtTokenStore) RemoveRefreshToken(tokenValue string) {
	// JWT令牌是自包含的，撤销操作需要维护黑名单
	if tokenStore.db != nil {
		// 检查令牌是否已经在黑名单中
		var count int64
		err := tokenStore.db.DB.Model(&RevokedToken{}).Where("token_value = ?", tokenValue).Count(&count).Error

		if err == nil && count == 0 {
			// 令牌不在黑名单中，才进行插入操作
			revokedToken := &RevokedToken{
				TokenValue: tokenValue,
				Expiry:     time.Now().Add(24 * time.Hour), // 默认保留24小时
				CreatedAt:  time.Now(),
			}
			// 插入撤销记录
			tokenStore.db.Insert(revokedToken)
		}
	}
}

// ReadRefreshToken 根据令牌值获取刷新令牌
// 参数:
//   - tokenValue: 令牌值
// 返回值:
//   - *OAuth2Token: 刷新令牌结构体
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) ReadRefreshToken(tokenValue string) (*OAuth2Token, error) {
	// 检查令牌是否被撤销
	revoked, err := tokenStore.IsRefreshTokenRevoked(tokenValue)
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

// ReadOAuth2DetailsForRefreshToken 根据令牌值获取刷新令牌对应的客户端和用户信息
// 参数:
//   - tokenValue: 令牌值
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

// IsAccessTokenRevoked 检查访问令牌是否被撤销
// 参数:
//   - tokenValue: 令牌值
// 返回值:
//   - bool: 撤销返回true，否则返回false
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) IsAccessTokenRevoked(tokenValue string) (bool, error) {
	if tokenStore.db == nil {
		// 没有数据库支持，无法检查撤销状态
		return false, nil
	}

	// 查询数据库检查令牌是否在撤销列表中
	var count int64
	err := tokenStore.db.DB.Model(&RevokedToken{}).Where("token_value = ?", tokenValue).Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// IsRefreshTokenRevoked 检查刷新令牌是否被撤销
// 参数:
//   - tokenValue: 令牌值
// 返回值:
//   - bool: 撤销返回true，否则返回false
//   - error: 操作成功返回nil，失败返回具体错误
func (tokenStore *JwtTokenStore) IsRefreshTokenRevoked(tokenValue string) (bool, error) {
	if tokenStore.db == nil {
		// 没有数据库支持，无法检查撤销状态
		return false, nil
	}

	// 查询数据库检查令牌是否在撤销列表中
	var count int64
	err := tokenStore.db.DB.Model(&RevokedToken{}).Where("token_value = ?", tokenValue).Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
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
	jwt.StandardClaims
}

// JwtTokenEnhancer JWT令牌增强器
type JwtTokenEnhancer struct {
	secretKey []byte  // 签名密钥
}

// NewJwtTokenEnhancer 创建新的JWT令牌增强器
// 参数:
//   - secretKey: 签名密钥
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
// 返回值:
//   - *OAuth2Token: 增强后的OAuth2令牌
//   - error: 操作成功返回nil，失败返回具体错误
func (enhancer *JwtTokenEnhancer) Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	return enhancer.sign(oauth2Token, oauth2Details)
}

// Extract 从 Token 中还原信息
// 参数:
//   - tokenValue: 令牌值
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
			User: &UserDetails{
				Username:    claims.UserDetails.Username,
				Authorities: claims.UserDetails.Authorities,
				// 不返回密码等敏感信息
			},
			Client: &ClientDetails{
				ClientId:                    claims.ClientDetails.ClientId,
				AccessTokenValiditySeconds:  claims.ClientDetails.AccessTokenValiditySeconds,
				RefreshTokenValiditySeconds: claims.ClientDetails.RefreshTokenValiditySeconds,
				RegisteredRedirectUri:       claims.ClientDetails.RegisteredRedirectUri,
				AuthorizedGrantTypes:        claims.ClientDetails.AuthorizedGrantTypes,
				// 不返回客户端密钥
			},
		}

		return oauth2Token, oauth2Details, nil
	}

	return nil, nil, errors.New("invalid token")
}

// sign 签名JWT令牌
// 参数:
//   - oauth2Token: OAuth2令牌
//   - oauth2Details: 客户端和用户信息
// 返回值:
//   - *OAuth2Token: 签名后的OAuth2令牌
//   - error: 操作成功返回nil，失败返回具体错误
func (enhancer *JwtTokenEnhancer) sign(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 获取过期时间
	expireTime := oauth2Token.ExpiresTime
	
	// 复制客户端和用户信息，避免修改原始数据
	clientDetails := *oauth2Details.Client
	userDetails := *oauth2Details.User
	
	// 清除敏感信息
	clientDetails.ClientSecret = ""
	userDetails.Password = ""

	// 构造JWT声明
	claims := OAuth2TokenCustomClaims{
		UserDetails:   userDetails,
		ClientDetails: clientDetails,
		JTI:           oauth2Token.TokenValue, // 使用TokenValue作为JWT ID
		IssuedAt:      time.Now().Unix(),
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