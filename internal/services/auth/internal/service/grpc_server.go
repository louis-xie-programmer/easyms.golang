package service

import (
	"context"
	pb "easyms/api/proto/auth"
	"easyms/internal/services/auth/internal/consts"
	shared_model "easyms/internal/shared/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

// grpcServer 实现了 AuthService gRPC 服务。
type grpcServer struct {
	pb.UnimplementedAuthServiceServer // 必须嵌入，以保证向前兼容
	tokenGranter                      TokenGranter
	tokenService                      TokenService
	clientDetailsService              ClientDetailsService
}

// NewGrpcServer 创建一个新的 gRPC 服务实例。
func NewGrpcServer(tg TokenGranter, ts TokenService, cs ClientDetailsService) pb.AuthServiceServer {
	return &grpcServer{
		tokenGranter:         tg,
		tokenService:         ts,
		clientDetailsService: cs,
	}
}

// GrantToken 实现了 gRPC 的 GrantToken 方法。
func (s *grpcServer) GrantToken(ctx context.Context, req *pb.TokenRequest) (*pb.TokenResponse, error) {
	// 1. 验证客户端信息
	clientDetails, err := s.clientDetailsService.LoadClientByClientId(req.ClientId)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, consts.ErrInvalidClient.Error())
	}
	if clientDetails.ClientSecret != req.ClientSecret {
		return nil, status.Error(codes.Unauthenticated, consts.ErrInvalidClient.Error())
	}

	// 2. 将 gRPC 请求转换为内部模型
	tokenReq := &shared_model.TokenRequest{
		GrantType:    req.GrantType,
		Username:     req.Username,
		Password:     req.Password,
		RefreshToken: req.RefreshToken,
	}

	// 3. 调用核心的 Grant 方法
	oauth2Token, err := s.tokenGranter.Grant(ctx, req.GrantType, clientDetails, tokenReq)
	if err != nil {
		// 将业务错误转换为 gRPC 状态错误
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// 4. 将结果转换为 gRPC 响应
	return &pb.TokenResponse{
		AccessToken:  oauth2Token.TokenValue,
		TokenType:    "Bearer",
		ExpiresIn:    int64(oauth2Token.ExpiresTime.Sub(context.Background().Value("now").(time.Time)).Seconds()),
		RefreshToken: oauth2Token.RefreshToken.TokenValue,
	}, nil
}

// VerifyToken 实现了 gRPC 的 VerifyToken 方法。
func (s *grpcServer) VerifyToken(ctx context.Context, req *pb.VerifyRequest) (*pb.VerifyResponse, error) {
	oauth2Details, err := s.tokenService.GetOAuth2DetailsByAccessToken(req.Token)
	if err != nil {
		return &pb.VerifyResponse{Valid: false}, nil
	}

	// 转换 UserDetails
	var userDetails *pb.UserDetails
	if oauth2Details.User != nil {
		userDetails = &pb.UserDetails{
			UserId:      oauth2Details.User.UserId,
			Username:    oauth2Details.User.Username,
			Authorities: oauth2Details.User.Authorities,
		}
	}

	// 转换 ClientDetails
	var clientDetails *pb.ClientDetails
	if oauth2Details.Client != nil {
		clientDetails = &pb.ClientDetails{
			ClientId:                    oauth2Details.Client.ClientId,
			AccessTokenValiditySeconds:  int32(oauth2Details.Client.AccessTokenValiditySeconds),
			RefreshTokenValiditySeconds: int32(oauth2Details.Client.RefreshTokenValiditySeconds),
			AuthorizedGrantTypes:        oauth2Details.Client.AuthorizedGrantTypes,
		}
	}

	return &pb.VerifyResponse{
		Valid:  true,
		User:   userDetails,
		Client: clientDetails,
	}, nil
}
