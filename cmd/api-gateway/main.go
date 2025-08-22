package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/louis-xie-programmer/easyms/pkg/config"
	"github.com/louis-xie-programmer/easyms/pkg/logger"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/sd"
	kitconsul "github.com/go-kit/kit/sd/consul"
	"github.com/go-kit/kit/sd/lb"
	"github.com/hashicorp/consul/api"
	"github.com/mercari/go-circuitbreaker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/louis-xie-programmer/easyms/pkg/auth"
)

var (
	cbStates = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Circuit breaker state per instance (0=closed,1=halfopen,2=open)",
	}, []string{"instance"})
)

func init() { prometheus.MustRegister(cbStates) }

var (
	breakers = map[string]*circuitbreaker.CircuitBreaker{}
	cbMutex  sync.RWMutex
)

func getBreaker(instance string) *circuitbreaker.CircuitBreaker {
	cbMutex.RLock()
	br, ok := breakers[instance]
	cbMutex.RUnlock()
	if ok {
		return br
	}
	cbMutex.Lock()
	defer cbMutex.Unlock()
	if br, ok = breakers[instance]; ok {
		return br
	}
	br = circuitbreaker.New(
		circuitbreaker.WithCounterResetInterval(10*time.Second),
		circuitbreaker.WithHalfOpenMaxSuccesses(4),
		circuitbreaker.WithTripFunc(circuitbreaker.NewTripFuncFailureRate(10, 0.4)),
		circuitbreaker.WithFailOnContextCancel(true),
		circuitbreaker.WithOnStateChangeHookFn(func(from, to circuitbreaker.State) {
			log.Printf("[CB][%s] %s -> %s", instance, from, to)
			switch to {
			case circuitbreaker.StateClosed:
				cbStates.WithLabelValues(instance).Set(0)
			case circuitbreaker.StateHalfOpen:
				cbStates.WithLabelValues(instance).Set(1)
			case circuitbreaker.StateOpen:
				cbStates.WithLabelValues(instance).Set(2)
			}
		}),
	)
	breakers[instance] = br
	return br
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
	svcCfg := loader.GetConfig()

	addr := ":10001"
	authUrl := ""
	svr := svcCfg["server"].(map[string]interface{})
	if svr != nil {
		addr = fmt.Sprintf("%s:%v", svr["host"].(string), svr["port"].(string))
		authUrl = svr["auth_url"].(string)
	}

	validator, err := auth.NewValidator(consulAddr, authUrl)
	if err != nil {
		logger.Error(err, "auth validator error", "main", [][]string{{"msg", "auth.NewValidator"}})
	}

	cfg := api.DefaultConfig()
	cfg.Address = consulAddr
	consulClient, err := api.NewClient(cfg)
	if err != nil {
		logger.Error(err, "consul client error", "main", [][]string{{"msg", "api.NewClient"}})
	}

	easyLog := logger.NewKitLoggerAdapter()

	instancer := kitconsul.NewInstancer(kitconsul.NewClient(consulClient), easyLog, "user-svc", []string{}, true)

	factory := func(instance string) (endpoint.Endpoint, io.Closer, error) {
		target, err := url.Parse("http://" + instance)
		if err != nil {
			return nil, nil, err
		}
		br := getBreaker(instance)

		ep := func(ctx context.Context, _ interface{}) (interface{}, error) {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target.String()+"/v1/profile", nil)
			if authz := ctx.Value("authz"); authz != nil {
				req.Header.Set("Authorization", authz.(string))
			}

			return br.Do(ctx, func() (interface{}, error) {
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					return nil, err
				}
				defer res.Body.Close()
				body, _ := io.ReadAll(res.Body)
				if res.StatusCode >= 500 {
					return nil, fmt.Errorf("backend %d: %s", res.StatusCode, string(body))
				}
				return body, nil
			})
		}
		return ep, nil, nil
	}

	endpointer := sd.NewEndpointer(instancer, factory, easyLog)
	rr := lb.NewRoundRobin(endpointer)

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Handle("/metrics", promhttp.Handler())

	r.Group(func(pr chi.Router) {
		pr.Use(validator.Middleware)
		pr.Get("/api/profile", func(w http.ResponseWriter, r *http.Request) {
			ep, err := rr.Endpoint()
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			ctx := context.WithValue(r.Context(), "authz", r.Header.Get("Authorization"))
			resp, err := ep(ctx, nil)
			if err != nil {
				if errors.Is(err, circuitbreaker.ErrOpen) {
					w.Header().Set("X-CB-State", "open")
					http.Error(w, "circuit open", http.StatusServiceUnavailable)
					return
				}
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(resp.([]byte))
		})
	})

	logger.Info("api-gateway starting", "main", [][]string{{"msg", "api-gateway starting"}})

	err = http.ListenAndServe(addr, r)

	if err != nil {
		logger.Error(err, "listen error", "main", [][]string{{"msg", "listen error"}})
	}
}
