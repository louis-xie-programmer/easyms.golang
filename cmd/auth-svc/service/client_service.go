package service

import (
	"context"
	"errors"
	"github.com/louis-xie-programmer/easyms/cmd/auth-svc/model"
	"github.com/louis-xie-programmer/easyms/pkg/db"
)

var (
	ErrClientNotExist = errors.New("clientId is not exist")
	ErrClientSecret   = errors.New("invalid clientSecret")
)

// ClientDetailsService Service Define a service interface
type ClientDetailsService interface {
	GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*model.ClientDetails, error)
}

func NewInMemoryClientDetailsService(clientDetailsDict map[string]*model.ClientDetails) ClientDetailsService {
	return &InMemoryClientDetailsService{
		clientDetailsDict: clientDetailsDict,
	}
}

type PostgresClientDetailsService struct {
	db *db.EasyDatabase
}

func NewPostgresClientDetailsService(db *db.EasyDatabase) ClientDetailsService {
	return &PostgresClientDetailsService{
		db: db,
	}
}

func (service *PostgresClientDetailsService) GetClientDetailByClientId(ctx context.Context, clientId string, clientSecret string) (*model.ClientDetails, error) {
	var clientDetails model.ClientDetails
	err := service.db.Query(&clientDetails, "select * from client_details where client_id = ? and client_secret = ?", clientId, clientSecret)
	if err != nil {
		return nil, err
	}
	return &clientDetails, nil
}

type InMemoryClientDetailsService struct {
	clientDetailsDict map[string]*model.ClientDetails
}

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
