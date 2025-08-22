package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type RS256Signer struct {
	Priv *rsa.PrivateKey
}

func NewRS256SignerFromPEM(privPem []byte) (*RS256Signer, error) {
	block, _ := pem.Decode(privPem)
	if block == nil {
		return nil, errors.New("invalid pem")
	}
	pk, err := jwt.ParseRSAPrivateKeyFromPEM(privPem)
	if err != nil {
		return nil, err
	}
	return &RS256Signer{Priv: pk}, nil
}

func GenerateRS256Key(bits int) ([]byte, error) {
	pk, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	b := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509MarshalPKCS1PrivateKey(pk)})
	return b, nil
}

// helper to marshal PKCS1 (since x509.MarshalPKCS1PrivateKey is in x509)
func x509MarshalPKCS1PrivateKey(key *rsa.PrivateKey) []byte {
	return x509.MarshalPKCS1PrivateKey(key)
}

func (s *RS256Signer) Issue(sub string, scopes []string, ttlSeconds int64) (string, error) {
	claims := jwt.MapClaims{
		"sub":   sub,
		"scope": scopes,
		"exp":   jwt.NewNumericDate(time.Now().Add(time.Duration(ttlSeconds) * time.Second)),
		"iat":   jwt.NewNumericDate(time.Now()),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return t.SignedString(s.Priv)
}

func (s *RS256Signer) PublicJWK() (map[string]interface{}, error) {
	n := base64URLEncode(s.Priv.PublicKey.N.Bytes())
	e := fmt.Sprintf("%d", s.Priv.PublicKey.E)
	jwk := map[string]interface{}{"kty": "RSA", "n": n, "e": e, "alg": "RS256", "use": "sig", "kid": "1"}
	return jwk, nil
}

// minimal helpers (base64url)
func base64URLEncode(b []byte) string {
	s := base64.RawURLEncoding.EncodeToString(b)
	return s
}
