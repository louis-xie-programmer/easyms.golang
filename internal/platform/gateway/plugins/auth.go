// Package plugins 包含了 API 网关的所有插件实现。
package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/logger"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "easyms/api/proto/auth" // 引入认证服务的 protobuf 定义
	"github.com/patrickmn/go-cache"
	"google.golang.org/grpc"
)

// AuthConfig 保存了认证插件所需的配置。
type AuthConfig struct {
	Scheme               string        // 认证方案，例如 "Bearer"
	CacheExpiration      time.Duration // 认证成功结果的缓存过期时间
	CacheCleanupInterval time.Duration // 缓存清理间隔
	SkipPaths            []string      // 需要跳过认证的路径列表
}

// AuthPlugin 实现了认证插件。
// 它负责从请求中提取令牌，通过 gRPC 调用认证服务进行验证，
// 并将验证结果缓存起来，同时将用户信息注入到下游请求的 Header 中。
type AuthPlugin struct {
	authSvcClient pb.AuthServiceClient // 认证服务的 gRPC 客户端
	authCache     *cache.Cache         // 用于缓存令牌验证结果的本地内存缓存
	config        AuthConfig
}

// NewAuthPlugin 创建一个新的认证插件实例。
// conn: 到认证服务的 gRPC 连接。
// config: 认证插件的配置。
func NewAuthPlugin(conn *grpc.ClientConn, config AuthConfig) *AuthPlugin {
	// 如果未配置，则提供默认的缓存参数
	if config.CacheExpiration == 0 {
		config.CacheExpiration = 5 * time.Minute
	}
	if config.CacheCleanupInterval == 0 {
		config.CacheCleanupInterval = 10 * time.Minute
	}
	if config.Scheme == "" {
		config.Scheme = "Bearer" // 默认认证方案
	}

	return &AuthPlugin{
		authSvcClient: pb.NewAuthServiceClient(conn),
		authCache:     cache.New(config.CacheExpiration, config.CacheCleanupInterval),
		config:        config,
	}
}

// Name 返回插件的名称。
func (p *AuthPlugin) Name() string {
	return "auth"
}

// Order 返回插件的执行顺序。
// 10 表示它在路由和代理等插件之前执行。
func (p *AuthPlugin) Order() int {
	return 10
}

// Execute 是认证插件的核心逻辑。
func (p *AuthPlugin) Execute(ctx *plugin.Context) {
	// 1. 检查当前请求路径是否在跳过认证的列表中
	for _, path := range p.config.SkipPaths {
		if ctx.Request.URL.Path == path {
			ctx.Next() // 跳过认证，继续执行下一个插件
			return
		}
	}

	// 2. 从 Authorization Header 中提取令牌
	authHeader := ctx.Request.Header.Get("Authorization")
	authScheme := p.config.Scheme + " "
	if !strings.HasPrefix(authHeader, authScheme) {
		// 如果 Header 不存在或格式不正确，返回 401 Unauthorized
		http.Error(ctx.ResponseWriter, `{"error": "缺少或无效的认证头"}`, http.StatusUnauthorized)
		return // 中断插件链
	}
	tokenStr := strings.TrimPrefix(authHeader, authScheme)

	// 3. 检查本地缓存
	if cachedResp, found := p.authCache.Get(tokenStr); found {
		if resp, ok := cachedResp.(*pb.VerifyResponse); ok && resp.User != nil {
			// 缓存命中，将用户信息注入 Header
			p.injectHeaders(ctx, resp.User)
		}
		ctx.Next() // 继续执行下一个插件
		return
	}

	// 4. 缓存未命中，通过 gRPC 调用认证服务进行验证
	resp, err := p.authSvcClient.VerifyToken(ctx.Request.Context(), &pb.VerifyRequest{Token: tokenStr})
	if err != nil || !resp.Valid {
		msg := "无效或已过期的令牌"
		if err != nil {
			logger.Warn("通过 gRPC 进行认证验证失败", "gateway", "error", err)
		}
		writeJSONError(ctx.ResponseWriter, http.StatusUnauthorized, msg)
		return // 验证失败，中断插件链
	}

	// 5. 将成功的验证结果存入缓存
	p.authCache.Set(tokenStr, resp, p.config.CacheExpiration)

	// 6. 将用户信息注入到下游请求的 Header 中
	if resp.User != nil {
		p.injectHeaders(ctx, resp.User)
	}

	// 7. 继续执行下一个插件
	ctx.Next()
}

// injectHeaders 将用户信息注入到请求的 Header 中，以便下游服务可以识别用户身份。
func (p *AuthPlugin) injectHeaders(ctx *plugin.Context, user *pb.User) {
	ctx.Request.Header.Set("X-User-Id", strconv.FormatInt(user.Id, 10))
	ctx.Request.Header.Set("X-User-Username", user.Username)
	ctx.Request.Header.Set("X-User-Authorities", user.Authorities)
}
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error": "%s"}`, msg)
}
