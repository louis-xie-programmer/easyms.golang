package main

import (
	"easyms/internal/shared/config"
	"easyms/internal/shared/discovery"
	"easyms/internal/shared/logger"
	"easyms/internal/shared/middleware"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// testCompleteOAuth2Flow 测试完整的OAuth2授权流程
func testCompleteOAuth2Flow() {
	fmt.Println("=== 开始测试完整的OAuth2授权流程 ===")

	// 第一步：获取客户端授权
	clientToken, clientRefreshToken := getClientCredentialsToken()
	if clientToken == "" {
		fmt.Println("客户端凭证授权失败，无法继续测试")
		return
	}

	// 第二步：通过客户端授权token获取用户授权
	userToken, userRefreshToken := getUserAuthorizationToken(clientToken)
	fmt.Println("用户授权令牌：", userToken)
	fmt.Println("用户授权刷新令牌：", userRefreshToken)
	if userRefreshToken == "" {
		fmt.Println("用户授权失败，无法继续测试")
		return
	}

	// 第三步：刷新用户令牌
	refreshedUserToken := refreshUserToken(userToken, userRefreshToken)
	if refreshedUserToken == "" {
		fmt.Println("用户令牌刷新失败")
		return
	}

	// 第四步：刷新客户端令牌
	refreshedClientToken := refreshClientToken(clientToken, clientRefreshToken)
	if refreshedClientToken == "" {
		fmt.Println("客户端令牌刷新失败")
		return
	}

	fmt.Println("=== 完整的OAuth2授权流程测试完成 ===")
}

type ClientTokenRequest struct {
	GrantType    string `json:"grant_type"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// TokenResponse 令牌响应结构体
type TokenResponse struct {
	AccessToken  *OAuth2Token `json:"access_token"`
	RefreshToken *OAuth2Token `json:"refresh_token,omitempty"`
	Error        string       `json:"error,omitempty"`
}

type OAuth2Token struct {
	// 刷新令牌
	RefreshToken *OAuth2Token
	// 令牌类型
	TokenType string
	// 令牌
	TokenValue string
	// 过期时间
	ExpiresTime *time.Time
}

// getClientCredentialsToken 获取客户端凭证令牌
func getClientCredentialsToken() (string, string) {
	fmt.Println("--- 步骤1: 获取客户端凭证令牌 ---")

	// 准备客户端凭证授权数据
	data := `{"grant_type":"client_credentials", "client_id":"user-svc", "client_secret":"user_secret"}`

	// 发送POST请求到客户端凭证授权端点
	req, err := http.NewRequest("POST", "http://localhost:10001/oauth2/token",
		strings.NewReader(data))
	if err != nil {
		fmt.Printf("创建客户端凭证请求失败: %v\n", err)
		return "", ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("客户端凭证请求失败: %v\n", err)
		return "", ""
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != 200 {
		fmt.Printf("客户端凭证授权失败，状态码: %d\n", resp.StatusCode)
		return "", ""
	}

	// 解析响应
	var tokenResp TokenResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应体失败: %v\n", err)
		return "", ""
	}

	err = json.Unmarshal(body, &tokenResp)
	if err != nil {
		fmt.Printf("解析响应体失败: %v\n", err)
		return "", ""
	}

	if tokenResp.Error != "" {
		fmt.Printf("获取客户端凭证令牌失败: %s\n", tokenResp.Error)
		return "", ""
	}

	fmt.Printf("成功获取客户端凭证令牌\n")
	if tokenResp.RefreshToken != nil {
		fmt.Printf("成功获取客户端凭证刷新令牌 %s\n; %s\n", tokenResp.AccessToken.TokenValue, tokenResp.RefreshToken.TokenValue)
		return tokenResp.AccessToken.TokenValue, tokenResp.RefreshToken.TokenValue
	}
	return tokenResp.AccessToken.TokenValue, ""
}

// getUserAuthorizationToken 获取用户授权令牌
func getUserAuthorizationToken(clientToken string) (string, string) {
	fmt.Println("--- 步骤2: 获取用户授权令牌 ---")

	// 准备用户授权数据
	loginData := `{"username":"user1","password":"password1"}`

	// 发送POST请求到用户登录端点
	req, err := http.NewRequest("POST", "http://localhost:10001/login",
		strings.NewReader(loginData))
	if err != nil {
		fmt.Printf("创建用户登录请求失败: %v\n", err)
		return "", ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 设置客户端凭证
	req.Header.Set("Authorization", "Bearer "+clientToken)

	// 发送请求
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("用户登录请求失败: %v\n", err)
		return "", ""
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != 200 {
		fmt.Printf("用户登录失败，状态码: %d\n", resp.StatusCode)
		return "", ""
	}

	// 解析登录响应
	var loginResp struct {
		AccessToken  *OAuth2Token `json:"access_token"`
		RefreshToken *OAuth2Token `json:"refresh_token,omitempty"`
		Error        string       `json:"error,omitempty"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应体失败: %v\n", err)
		return "", ""
	}

	err = json.Unmarshal(body, &loginResp)
	if err != nil {
		fmt.Printf("解析响应体失败: %v\n", err)
		return "", ""
	}

	if loginResp.Error != "" {
		fmt.Printf("用户登录失败: %s\n", loginResp.Error)
		return "", ""
	}

	fmt.Printf("成功获取用户访问令牌\n")
	return loginResp.AccessToken.TokenValue, loginResp.RefreshToken.TokenValue
}

