package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

func TestValidator(t *testing.T) {
	// 生成RSA密钥对用于测试
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// 创建JWK
	key, err := jwk.FromRaw(privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create JWK: %v", err)
	}
	
	// 设置key ID
	key.Set(jwk.KeyIDKey, "test-key-id")

	// 创建JWK Set
	set := jwk.NewSet()
	set.AddKey(key)

	// 创建模拟的JWKS服务器
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(set)
		if err != nil {
			t.Errorf("Failed to encode JWKS: %v", err)
		}
	}))
	defer jwksServer.Close()

	// 创建验证器
	validator, err := NewValidator("", jwksServer.URL)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// 等待验证器刷新JWKS
	time.Sleep(100 * time.Millisecond)

	// 创建测试令牌
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "test-user",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iss": "test-issuer",
	})
	token.Header["kid"] = "test-key-id"

	// 签名令牌
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	// 创建测试HTTP处理器
	handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 创建测试请求
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	// 创建响应记录器
	rr := httptest.NewRecorder()

	// 执行请求
	handler.ServeHTTP(rr, req)

	// 检查响应
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestValidatorInvalidToken(t *testing.T) {
	// 生成RSA密钥对用于测试
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// 创建JWK
	key, err := jwk.FromRaw(privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to create JWK: %v", err)
	}
	
	// 设置key ID
	key.Set(jwk.KeyIDKey, "test-key-id")

	// 创建JWK Set
	set := jwk.NewSet()
	set.AddKey(key)

	// 创建模拟的JWKS服务器
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(set)
		if err != nil {
			t.Errorf("Failed to encode JWKS: %v", err)
		}
	}))
	defer jwksServer.Close()

	// 创建验证器
	validator, err := NewValidator("", jwksServer.URL)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// 等待验证器刷新JWKS
	time.Sleep(100 * time.Millisecond)

	// 创建无效的测试令牌（未签名）
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "test-user",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	tokenString, err := token.SigningString()
	if err != nil {
		t.Fatalf("Failed to create token string: %v", err)
	}

	// 创建测试HTTP处理器
	handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 创建测试请求
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	// 创建响应记录器
	rr := httptest.NewRecorder()

	// 执行请求
	handler.ServeHTTP(rr, req)

	// 检查响应
	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
	}
}