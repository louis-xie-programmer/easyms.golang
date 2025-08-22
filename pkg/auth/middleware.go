package auth

import (
	"context"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"strings"
)

type ctxKey string

const CtxClaims ctxKey = "claims"

func MiddlewareJWTVerify(keyGetter func(*jwt.Token) (interface{}, error), requiredScopes ...string) func(http.Handler) http.Handler {
	need := map[string]struct{}{}
	for _, s := range requiredScopes {
		need[s] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(authz, "Bearer ") {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			tok := strings.TrimPrefix(authz, "Bearer ")
			parsed, err := jwt.Parse(tok, keyGetter)
			if err != nil || !parsed.Valid {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			if c, ok := parsed.Claims.(jwt.MapClaims); ok {
				if len(need) > 0 {
					// scopes possibly []string or space-separated
					var has bool
					if sc, ok := c["scope"].([]interface{}); ok {
						m := map[string]struct{}{}
						for _, v := range sc {
							if s, ok := v.(string); ok {
								m[s] = struct{}{}
							}
						}
						has = true
						for k := range need {
							if _, ok := m[k]; !ok {
								has = false
								break
							}
						}
					} else if ss, ok := c["scope"].(string); ok {
						// space separated
						for k := range need {
							if !strings.Contains(ss, k) {
								has = false
								break
							}
							has = true
						}
					}
					if !has {
						http.Error(w, "insufficient_scope", http.StatusForbidden)
						return
					}
				}
				ctx := context.WithValue(r.Context(), CtxClaims, c)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			http.Error(w, "claims", http.StatusUnauthorized)
		})
	}
}