// refreshUserToken 刷新用户令牌
func refreshUserToken(userToken, refreshToken string) string {
	fmt.Println("--- 步骤3: 刷新用户令牌 ---")

	// 准备刷新令牌数据
	refreshData := "{\"refresh_token\":\"" + refreshToken + "\"}"

	// 发送POST请求到用户刷新令牌端点
	req, err := http.NewRequest("POST", "http://localhost:10001/oauth2/refresh", strings.NewReader(refreshData))
	if err != nil {
		fmt.Printf("创建用户令牌刷新请求失败: %v\n", err)
		return ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 设置客户端凭证
	req.Header.Set("client_id", "user-svc")
	req.Header.Set("client_secret", "user_secret")
	// 使用用户访问令牌作为Authorization头
	req.Header.Set("Authorization", "Bearer "+userToken)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("用户令牌刷新请求失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	// 解析响应
	var refreshResp TokenResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应体失败: %v\n", err)
		return ""
	}

	err = json.Unmarshal(body, &refreshResp)
	if err != nil {
		fmt.Printf("解析响应体失败: %v\n", err)
		return ""
	}

	if refreshResp.Error != "" {
		fmt.Printf("刷新用户令牌失败: %s\n", refreshResp.Error)
		return ""
	}

	fmt.Printf("成功刷新用户令牌\n")
	return refreshResp.AccessToken.TokenValue
}

// refreshClientToken 刷新客户端令牌
func refreshClientToken(clientToken, refreshToken string) string {
	fmt.Println("--- 步骤4: 刷新客户端令牌 ---")

	// 准备刷新令牌数据
	refreshData := "{\"refresh_token\":\"" + refreshToken + "\"}"

	// 发送POST请求到客户端刷新令牌端点
	req, err := http.NewRequest("POST", "http://localhost:10001/oauth2/client-refresh", strings.NewReader(refreshData))
	if err != nil {
		fmt.Printf("创建客户端令牌刷新请求失败: %v\n", err)
		return ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 设置客户端凭证
	req.Header.Set("client_id", "user-svc")
	req.Header.Set("client_secret", "user_secret")
	// 使用客户端访问令牌作为Authorization头
	req.Header.Set("Authorization", "Bearer "+clientToken)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("客户端令牌刷新请求失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	// 解析响应
	var refreshResp TokenResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应体失败: %v\n", err)
		return ""
	}

	err = json.Unmarshal(body, &refreshResp)
	if err != nil {
		fmt.Printf("解析响应体失败: %v\n", err)
		return ""
	}

	if refreshResp.Error != "" {
		fmt.Printf("刷新客户端令牌失败: %s\n", refreshResp.Error)
		return ""
	}

	fmt.Printf("成功刷新客户端令牌\n")
	return refreshResp.AccessToken.TokenValue
}

// main 用户服务主函数
func main() {
	// 获取环境变量
	// 用户服务名称和端口
	serverName := "user-svc"

	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		logger.Error(err, "Failed to initialize app config store", serverName, nil)
	}

	// 初始化服务发现客户端
	// 连接到Consul服务注册与发现中心
	d, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName, nil)
	}

	// 创建 Discovery 客户端用于配置加载
	discoveryClient := d

	// 加载服务配置
	// 根据配置类型（本地或Consul）加载服务配置
	var provider config.AppConfigProvider
	// 加载应用配置
	if cfgStore.StoreType == "consul" {
		// 使用Consul配置提供者
		provider = config.NewConsulConfig(discoveryClient, serverName, cfgStore.Consul.KeyPath, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			logger.Error(err, "Failed to load app config", serverName, nil)
			panic(err)
		}
		// 动态监听配置文件并更新服务
		watch := config.NewConfigWatcher(discoveryClient, cfgStore.Consul.KeyPath, serverName, cfgStore.Env, provider.OnChange())
		go watch.Start()
	} else {
		// 使用本地配置提供者
		// 从本地配置文件加载配置
		provider = config.NewLocalConfig(serverName, cfgStore.Env)
		err := provider.LoadAppConfig()
		if err != nil {
			logger.Error(err, "Failed to load local app config", serverName, nil)
			panic(err)
		}
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()

	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	// 只在非生产环境中执行测试
	if cfgStore.Env != "prod" {
		go func() {
			// 给其他服务一些启动时间
			time.Sleep(5 * time.Second)
			// 执行完整的OAuth2授权流程测试
			testCompleteOAuth2Flow()
		}()
	}

	// 注册服务到Consul
	// 将当前服务注册到Consul服务注册中心
	// 添加短暂延迟以避免服务注册冲突
	time.Sleep(time.Millisecond * 200)
	err = d.Register(serverName, appConfig.Server.Host, appConfig.Server.Port, nil)
	if err != nil {
		panic(err)
	}

	//延迟注销服务
	//确保服务在退出时从Consul中注销
	defer d.DeRegister(serverName)

	//启动 HTTP 服务
	//使用Gin框架启动HTTP服务
	g := gin.Default()

	//健康检查端点
	//提供健康检查接口，供Consul等监控系统使用
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})

	//用户服务接口
	//提供用户相关信息的接口
	g.GET("/api/user/:id", func(c *gin.Context) {
		c.String(200, "user id: "+c.Param("id"))
	})

	// 测试接口 - 模拟受保护资源
	g.GET("/api/protected", middleware.AuthMiddleware(), func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello, you have accessed protected resource!",
			"user":    c.Request.Header.Get("user"), // 示例用户信息
		})
	})

	//启动HTTP服务
	fmt.Printf("Starting %s on port %d\n", serverName, appConfig.Server.Port)
	if err = g.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName, nil)
	}
}
