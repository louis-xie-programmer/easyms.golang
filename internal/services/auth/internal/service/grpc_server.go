// Package service 包含了认证服务的核心业务逻辑。
package service

import (
	"context"
	auth "easyms/api/proto/auth" // 引入 gRPC 的 protobuf 定义
	"easyms/internal/services/auth/internal/consts"
	shared_model "easyms/internal/shared/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

// grpcServer 实现了由 protobuf 定义的 AuthServiceServer 接口。
// 它是认证服务 gRPC 接口的具体实现。
type grpcServer struct {
	auth.UnimplementedAuthServiceServer // 必须嵌入，以保证向前兼容性。如果未来 proto 文件增加了新的方法，旧的服务端代码不会因此编译失败。
	tokenGranter                        TokenGranter
	tokenService                        TokenService
	clientDetailsService                ClientDetailsService
}

// NewGrpcServer 创建一个新的 gRPC 服务实例。
// 它依赖于 TokenGranter, TokenService, 和 ClientDetailsService 等核心业务逻辑接口。
func NewGrpcServer(tg TokenGranter, ts TokenService, cs ClientDetailsService) auth.AuthServiceServer {
	return &grpcServer{
		tokenGranter:         tg,
		tokenService:         ts,
		clientDetailsService: cs,
	}
}

// GrantToken 实现了 gRPC 的 GrantToken 方法，用于处理令牌授予请求。
func (s *grpcServer) GrantToken(ctx context.Context, req *auth.TokenRequest) (*auth.TokenResponse, error) {
	// 1. 验证客户端信息 (Client ID 和 Client Secret)
	clientDetails, err := s.clientDetailsService.LoadClientByClientId(ctx, req.ClientId)
	if err != nil {
		// 如果客户端不存在，返回 Unauthenticated 错误
		return nil, status.Error(codes.Unauthenticated, consts.ErrInvalidClient.Error())
	}
	if clientDetails.ClientSecret != req.ClientSecret {
		// 如果客户端密钥不匹配，返回 Unauthenticated 错误
		return nil, status.Error(codes.Unauthenticated, consts.ErrInvalidClient.Error())
	}

	// 2. 将 gRPC 请求对象转换为内部业务模型对象
	tokenReq := &shared_model.TokenRequest{
		GrantType:    req.GrantType,
		Username:     req.Username,
		Password:     req.Password,
		RefreshToken: req.RefreshToken,
	}

	// 3. 调用核心的 Grant 方法，根据 grant_type 执行相应的授权逻辑
	oauth2Token, err := s.tokenGranter.Grant(ctx, req.GrantType, clientDetails, tokenReq)
	if err != nil {
		// 将业务逻辑层返回的错误转换为 gRPC 状态错误
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// 4. 将生成的令牌转换为 gRPC 响应对象
	return &auth.TokenResponse{
		AccessToken:  oauth2Token.TokenValue,
		TokenType:    "Bearer", // 令牌类型通常为 Bearer
		ExpiresIn:    int64(time.Until(*oauth2Token.ExpiresTime).Seconds()), // 计算剩余过期时间
		RefreshToken: oauth2Token.RefreshToken.TokenValue,
	}, nil
}

// VerifyToken 实现了 gRPC 的 VerifyToken 方法，用于验证令牌的有效性。
// 这个接口通常由 API 网关或其他需要验证令牌的服务调用。
func (s *grpcServer) VerifyToken(ctx context.Context, req *auth.VerifyRequest) (*auth.VerifyResponse, error) {
	// 调用核心的 TokenService 来获取令牌的详细信息
	oauth2Details, err := s.tokenService.GetOAuth2DetailsByAccessToken(ctx, req.Token)
	if err != nil {
		// 如果发生任何错误 (如令牌无效、过期、被撤销)，都返回 "Valid: false"
		return &auth.VerifyResponse{Valid: false}, nil
	}

	// 将内部的用户模型转换为 gRPC 的 User 对象
	var userDetails *auth.User
	if oauth2Details.User != nil {
		userDetails = &auth.User{
			Id:          oauth2Details.User.ID,
			Username:    oauth2Details.User.Username,
			Authorities: oauth2Details.User.Authorities,
		}
	}

	// 将内部的客户端模型转换为 gRPC 的 ClientDetails 对象
	var clientDetails *auth.ClientDetails
	if oauth2Details.Client != nil {
		clientDetails = &auth.ClientDetails{
			ClientId:                    oauth2Details.Client.ClientId,
			AccessTokenValiditySeconds:  int32(oauth2Details.Client.AccessTokenValiditySeconds),
			RefreshTokenValiditySeconds: int32(oauth2Details.Client.RefreshTokenValiditySeconds),
			AuthorizedGrantTypes:        oauth2Details.Client.AuthorizedGrantTypes,
		}
	}

	// 令牌有效，返回详细信息
	return &auth.VerifyResponse{
		Valid:  true,
		User:   userDetails,
		Client: clientDetails,
	}, nil
}
