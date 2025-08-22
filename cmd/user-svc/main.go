package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/louis-xie-programmer/easyms/pkg/auth"
	"github.com/louis-xie-programmer/easyms/pkg/config"
	"github.com/louis-xie-programmer/easyms/pkg/logger"
	"github.com/louis-xie-programmer/easyms/pkg/registry"
	"log"
	"net/http"
)

type Profile struct {
	ID, Name string `json:"id"`
}

func main() {
	appConfig, err := config.LoadAppConfig()
	if err != nil {
		log.Fatal(err)
	}

	logger.Init("user-svc", appConfig)

	consulAddr := appConfig.Consul.Host
	if consulAddr == "" {
		consulAddr = "127.0.0.1:8500"
	}

	loader, _ := config.NewServiceConfigLoader(appConfig, "user-svc")
	cfg := loader.GetConfig()

	addr := ":10002"
	svr := cfg["server"].(map[string]interface{})
	if svr != nil {
		addr = fmt.Sprintf("%s:%v", svr["host"].(string), svr["port"].(string))
	}

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r.Group(func(pr chi.Router) {
		// middleware: naive verification using remote jwks; for demo we simply decode token without verifying to extract scopes.
		pr.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// naive check - in prod use proper JWK fetch & jwt.Parse with keyfunc
				authz := r.Header.Get("Authorization")
				if authz == "" {
					http.Error(w, "missing auth", 401)
					return
				}
				token := authz[len("Bearer "):]
				parsed, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{})
				if err != nil {
					http.Error(w, "invalid token", 401)
					return
				}
				if claims, ok := parsed.Claims.(jwt.MapClaims); ok {
					// check scope contains user.read
					if sc, ok := claims["scope"].([]interface{}); ok {
						has := false
						for _, v := range sc {
							if s, ok := v.(string); ok && s == "user.read" {
								has = true
								break
							}
						}
						if !has {
							http.Error(w, "forbidden", 403)
							return
						}
					}
					ctx := context.WithValue(r.Context(), auth.CtxClaims, claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				http.Error(w, "invalid claims", 401)
			})
		})
		pr.Get("/v1/profile", func(w http.ResponseWriter, r *http.Request) {
			p := Profile{ID: "42", Name: "Ada Lovelace"}
			json.NewEncoder(w).Encode(p)
		})
	})

	// register
	apiCfg := &consulapi.Config{
		Address: consulAddr,
	}
	client, _ := consulapi.NewClient(apiCfg)

	h, p := registry.HostPort(addr)
	err = registry.Register(client, "user-svc", h, p, []string{"v1"})

	if err != nil {
		logger.Error(err, "", "main", [][]string{{"event", "register"}})
	}

	logger.Info("user-svc listening", "main", [][]string{{"event", "listening"}})
	err = http.ListenAndServe(addr, r)
	if err != nil {
		logger.Error(err, "listen error", "main", [][]string{{"event", "listening"}})
	}

}
