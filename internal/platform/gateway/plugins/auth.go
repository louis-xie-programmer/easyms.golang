package plugins

import (
	"easyms/internal/platform/gateway/plugin"
	"easyms/internal/shared/logger"
	"fmt"
	"net/http"
	"strings"
	"time"

	pb "easyms/api/proto/auth"
	"github.com/patrickmn/go-cache"
	"google.golang.org/grpc"
)

// AuthPlugin 实现了认证功能。
type AuthPlugin struct {
	authSvcClient pb.AuthServiceClient
	authCache     *cache.Cache
}

// NewAuthPlugin 创建一个新的认证插件。
// 它需要一个到认证服务的 gRPC 连接。
func NewAuthPlugin(conn *grpc.ClientConn) *AuthPlugin {
	return &AuthPlugin{
		authSvcClient: pb.NewAuthServiceClient(conn),
		authCache:     cache.New(5*time.Minute, 10*time.Minute),
	}
}

func (p *AuthPlugin) Name() string {
	return "auth"
}

func (p *AuthPlugin) Order() int {
	return 10
}

func (p *AuthPlugin) Execute(ctx *plugin.Context) {
	// 对特定路径（如健康检查）跳过认证
	if ctx.Request.URL.Path == "/health" {
		ctx.Next()
		return
	}

	authHeader := ctx.Request.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(ctx.ResponseWriter, `{"error": "Missing or invalid Authorization header"}`, http.StatusUnauthorized)
		return // 中断插件链
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	// 检查缓存
	if _, found := p.authCache.Get(tokenStr); found {
		ctx.Next() // 缓存命中，继续下一个插件
		return
	}

	// 使用 gRPC 客户端进行验证
	resp, err := p.authSvcClient.VerifyToken(ctx.Request.Context(), &pb.VerifyRequest{Token: tokenStr})
	if err != nil || !resp.Valid {
		msg := "invalid or expired token"
		if err != nil {
			logger.Warn("Auth verification failed via gRPC", "gateway", "error", err)
		}
		http.Error(ctx.ResponseWriter, fmt.Sprintf(`{"error": "%s"}`, msg), http.StatusUnauthorized)
		return // 验证失败，中断插件链
	}

	// 存入缓存
	p.authCache.Set(tokenStr, true, 5*time.Minute)

	// 继续执行下一个插件
	ctx.Next()
}
