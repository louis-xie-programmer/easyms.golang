package service

import (
	"fmt"
	. "easyms/cmd/auth-svc/model"
	"easyms/pkg/db"
)

// ClientDetailsService 客户端详情服务接口
// 定义了客户端信息管理的标准方法
type ClientDetailsService interface {
	// LoadClientByClientId 根据客户端ID加载客户端详情
	LoadClientByClientId(clientId string) (*ClientDetails, error)
}

// PostgresClientDetailsService 基于PostgreSQL的客户端详情服务实现
type PostgresClientDetailsService struct {
	db db.Database
}

// NewPostgresClientDetailsService 创建新的PostgreSQL客户端详情服务实例
// 参数:
//   - db: 数据库实例
//
// 返回值:
//   - ClientDetailsService: 客户端详情服务实例
func NewPostgresClientDetailsService(db db.Database) ClientDetailsService {
	return &PostgresClientDetailsService{db: db}
}

// LoadClientByClientId 根据客户端ID加载客户端详情
// 参数:
//   - clientId: 客户端ID
//
// 返回值:
//   - *ClientDetails: 客户端详情
//   - error: 操作成功返回nil，失败返回具体错误
func (service *PostgresClientDetailsService) LoadClientByClientId(clientId string) (*ClientDetails, error) {
	// 构造查询SQL
	querySql := fmt.Sprintf("SELECT client_id, client_secret, access_token_validity_seconds, refresh_token_validity_seconds,"+
		"registered_redirect_uri, authorized_grant_types FROM client_details WHERE client_id = '%s'", clientId)

	// 执行查询
	var client ClientDetails
	err := service.db.Query(&client, querySql)
	if err != nil {
		return nil, err
	}

	// 构造客户端详情对象
	clientDetails := &ClientDetails{
		ClientId:                    client.ClientId,
		ClientSecret:                client.ClientSecret,
		AccessTokenValiditySeconds:  client.AccessTokenValiditySeconds,
		RefreshTokenValiditySeconds: client.RefreshTokenValiditySeconds,
		RegisteredRedirectUri:       client.RegisteredRedirectUri,
		AuthorizedGrantTypes:        client.AuthorizedGrantTypes,
	}

	return clientDetails, nil
}