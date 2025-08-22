
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hashicorp/consul/api"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type Validator struct {
	jwksURL string
	set     jwk.Set
	mu      sync.RWMutex
}

func NewValidator(consulAddr, overrideURL string) (*Validator, error) {
	v := &Validator{}
	if overrideURL != "" {
		v.jwksURL = overrideURL
	} else {
		cfg := api.DefaultConfig(); cfg.Address = consulAddr
		client, err := api.NewClient(cfg); if err != nil { return nil, err }
		services, _, err := client.Health().Service("auth-svc", "", true, nil)
		if err != nil || len(services) == 0 {
			if env := os.Getenv("AUTH_JWKS_URL"); env != "" { v.jwksURL = env } else { return nil, errors.New("auth-svc not found and AUTH_JWKS_URL not set") }
		} else {
			svc := services[0].Service
			v.jwksURL = fmt.Sprintf("http://%s:%d/oauth/jwks", svc.Address, svc.Port)
		}
	}
	if err := v.refresh(); err != nil { return nil, err }
	go func() {
		t := time.NewTicker(10*time.Minute)
		for range t.C { _ = v.refresh() }
	}()
	return v, nil
}

func (v *Validator) refresh() error {
	resp, err := http.Get(v.jwksURL)
	if err != nil { return err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	set, err := jwk.Parse(body)
	if err != nil { return err }
	v.mu.Lock(); v.set = set; v.mu.Unlock()
	return nil
}

func (v *Validator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized); return
		}
		raw := strings.TrimSpace(authz[len("bearer "):])
		parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
		var claims jwt.MapClaims
		_, err := parser.ParseWithClaims(raw, &claims, func(t *jwt.Token) (interface{}, error) {
			v.mu.RLock(); defer v.mu.RUnlock()
			keyID, _ := t.Header["kid"].(string)
			if keyID == "" { return nil, errors.New("missing kid") }
			key, found := v.set.LookupKeyID(keyID)
			if !found {
				_ = v.refresh()
				key, found = v.set.LookupKeyID(keyID)
				if !found { return nil, errors.New("unknown key id") }
			}
			return key.Materialize()
		})
		if err != nil { http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized); return }
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "claims", claims)))
	})
}
