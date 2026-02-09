package storage

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/models"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// --- Mocks ---

// MockDatabase is a mock for db.Database
type MockDatabase struct {
	mock.Mock
}

func (m *MockDatabase) Insert(ctx context.Context, value interface{}) error {
	args := m.Called(ctx, value)
	return args.Error(0)
}

func (m *MockDatabase) CountByField(ctx context.Context, model interface{}, field string, value interface{}) (int64, error) {
	args := m.Called(ctx, model, field, value)
	return args.Get(0).(int64), args.Error(1)
}

// Implement other db.Database methods as needed by the compiler, returning nil or zero values.
func (m *MockDatabase) AutoMigrate(models ...interface{}) error { return nil }
func (m *MockDatabase) Query(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return nil
}
func (m *MockDatabase) Count(ctx context.Context, query string, args ...interface{}) (int64, error) {
	return 0, nil
}
func (m *MockDatabase) Where(ctx context.Context, query string, args ...interface{}) *gorm.DB {
	return nil
}
func (m *MockDatabase) Order(ctx context.Context, query string) *gorm.DB { return nil }
func (m *MockDatabase) Limit(ctx context.Context, limit int) *gorm.DB    { return nil }
func (m *MockDatabase) Update(ctx context.Context, model interface{}, updates map[string]interface{}) error {
	return nil
}
func (m *MockDatabase) Delete(ctx context.Context, model interface{}, conds ...interface{}) error {
	return nil
}
func (m *MockDatabase) GetDB() *gorm.DB                                     { return nil }
func (m *MockDatabase) GetType() string                                     { return "" }
func (m *MockDatabase) Begin(ctx context.Context) (db.TxTransaction, error) { return nil, nil }
func (m *MockDatabase) RunInTransaction(ctx context.Context, fn func(tx db.TxTransaction) error) error {
	return nil
}
func (m *MockDatabase) Close() error { return nil }

// Ensure MockDatabase implements the interface
var _ db.Database = (*MockDatabase)(nil)

// MockRedis is a mock for db.RedisClient
type MockRedis struct {
	mock.Mock
}

func (m *MockRedis) SetEx(key string, value interface{}, expireSeconds int) error {
	args := m.Called(key, value, expireSeconds)
	return args.Error(0)
}

func (m *MockRedis) GetCache(key string) (string, error) {
	args := m.Called(key)
	return args.String(0), args.Error(1)
}

func (m *MockRedis) RunScript(ctx context.Context, script *redis.Script, keys []string, args ...interface{}) *redis.Cmd {
	// For testing purposes, we don't need to mock the return of RunScript in this specific test file.
	// If other tests need it, they can set expectations on this method.
	// Returning a new Cmd allows chaining, though we don't use it here.
	return redis.NewCmd(ctx)
}

// Ensure MockRedis implements the interface
var _ db.RedisClient = (*MockRedis)(nil)

// --- Tests ---

func TestJwtTokenStore_RevokeAndCheck(t *testing.T) {
	secret := "test-secret"
	enhancer := NewJwtTokenEnhancer(secret).(*JwtTokenEnhancer)

	oauth2Details := &models.OAuth2Details{
		Client: &models.ClientDetails{ClientId: "test-client"},
		User:   &models.User{Username: "test-user"},
	}
	token, _ := enhancer.Enhance(&models.OAuth2Token{
		ExpiresTime: func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
		TokenValue:  "test-jti",
	}, oauth2Details)

	tokenValue := token.TokenValue
	tokenHash := getTokenHash(tokenValue)
	accessCacheKey := "revoked:access:" + tokenHash

	t.Run("Revoke should write to Redis and DB", func(t *testing.T) {
		mockDB := new(MockDatabase)
		mockRedis := new(MockRedis)
		tokenStore := NewJwtTokenStore(enhancer, mockDB, mockRedis).(*JwtTokenStore)

		mockRedis.On("SetEx", accessCacheKey, "revoked", mock.AnythingOfType("int")).Return(nil)
		mockDB.On("Insert", mock.Anything, mock.AnythingOfType("*models.RevokedToken")).Return(nil)

		tokenStore.RemoveAccessToken(tokenValue)

		mockRedis.AssertExpectations(t)
		mockDB.AssertExpectations(t)
		_, found := tokenStore.revokedCache.Get(accessCacheKey)
		assert.True(t, found)
	})

	t.Run("Check revoked token found in local cache", func(t *testing.T) {
		mockDB := new(MockDatabase)
		mockRedis := new(MockRedis)
		tokenStore := NewJwtTokenStore(enhancer, mockDB, mockRedis).(*JwtTokenStore)

		tokenStore.revokedCache.Set(accessCacheKey, true, 1*time.Minute)

		revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)

		assert.NoError(t, err)
		assert.True(t, revoked)
		mockRedis.AssertNotCalled(t, "GetCache")
		mockDB.AssertNotCalled(t, "CountByField")
	})

	t.Run("Check revoked token found in Redis (local cache miss)", func(t *testing.T) {
		mockDB := new(MockDatabase)
		mockRedis := new(MockRedis)
		tokenStore := NewJwtTokenStore(enhancer, mockDB, mockRedis).(*JwtTokenStore)

		mockRedis.On("GetCache", accessCacheKey).Return("revoked", nil)

		revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)

		assert.NoError(t, err)
		assert.True(t, revoked)
		mockRedis.AssertExpectations(t)
		_, found := tokenStore.revokedCache.Get(accessCacheKey)
		assert.True(t, found)
	})

	t.Run("Check revoked token found in DB (Redis miss)", func(t *testing.T) {
		mockDB := new(MockDatabase)
		mockRedis := new(MockRedis)
		tokenStore := NewJwtTokenStore(enhancer, mockDB, mockRedis).(*JwtTokenStore)

		mockRedis.On("GetCache", accessCacheKey).Return("", redis.Nil)
		mockDB.On("CountByField", mock.Anything, &models.RevokedToken{}, "token_hash", tokenHash).Return(int64(1), nil)

		revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)

		assert.NoError(t, err)
		assert.True(t, revoked)
		mockRedis.AssertExpectations(t)
		mockDB.AssertExpectations(t)
		_, found := tokenStore.revokedCache.Get(accessCacheKey)
		assert.True(t, found)
	})

	t.Run("Check revoked token found in DB (Redis error)", func(t *testing.T) {
		mockDB := new(MockDatabase)
		mockRedis := new(MockRedis)
		tokenStore := NewJwtTokenStore(enhancer, mockDB, mockRedis).(*JwtTokenStore)

		mockRedis.On("GetCache", accessCacheKey).Return("", errors.New("redis connection error"))
		mockDB.On("CountByField", mock.Anything, &models.RevokedToken{}, "token_hash", tokenHash).Return(int64(1), nil)

		revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)

		assert.NoError(t, err)
		assert.True(t, revoked)
		mockRedis.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})

	t.Run("Check not-revoked token", func(t *testing.T) {
		mockDB := new(MockDatabase)
		mockRedis := new(MockRedis)
		tokenStore := NewJwtTokenStore(enhancer, mockDB, mockRedis).(*JwtTokenStore)

		mockRedis.On("GetCache", accessCacheKey).Return("", redis.Nil)
		mockDB.On("CountByField", mock.Anything, &models.RevokedToken{}, "token_hash", tokenHash).Return(int64(0), nil)

		revoked, err := tokenStore.IsAccessTokenRevoked(tokenValue)

		assert.NoError(t, err)
		assert.False(t, revoked)
		mockRedis.AssertExpectations(t)
		mockDB.AssertExpectations(t)
	})
}
