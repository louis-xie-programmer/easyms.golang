package service

import (
	"context"
	"easyms/cmd/auth-svc/model"
	"easyms/pkg/db"
	"errors"
)

var (
	// ErrClientNotExist 客户端不存在错误
	ErrClientNotExist = errors.New("clientId is not exist")
	// ErrClientSecret 客户端密钥错误错误
	ErrClientSecret = errors.New("invalid clientSecret")
)

// ClientDetailsService Service Define a service interface
type ClientDetailsService interface {
	// GetClientDetailByClientId 根据客户端ID和客户端密钥获取客户端详细信息
	GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*model.ClientDetails, error)
}

// NewInMemoryClientDetailsService 创建内存中的客户端详细信息服务实例
func NewInMemoryClientDetailsService(clientDetailsDict map[string]*model.ClientDetails) ClientDetailsService {
	return &InMemoryClientDetailsService{
		clientDetailsDict: clientDetailsDict,
	}
}

// PostgresClientDetailsService 创建 Postgres 数据库客户端详细信息服务实例
type PostgresClientDetailsService struct {
	db *db.EasyDatabase
}

// NewPostgresClientDetailsService 创建 Postgres 数据库客户端详细信息服务实例
func NewPostgresClientDetailsService(db *db.EasyDatabase) ClientDetailsService {
	return &PostgresClientDetailsService{
		db: db,
	}
}

// GetClientDetailByClientId 根据客户端ID和客户端密钥获取客户端详细信息
func (service *PostgresClientDetailsService) GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*model.ClientDetails, error) {
	var clientDetails model.ClientDetails
	err := service.db.Query(&clientDetails, "select * from client_details where client_id = ? and client_secret = ?", clientId, clientSecret)
	if err != nil {
		return nil, err
	}
	return &clientDetails, nil
}

// InMemoryClientDetailsService 内存中的客户端详细信息服务
type InMemoryClientDetailsService struct {
	clientDetailsDict map[string]*model.ClientDetails
}

// GetClientDetailByClientId 根据客户端ID和客户端密钥获取客户端详细信息
func (service *InMemoryClientDetailsService) GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*model.ClientDetails, error) {
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
