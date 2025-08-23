package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/louis-xie-programmer/easyms/pkg/auth"
	"github.com/louis-xie-programmer/easyms/pkg/config"
	"github.com/louis-xie-programmer/easyms/pkg/logger"
	"github.com/louis-xie-programmer/easyms/pkg/registry"
	"log"
	"net/http"
)

type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
		// 创建认证验证器
		validator, err := auth.NewValidator(consulAddr, "")
		if err != nil {
			log.Fatal("Failed to create auth validator:", err)
		}

		// 使用pkg/auth包中的安全验证中间件
		pr.Use(validator.Middleware)
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
