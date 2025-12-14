package service

import (
	"context"
	"errors"
	. "easyms/cmd/auth-svc/model"
	"easyms/pkg/db"
)

var (
	// ErrClientNotExist 客户端不存在
	ErrClientNotExist = errors.New("client is not exist")
	// ErrClientSecret 客户端密钥错误
	ErrClientSecret = errors.New("invalid client secret")
)

type ClientDetailsService interface {
	// GetClientDetailByClientId 根据客户端ID和客户端密钥获取客户端详细信息
	GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*ClientDetails, error)
}

// PostgresClientDetailsService postgres客户端详情服务
type PostgresClientDetailsService struct {
	db *db.EasyDatabase
}

func NewPostgresClientDetailsService(db *db.EasyDatabase) ClientDetailsService {
	return &PostgresClientDetailsService{
		db: db,
	}
}

// GetClientDetailByClientId 根据客户端ID和客户端密钥获取客户端详细信息
func (service *PostgresClientDetailsService) GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*ClientDetails, error) {
	var clientDetails ClientDetails
	err := service.db.Query(&clientDetails, "select * from client_details where client_id = ? and client_secret = ?", clientId, clientSecret)
	if err != nil {
		return nil, err
	}
	return &clientDetails, nil
}

// InMemoryClientDetailsService 内存中的客户端详细信息服务
type InMemoryClientDetailsService struct {
	clientDetailsDict map[string]*ClientDetails
}

// GetClientDetailByClientId 根据客户端ID和客户端密钥获取客户端详细信息
func (service *InMemoryClientDetailsService) GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*ClientDetails, error) {
	// 根据 clientId 获取 clientDetails
	clientDetails, ok := service.clientDetailsDict[clientId]
	if ok {
		// 比较 clientSecret 是否正确
		if clientDetails.ClientSecret == clientSecret {
			return clientDetails, nil
		} else {
			return nil, ErrClientSecret
		}
	} else {
		return nil, ErrClientNotExist
	}
}