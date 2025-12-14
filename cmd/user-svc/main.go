// main.go 用户服务主程序
// 主要功能：
// 1. 初始化服务配置
// 2. 启动服务发现客户端
// 3. 注册服务到Consul
// 4. 启动HTTP服务
package main

import (
	"bytes"
	"easyms/cmd/auth-svc/model"
	"easyms/pkg/config"
	"easyms/pkg/discovery"
	"easyms/pkg/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type TokenResponse struct {
	AccessToken  *model.OAuth2Token `json:"access_token"`
	RefreshToken *model.OAuth2Token `json:"refresh_token,omitempty"`
	Error        string             `json:"error"`
}

// main 用户服务入口函数
// 初始化配置、服务发现客户端，注册服务并启动HTTP服务
func main() {
	// 获取环境变量
	// 用户服务名称
	serverName := "user-svc"

	// 初始化应用配置存储
	// 读取 configs/app.yaml 配置文件
	cfgStore, err := config.InitAppConfigStore()
	if err != nil {
		logger.Error(err, "Failed to initialize app config store", serverName, nil)
	}

	// 初始化 Consul 服务发现客户端
	// 连接到Consul服务注册与发现中心
	d, err := discovery.NewDiscovery(cfgStore.Consul.Host)
	if err != nil {
		logger.Error(err, "Failed to create consul client", serverName, nil)
	}

	// 加载服务配置
	// 根据配置类型（本地或Consul）加载服务配置
	err = config.LoadServiceConfig(d, cfgStore.Consul.KeyPath, cfgStore.StoreType, serverName, cfgStore.Env)
	if err != nil {
		logger.Error(err, "Failed to load service config", serverName, nil)
	}

	// 获取应用配置
	appConfig := config.GetAppConfig()

	// 初始化日志系统
	// 根据配置初始化日志系统（本地或Loki）
	logger.Init(serverName, appConfig)

	// 执行完整的OAuth2授权流程测试
	// testCompleteOAuth2Flow()

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

	//启动HTTP服务
	fmt.Printf("Starting %s on port %d\n", serverName, appConfig.Server.Port)
	if err = g.Run(fmt.Sprintf(":%d", appConfig.Server.Port)); err != nil {
		logger.Error(err, "Failed to start HTTP server", serverName, nil)
	}
}

// testCompleteOAuth2Flow 测试完整的OAuth2授权流程
// 1. 获取客户端授权
// 2. 通过客户端授权token获取用户授权
// 3. 刷新客户端令牌
// 4. 刷新用户令牌
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

// getClientCredentialsToken 获取客户端凭证授权令牌
// 返回访问令牌字符串
func getClientCredentialsToken() (string, string) {
	fmt.Println("\n--- 步骤1: 获取客户端凭证授权 ---")

	// 准备JSON数据 (认证服务期望JSON格式)
	tokenData := map[string]interface{}{
		"grant_type": "client_credentials",
	}

	jsonData, err := json.Marshal(tokenData)
	if err != nil {
		fmt.Printf("序列化令牌数据失败: %v\n", err)
		return "", ""
	}

	// 构造请求
	req, err := http.NewRequest("POST", "http://localhost:10001/c-auth/token", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("创建客户端凭证请求失败: %v\n", err)
		return "", ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 设置客户端认证信息（通过header传递）
	// 使用在数据库中创建的客户端信息
	req.Header.Set("client_id", "user-svc")
	req.Header.Set("client_secret", "user_secret")

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("客户端凭证请求失败: %v\n", err)
		return "", ""
	}
	defer resp.Body.Close()

	// 读取响应
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)

	fmt.Printf("客户端凭证授权响应状态: %s\n", resp.Status)

	// 检查响应状态
	if resp.StatusCode != 200 {
		fmt.Printf("客户端凭证授权失败，响应内容: %s\n", buf.String())
		return "", ""
	}

	// 解析令牌响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(buf.Bytes(), &tokenResp); err != nil {
		fmt.Printf("解析客户端令牌响应失败: %v\n%s\n", err, buf.String())
		return "", ""
	}

	if tokenResp.Error != "" {
		fmt.Printf("客户端凭证授权出错: %s\n", tokenResp.Error)
		return "", ""
	}

	fmt.Printf("成功获取客户端访问令牌\n")
	return tokenResp.AccessToken.TokenValue, tokenResp.RefreshToken.TokenValue
}

