package service

import (
	"context"
	"easyms/internal/services/auth/internal/storage"
	. "easyms/internal/shared/models"
	"errors"
	"github.com/satori/go.uuid"
	"strconv"
	"time"
)

// TokenService defines the interface for token management.
type TokenService interface {
	GetOAuth2DetailsByAccessToken(ctx context.Context, tokenValue string) (*OAuth2Details, error)
	CreateAccessToken(ctx context.Context, oauth2Details *OAuth2Details) (*OAuth2Token, error)
	RefreshAccessToken(ctx context.Context, refreshTokenValue string) (*OAuth2Token, error)
	ReadAccessToken(ctx context.Context, tokenValue string) (*OAuth2Token, error)
	ReadOAuth2Details(ctx context.Context, tokenValue string) (*OAuth2Details, error)
}

// Pre-defined business errors for better error handling in the upper layers.
var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrTokenExpired   = errors.New("token is expired")
	ErrInternalServer = errors.New("internal server error")
)

type DefaultTokenService struct {
	tokenStore    storage.TokenStore
	tokenEnhancer storage.TokenEnhancer
}

func NewTokenService(tokenStore storage.TokenStore, tokenEnhancer storage.TokenEnhancer) TokenService {
	return &DefaultTokenService{
		tokenStore:    tokenStore,
		tokenEnhancer: tokenEnhancer,
	}
}

func (s *DefaultTokenService) CreateAccessToken(ctx context.Context, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	refreshToken, err := s.createRefreshToken(ctx, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}
	accessToken, err := s.createAccessToken(ctx, refreshToken, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}
	return accessToken, nil
}

func (s *DefaultTokenService) createAccessToken(ctx context.Context, refreshToken *OAuth2Token, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	validitySeconds := oauth2Details.Client.AccessTokenValiditySeconds
	duration, _ := time.ParseDuration(strconv.Itoa(validitySeconds) + "s")
	expiredTime := time.Now().Add(duration)
	accessToken := &OAuth2Token{
		RefreshToken: refreshToken,
		ExpiresTime:  &expiredTime,
		TokenValue:   uuid.NewV4().String(),
	}

	if s.tokenEnhancer != nil {
		return s.tokenEnhancer.Enhance(accessToken, oauth2Details)
	}
	return accessToken, nil
}

func (s *DefaultTokenService) createRefreshToken(ctx context.Context, oauth2Details *OAuth2Details) (*OAuth2Token, error) {
	validitySeconds := oauth2Details.Client.RefreshTokenValiditySeconds
	duration, _ := time.ParseDuration(strconv.Itoa(validitySeconds) + "s")
	expiredTime := time.Now().Add(duration)
	refreshToken := &OAuth2Token{
		ExpiresTime: &expiredTime,
		TokenValue:  uuid.NewV4().String(),
	}

	if s.tokenEnhancer != nil {
		return s.tokenEnhancer.Enhance(refreshToken, oauth2Details)
	}
	return refreshToken, nil
}

func (s *DefaultTokenService) RefreshAccessToken(ctx context.Context, refreshTokenValue string) (*OAuth2Token, error) {
	// 1. Read the old refresh token. This also checks if it's revoked.
	refreshToken, err := s.tokenStore.ReadAccessToken(refreshTokenValue)
	if err != nil {
		if errors.Is(err, storage.ErrTokenRevoked) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	// 2. Check if the token is expired.
	if refreshToken.IsExpired() {
		// If expired, remove it from the store to prevent reuse.
		s.tokenStore.RemoveRefreshToken(refreshTokenValue)
		return nil, ErrTokenExpired
	}

	// 3. Read the details associated with the token.
	oauth2Details, err := s.tokenStore.ReadOAuth2Details(refreshTokenValue)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// 4. Create new refresh and access tokens first.
	newRefreshToken, err := s.createRefreshToken(ctx, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}

	newAccessToken, err := s.createAccessToken(ctx, newRefreshToken, oauth2Details)
	if err != nil {
		return nil, ErrInternalServer
	}

	// 5. Only after new tokens are successfully created, safely remove the old refresh token.
	s.tokenStore.RemoveRefreshToken(refreshTokenValue)

	return newAccessToken, nil
}

func (s *DefaultTokenService) ReadAccessToken(ctx context.Context, tokenValue string) (*OAuth2Token, error) {
	return s.tokenStore.ReadAccessToken(tokenValue)
}

func (s *DefaultTokenService) ReadOAuth2Details(ctx context.Context, tokenValue string) (*OAuth2Details, error) {
	return s.tokenStore.ReadOAuth2Details(tokenValue)
}

func (s *DefaultTokenService) GetOAuth2DetailsByAccessToken(ctx context.Context, tokenValue string) (*OAuth2Details, error) {
	accessToken, err := s.tokenStore.ReadAccessToken(tokenValue)
	if err != nil {
		if errors.Is(err, storage.ErrTokenRevoked) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	if accessToken.IsExpired() {
		return nil, ErrTokenExpired
	}

	// This seems redundant as ReadAccessToken already extracts details.
	// Assuming ReadOAuth2Details is the correct way to get full details.
	return s.tokenStore.ReadOAuth2Details(tokenValue)
}
