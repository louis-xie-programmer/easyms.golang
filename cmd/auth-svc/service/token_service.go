package service

import (
	. "easyms/cmd/auth-svc/model"
	"easyms/cmd/auth-svc/storage"
	"errors"
	"github.com/satori/go.uuid"
	"strconv"
	"time"
)

var (
	// ErrNotSupportGrantType               授权类型不支持错误
	ErrNotSupportGrantType = errors.New("grant type is not supported")
	// ErrInvalidUsernameAndPasswordRequest 用户名密码错误错误
	ErrInvalidUsernameAndPasswordRequest = errors.New("invalid username, password")
	// ErrInvalidTokenRequest               令牌错误错误
	ErrInvalidTokenRequest = errors.New("invalid token")
	// ErrExpiredToken                       令牌已过期错误
	ErrExpiredToken = errors.New("token is expired")
)

type TokenService interface {
	// GetOAuth2DetailsByAccessToken 根据访问令牌获取对应的用户信息和客户端信息
	GetOAuth2DetailsByAccessToken(tokenValue string) (*OAuth2Details, error)
	// CreateAccessToken 根据用户信息和客户端信息生成访问令牌
	CreateAccessToken(oauth2Details *OAuth2Details) (*OAuth2Token, error)
	// RefreshAccessToken 根据刷新令牌获取访问令牌
	RefreshAccessToken(refreshTokenValue string) (*OAuth2Token, error)
	// GetAccessToken 根据用户信息和客户端信息获取已生成访问令牌
	GetAccessToken(details *OAuth2Details) (*OAuth2Token, error)
	// ReadAccessToken 根据访问令牌值获取访问令牌结构体
	ReadAccessToken(tokenValue string) (*OAuth2Token, error)
}

type DefaultTokenService struct {
	tokenStore    storage.TokenStore    // 令牌存储器
	tokenEnhancer storage.TokenEnhancer // 令牌增强器
}

func NewTokenService(tokenStore storage.TokenStore, tokenEnhancer storage.TokenEnhancer) TokenService {
	return &DefaultTokenService{
		tokenStore:    tokenStore,
		tokenEnhancer: tokenEnhancer,
	}
}

func (tokenService *DefaultTokenService) CreateAccessToken(oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 判断是否存在未失效的访问令牌
	existToken, err := tokenService.tokenStore.GetAccessToken(oauth2Details)
	var refreshToken *OAuth2Token
	if err == nil {
		// 存在未失效访问令牌，直接返回
		if !existToken.IsExpired() {
			tokenService.tokenStore.StoreAccessToken(existToken, oauth2Details)
			return existToken, nil
		}
		// 访问令牌已失效，移除
		tokenService.tokenStore.RemoveAccessToken(existToken.TokenValue)
		if existToken.RefreshToken != nil {
			refreshToken = existToken.RefreshToken
			tokenService.tokenStore.RemoveRefreshToken(refreshToken.TokenType)
		}
	}
	// 不存在 生成刷新令牌
	if refreshToken == nil || refreshToken.IsExpired() {
		refreshToken, err = tokenService.createRefreshToken(oauth2Details)
		if err != nil {
			return nil, err
		}
	}

	// 生成新的访问令牌
	accessToken, err := tokenService.createAccessToken(refreshToken, oauth2Details)
	if err == nil {
		// 保存新生成令牌
		tokenService.tokenStore.StoreAccessToken(accessToken, oauth2Details)
		tokenService.tokenStore.StoreRefreshToken(refreshToken, oauth2Details)
	}
	return accessToken, err
}

func (tokenService *DefaultTokenService) createAccessToken(refreshToken *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 生成访问令牌
	validitySeconds := oauth2Details.Client.AccessTokenValiditySeconds
	s, _ := time.ParseDuration(strconv.Itoa(validitySeconds) + "s")
	expiredTime := time.Now().Add(s)
	accessToken := &OAuth2Token{
		RefreshToken: refreshToken,
		ExpiresTime:  &expiredTime,
		TokenValue:   uuid.NewV4().String(),
	}

	if tokenService.tokenEnhancer != nil {
		return tokenService.tokenEnhancer.Enhance(accessToken, oauth2Details)
	}
	return accessToken, nil
}

func (tokenService *DefaultTokenService) createRefreshToken(oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	// 生成刷新令牌
	validitySeconds := oauth2Details.Client.RefreshTokenValiditySeconds
	s, _ := time.ParseDuration(strconv.Itoa(validitySeconds) + "s")
	expiredTime := time.Now().Add(s)
	refreshToken := &OAuth2Token{
		ExpiresTime: &expiredTime,
		TokenValue:  uuid.NewV4().String(),
	}

	if tokenService.tokenEnhancer != nil {
		return tokenService.tokenEnhancer.Enhance(refreshToken, oauth2Details)
	}
	return refreshToken, nil
}

func (tokenService *DefaultTokenService) RefreshAccessToken(refreshTokenValue string) (*OAuth2Token, error) {
	refreshToken, err := tokenService.tokenStore.ReadRefreshToken(refreshTokenValue)
	if err == nil {
		// 判断刷新令牌是否已过期
		if refreshToken.IsExpired() {
			return nil, ErrExpiredToken
		}
		// 读取刷新令牌对应的用户信息和客户端信息
		oauth2Details, err := tokenService.tokenStore.ReadOAuth2DetailsForRefreshToken(refreshTokenValue)
		if err == nil {
			// 获取访问令牌对应的用户信息和客户端信息
			oauth2Token, err := tokenService.tokenStore.GetAccessToken(oauth2Details)
			// 移除原有的访问令牌
			if err == nil {
				tokenService.tokenStore.RemoveAccessToken(oauth2Token.TokenValue)
			}
			// 移除已使用的刷新令牌
			tokenService.tokenStore.RemoveRefreshToken(refreshTokenValue)
			refreshToken, err = tokenService.createRefreshToken(oauth2Details)
			if err == nil {
				accessToken, err := tokenService.createAccessToken(refreshToken, oauth2Details)
				if err == nil {
					tokenService.tokenStore.StoreAccessToken(accessToken, oauth2Details)
					tokenService.tokenStore.StoreRefreshToken(refreshToken, oauth2Details)
				}
				return accessToken, err
			}
		}
	}
	return nil, err
}

func (tokenService *DefaultTokenService) GetAccessToken(details *OAuth2Details) (*OAuth2Token, error) {
	return tokenService.tokenStore.GetAccessToken(details)
}

func (tokenService *DefaultTokenService) ReadAccessToken(tokenValue string) (*OAuth2Token, error) {
	return tokenService.tokenStore.ReadAccessToken(tokenValue)
}

func (tokenService *DefaultTokenService) GetOAuth2DetailsByAccessToken(tokenValue string) (*OAuth2Details, error) {
	accessToken, err := tokenService.tokenStore.ReadAccessToken(tokenValue)
	if err == nil {
		if accessToken.IsExpired() {
			return nil, ErrExpiredToken
		}
		return tokenService.tokenStore.ReadOAuth2Details(tokenValue)
	}
	return nil, err
}