// getUserAuthorizationToken 通过客户端令牌获取用户授权令牌
// 参数 clientToken: 客户端访问令牌
// 返回用户访问令牌字符串
func getUserAuthorizationToken(clientToken string) (string, string) {
	fmt.Println("\n--- 步骤2: 通过客户端授权获取用户授权 ---")

	// 准备用户登录数据 (使用JSON格式)
	loginData := map[string]interface{}{
		"username":   "admin",
		"password":   "password2",
		"grant_type": "password",
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		fmt.Printf("序列化登录数据失败: %v\n", err)
		return "", ""
	}

	// 发送POST请求到登录端点
	req, err := http.NewRequest("POST", "http://localhost:10001/oauth2/login", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("创建用户登录请求失败: %v\n", err)
		return "", ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 使用客户端令牌作为Authorization头，注意不要加"Bearer "前缀
	req.Header.Set("Authorization", clientToken)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("用户登录请求失败: %v\n", err)
		return "", ""
	}
	defer resp.Body.Close()

	// 读取响应
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)

	fmt.Printf("用户授权响应状态: %s\n", resp.Status)
	// fmt.Printf("用户授权响应内容: %s\n", buf.String())

	// 检查响应状态
	if resp.StatusCode == 200 {
		// 解析用户令牌响应
		var tokenResp TokenResponse
		if err := json.Unmarshal(buf.Bytes(), &tokenResp); err == nil {
			if tokenResp.Error == "" {
				// 同样提取refresh_token
				refreshToken := ""
				if tokenResp.RefreshToken.TokenValue != "" {
					// 如果refresh_token是字符串形式
					refreshToken = tokenResp.RefreshToken.TokenValue
				}

				fmt.Printf("成功获取用户访问令牌，刷新令牌: %s\n", refreshToken)
				return tokenResp.AccessToken.TokenValue, refreshToken
			} else {
				fmt.Printf("用户授权出错: %s\n", tokenResp.Error)
				return "", ""
			}
		}
	} else {
		fmt.Printf("用户授权失败，响应内容: %s\n", buf.String())
		return "", ""
	}
	return "", ""
}

// refreshClientToken 刷新客户端令牌
// 参数 clientToken: 客户端访问令牌
// 返回刷新后的客户端令牌
func refreshClientToken(clientToken string, refreshToken string) string {
	fmt.Println("\n--- 步骤4: 刷新客户端令牌 ---")

	// 首先需要获取原始客户端令牌中的刷新令牌
	// 在实际场景中，客户端会在初次获取令牌时保存刷新令牌
	// 这里我们简化处理，假设我们知道刷新令牌就是访问令牌本身
	// （因为JWT令牌是自包含的）

	// 准备刷新令牌数据
	refreshData := map[string]interface{}{
		"refresh_token": refreshToken, // 简化处理，实际应使用真正的刷新令牌
	}

	jsonData, err := json.Marshal(refreshData)
	if err != nil {
		fmt.Printf("序列化刷新令牌数据失败: %v\n", err)
		return ""
	}

	// 发送POST请求到客户端刷新令牌端点
	req, err := http.NewRequest("POST", "http://localhost:10001/c-auth/refresh", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("创建客户端令牌刷新请求失败: %v\n", err)
		return ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 客户端认证信息
	req.Header.Set("Authorization", clientToken) // 客户端认证信息，Base64编码的"user-svc:user_secret

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("客户端令牌刷新请求失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	// 读取响应
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)

	fmt.Printf("客户端令牌刷新响应状态: %s\n", resp.Status)

	// 检查响应状态
	if resp.StatusCode != 200 {
		fmt.Printf("客户端令牌刷新失败，响应内容: %s\n", buf.String())
		return ""
	}

	// 解析刷新令牌响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(buf.Bytes(), &tokenResp); err != nil {
		fmt.Printf("解析客户端刷新令牌响应失败: %v\n%s\n", err, buf.String())
		return ""
	}

	if tokenResp.Error != "" {
		fmt.Printf("客户端令牌刷新出错: %s\n", tokenResp.Error)
		return ""
	}

	fmt.Printf("成功刷新客户端令牌\n")
	return tokenResp.AccessToken.TokenValue
}

// refreshUserToken 刷新用户令牌
// 参数 userToken: 用户访问令牌
// 返回刷新后的用户令牌
func refreshUserToken(userToken string, refreshToken string) string {
	fmt.Println("\n--- 步骤3: 刷新用户令牌 ---")

	// 准备刷新令牌数据
	refreshData := map[string]interface{}{
		"refresh_token": refreshToken, // 简化处理，实际应使用真正的刷新令牌
	}

	jsonData, err := json.Marshal(refreshData)
	if err != nil {
		fmt.Printf("序列化登录数据失败: %v\n", err)
		return ""
	}

	// 发送POST请求到用户刷新令牌端点
	req, err := http.NewRequest("POST", "http://localhost:10001/oauth2/refresh", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("创建用户令牌刷新请求失败: %v\n", err)
		return ""
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	// 使用用户访问令牌作为Authorization头
	req.Header.Set("Authorization", userToken)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("用户令牌刷新请求失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	// 读取响应
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)

	fmt.Printf("用户令牌刷新响应状态: %s\n", resp.Status)

	// 检查响应状态
	if resp.StatusCode != 200 {
		fmt.Printf("用户令牌刷新失败，响应内容: %s\n", buf.String())
		return ""
	}

	// 解析刷新令牌响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(buf.Bytes(), &tokenResp); err != nil {
		fmt.Printf("解析用户刷新令牌响应失败: %v\n%s\n", err, buf.String())
		return ""
	}

	if tokenResp.Error != "" {
		fmt.Printf("用户令牌刷新出错: %s\n", tokenResp.Error)
		return ""
	}

	fmt.Printf("成功刷新用户令牌\n")
	return tokenResp.AccessToken.TokenValue
}