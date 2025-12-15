package service

import (
	"easyms/cmd/auth-svc/consts"
	. "easyms/cmd/auth-svc/model"
	"easyms/cmd/auth-svc/storage"
	"github.com/satori/go.uuid"
	"strconv"
	"time"
)

type TokenService interface {
	// GetOAuth2DetailsByAccessToken 根据访问令牌获取对应的用户信息和客户端信息
	GetOAuth2DetailsByAccessToken(tokenValue string) (*OAuth2Details, error)
	// CreateAccessToken 根据用户信息和客户端信息生成访问令牌
	CreateAccessToken(oauth2Details *OAuth2Details) (*OAuth2Token, error)
	// RefreshAccessToken 根据刷新令牌获取访问令牌
	RefreshAccessToken(refreshTokenValue string) (*OAuth2Token, error)
	ReadAccessToken(tokenValue string) (*OAuth2Token, error)
	// ReadOAuth2Details 根据访问令牌值获取OAuth2Details信息
	ReadOAuth2Details(tokenValue string) (*OAuth2Details, error)
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
	var refreshToken *OAuth2Token
	refreshToken, err := tokenService.createRefreshToken(oauth2Details)
	// 生成新的访问令牌
	accessToken, err := tokenService.createAccessToken(refreshToken, oauth2Details)
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
			return nil, consts.ErrExpiredToken
		}
		// 读取刷新令牌对应的用户信息和客户端信息
		oauth2Details, err := tokenService.tokenStore.ReadOAuth2DetailsForRefreshToken(refreshTokenValue)
		if err == nil {
			tokenService.tokenStore.RemoveRefreshToken(refreshTokenValue)
			refreshToken, err = tokenService.createRefreshToken(oauth2Details)
			if err == nil {
				accessToken, err := tokenService.createAccessToken(refreshToken, oauth2Details)
				return accessToken, err
			}
		}
	}
	return nil, err
}

func (tokenService *DefaultTokenService) ReadAccessToken(tokenValue string) (*OAuth2Token, error) {
	return tokenService.tokenStore.ReadAccessToken(tokenValue)
}

func (tokenService *DefaultTokenService) ReadOAuth2Details(tokenValue string) (*OAuth2Details, error) {
	return tokenService.tokenStore.ReadOAuth2Details(tokenValue)
}

func (tokenService *DefaultTokenService) GetOAuth2DetailsByAccessToken(tokenValue string) (*OAuth2Details, error) {
	accessToken, err := tokenService.tokenStore.ReadAccessToken(tokenValue)
	if err == nil {
		if accessToken.IsExpired() {
			return nil, consts.ErrExpiredToken
		}
		return tokenService.tokenStore.ReadOAuth2Details(tokenValue)
	}
	return nil, err
}
