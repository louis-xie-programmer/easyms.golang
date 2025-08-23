package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/endpoint"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/service"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/storage"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/transport"
	"github.com/louis-xie-programmer/easyms/cmd/common/discover"
	"github.com/louis-xie-programmer/easyms/pkg/config"
	"github.com/louis-xie-programmer/easyms/pkg/db"
	"github.com/louis-xie-programmer/easyms/pkg/logger"
	uuid "github.com/satori/go.uuid"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	kitlog "github.com/go-kit/kit/log"
)

func main() {

	serviceName := "auth-svc"

	appConfig, err := config.LoadAppConfig()
	if err != nil {
		log.Fatal(err)
	}
	logger.Init(serviceName, appConfig)

	consulAddr := appConfig.Consul.Host
	if consulAddr == "" {
		consulAddr = "127.0.0.1:8500"
	}

	loader, _ := config.NewServiceConfigLoader(appConfig, serviceName)
	svcCfg := loader.GetConfig()

	var (
		serviceHost = "127.0.0.1"
		servicePort = 10001
	)
	svr := svcCfg["server"].(map[string]interface{})
	if svr != nil {
		serviceHost = svr["host"].(string)
		servicePort = int(svr["port"].(int))
	}

	discoveryClient, err := discover.NewKitDiscoverClient(svr["host"].(string), svr["port"].(int))
	if err != nil {
		logger.Error(err, "Discover Consul Service Error!", "main", [][]string{{"event", "NewKitDiscoverClient"}})
	}

	flag.Parse()

	ctx := context.Background()
	errChan := make(chan error)

	var tokenService service.TokenService
	var tokenGranter service.TokenGranter
	var tokenEnhancer storage.TokenEnhancer
	var tokenStore storage.TokenStore
	var userDetailsService service.UserDetailsService
	var clientDetailsService service.ClientDetailsService
	var srv service.Service

	tokenEnhancer = storage.NewJwtTokenEnhancer("secret")

	// 初始化数据库连接
	dbase, err := db.NewEasyDatabase("postgres", "postgres://postgres:123456@127.0.0.1:5432/postgres?sslmode=disable")
	if err != nil {
		logger.Error(err, "Database connection failed", "main", [][]string{{"event", "db_connect"}})
		os.Exit(-1)
	}

	// 使用支持令牌撤销功能的TokenStore
	tokenStore = storage.NewJwtTokenStore(tokenEnhancer.(*storage.JwtTokenEnhancer), dbase.(*db.EasyDatabase))
	tokenService = service.NewTokenService(tokenStore, tokenEnhancer)

	users := []*model.UserDetails{{
		Username:    "simple",
		Password:    "123456",
		UserId:      1,
		Authorities: []string{"Simple"},
	},
		{
			Username:    "admin",
			Password:    "123456",
			UserId:      1,
			Authorities: []string{"Admin"},
		}}

	// 为用户生成密码哈希
	for _, user := range users {
		err = user.HashPassword()
		if err != nil {
			logger.Error(err, "Failed to hash password", "main", [][]string{{"event", "hash_password"}})
			os.Exit(-1)
		}
		// 清除明文密码
		user.Password = ""
		err = dbase.Insert(user)
		if err != nil {
			// 忽略插入错误，可能用户已存在
		}
	}

	clientDetailsService = service.NewPostgresClientDetailsService(dbase.(*db.EasyDatabase))

	tokenGranter = service.NewComposeTokenGranter(map[string]service.TokenGranter{
		"password":      service.NewUsernamePasswordTokenGranter("password", userDetailsService, tokenService),
		"refresh_token": service.NewRefreshGranter("refresh_token", userDetailsService, tokenService),
	})

	var KitLogger kitlog.Logger
	KitLogger = logger.NewKitLoggerAdapter()
	KitLogger = kitlog.With(KitLogger, "ts", kitlog.DefaultTimestampUTC)
	KitLogger = kitlog.With(KitLogger, "caller", kitlog.DefaultCaller)

	tokenEndpoint := endpoint.MakeTokenEndpoint(tokenGranter, clientDetailsService)
	tokenEndpoint = endpoint.MakeClientAuthorizationMiddleware(KitLogger)(tokenEndpoint)
	checkTokenEndpoint := endpoint.MakeCheckTokenEndpoint(tokenService)
	checkTokenEndpoint = endpoint.MakeClientAuthorizationMiddleware(KitLogger)(checkTokenEndpoint)

	srv = service.NewCommonService()

	simpleEndpoint := endpoint.MakeSimpleEndpoint(srv)
	simpleEndpoint = endpoint.MakeOAuth2AuthorizationMiddleware(KitLogger)(simpleEndpoint)
	adminEndpoint := endpoint.MakeAdminEndpoint(srv)
	adminEndpoint = endpoint.MakeOAuth2AuthorizationMiddleware(KitLogger)(adminEndpoint)
	adminEndpoint = endpoint.MakeAuthorityAuthorizationMiddleware("Admin", KitLogger)(adminEndpoint)

	//创建健康检查的Endpoint
	healthEndpoint := endpoint.MakeHealthCheckEndpoint(srv)

	endpts := endpoint.OAuth2Endpoints{
		TokenEndpoint:       tokenEndpoint,
		CheckTokenEndpoint:  checkTokenEndpoint,
		HealthCheckEndpoint: healthEndpoint,
		SimpleEndpoint:      simpleEndpoint,
		AdminEndpoint:       adminEndpoint,
	}

	//创建http.Handler
	r := transport.MakeHttpHandler(ctx, endpts, tokenService, clientDetailsService, KitLogger)

	instanceId := serviceName + "-" + uuid.NewV4().String()

	//http server
	go func() {
		//config.Logger.Println("Http Server start at port:" + strconv.Itoa(*servicePort))
		//启动前执行注册
		if !discoveryClient.Register(serviceName, instanceId, "/health", serviceHost, servicePort, nil, &KitLogger) {
			//config.Logger.Printf("use-string-service for service %s failed.", serviceName)
			// 注册失败，服务启动失败
			os.Exit(-1)
		}
		handler := r
		errChan <- http.ListenAndServe(":"+strconv.Itoa(servicePort), handler)
	}()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errChan <- fmt.Errorf("%s", <-c)
	}()

	//服务退出取消注册
	discoveryClient.DeRegister(instanceId, &KitLogger)

	logger.Info("exit", "main", [][]string{{"event", "main"}})
}
