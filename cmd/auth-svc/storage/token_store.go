package storage

import (
	"errors"
	"github.com/golang-jwt/jwt"
	. "github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
	"time"
)

var ErrNotSupportOperation = errors.New("no support operation")

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
}

func NewJwtTokenStore(jwtTokenEnhancer *JwtTokenEnhancer) TokenStore {
	return &JwtTokenStore{
		jwtTokenEnhancer: jwtTokenEnhancer,
	}
}

type JwtTokenStore struct {
	jwtTokenEnhancer *JwtTokenEnhancer
}

func (tokenStore *JwtTokenStore) StoreAccessToken(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) {

}

func (tokenStore *JwtTokenStore) ReadAccessToken(tokenValue string) (*OAuth2Token, error) {
	oauth2Token, _, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Token, err

}

// ReadOAuth2Details 根据令牌值获取令牌对应的客户端和用户信息
func (tokenStore *JwtTokenStore) ReadOAuth2Details(tokenValue string) (*OAuth2Details, error) {
	_, oauth2Details, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Details, err

}

// GetAccessToken 根据客户端信息和用户信息获取访问令牌
func (tokenStore *JwtTokenStore) GetAccessToken(oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	return nil, ErrNotSupportOperation
}

// RemoveAccessToken 移除存储的访问令牌
func (tokenStore *JwtTokenStore) RemoveAccessToken(tokenValue string) {

}

// StoreRefreshToken 存储刷新令牌
func (tokenStore *JwtTokenStore) StoreRefreshToken(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) {

}

// RemoveRefreshToken 移除存储的刷新令牌
func (tokenStore *JwtTokenStore) RemoveRefreshToken(oauth2Token string) {

}

// ReadRefreshToken 根据令牌值获取刷新令牌
func (tokenStore *JwtTokenStore) ReadRefreshToken(tokenValue string) (*OAuth2Token, error) {
	oauth2Token, _, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Token, err
}

// ReadOAuth2DetailsForRefreshToken 根据令牌值获取刷新令牌对应的客户端和用户信息
func (tokenStore *JwtTokenStore) ReadOAuth2DetailsForRefreshToken(tokenValue string) (*OAuth2Details, error) {
	_, oauth2Details, err := tokenStore.jwtTokenEnhancer.Extract(tokenValue)
	return oauth2Details, err
}

type TokenEnhancer interface {
	// Enhance 组装 Token 信息
	Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error)
	// Extract 从 Token 中还原信息
	Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error)
}

// OAuth2TokenCustomClaims Token 信息
type OAuth2TokenCustomClaims struct {
	UserDetails   UserDetails
	ClientDetails ClientDetails
	RefreshToken  OAuth2Token
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

// Enhance 组装 Token 信息
func (enhancer *JwtTokenEnhancer) Enhance(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	return enhancer.sign(oauth2Token, oauth2Details)
}

// Extract 从 Token 中还原信息
func (enhancer *JwtTokenEnhancer) Extract(tokenValue string) (*OAuth2Token, *OAuth2Details, error) {
	token, err := jwt.ParseWithClaims(tokenValue, &OAuth2TokenCustomClaims{}, func(token *jwt.Token) (i interface{}, e error) {
		return enhancer.secretKey, nil
	})

	if err == nil {

		claims := token.Claims.(*OAuth2TokenCustomClaims)
		expiresTime := time.Unix(claims.ExpiresAt, 0)

		return &OAuth2Token{
				RefreshToken: &claims.RefreshToken,
				TokenValue:   tokenValue,
				ExpiresTime:  &expiresTime,
			}, &OAuth2Details{
				User:   &claims.UserDetails,
				Client: &claims.ClientDetails,
			}, nil

	}
	return nil, nil, err
}

// sign 签名
func (enhancer *JwtTokenEnhancer) sign(oauth2Token *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {

	expireTime := oauth2Token.ExpiresTime
	clientDetails := *oauth2Details.Client
	userDetails := *oauth2Details.User
	clientDetails.ClientSecret = ""
	userDetails.Password = ""

	claims := OAuth2TokenCustomClaims{
		UserDetails:   userDetails,
		ClientDetails: clientDetails,
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
