package service

import (
	"easyms/internal/shared/db"
	. "easyms/internal/shared/models"
	"fmt"

	"github.com/gofrs/uuid"
)

// ClientDetailsService 客户端详情服务接口
// 定义了客户端信息管理的标准方法
type ClientDetailsService interface {
	// LoadClientByClientId 根据客户端ID加载客户端详情
	LoadClientByClientId(clientId string) (*ClientDetails, error)
	// CreateClientDetails 创建一个新的客户端详情
	CreateClientDetails(clientId string) (*ClientDetails, error)
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
		"registered_redirect_uri, authorized_grant_types, allowed_scopes, default_scope FROM client_details WHERE client_id = '%s'", clientId)

	// 执行查询
	var client ClientDetails

	err := service.db.Query(&client, querySql)
	if err != nil {
		fmt.Printf("sql: %s\n", querySql)
		fmt.Printf("查询失败: %v\n", err)
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
		AllowedAuthorities:          client.AllowedAuthorities,
		DefaultAuthorities:          client.DefaultAuthorities,
	}

	return clientDetails, nil
}

const (
	// DefaultAccessTokenValiditySeconds 默认的访问令牌有效期，秒
	DefaultAccessTokenValiditySeconds = 3600
	// DefaultRefreshTokenValiditySeconds 默认的刷新令牌有效期，秒
	DefaultRefreshTokenValiditySeconds = 7200
	// DefaultAuthorizedGrantTypes 默认的授权类型
	DefaultAuthorizedGrantTypes = "password,refresh_token"
	// DefaultDefaultAuthorities 默认的默认权限
	DefaultDefaultAuthorities = "ROLE_USER"
	// DefaultAllowedAuthorities 默认的允许的权限
	DefaultAllowedAuthorities = "ROLE_USER"
)

func (service *PostgresClientDetailsService) CreateClientDetails(clientId string) (*ClientDetails, error) {
	clientDetails := ClientDetails{
		ClientId:                    clientId,
		ClientSecret:                CreateClientSecret(),
		AccessTokenValiditySeconds:  DefaultAccessTokenValiditySeconds,
		RefreshTokenValiditySeconds: DefaultRefreshTokenValiditySeconds,
		AuthorizedGrantTypes:        DefaultAuthorizedGrantTypes,
		AllowedAuthorities:          DefaultAllowedAuthorities,
		DefaultAuthorities:          DefaultDefaultAuthorities,
	}

	// 验证客户端信息是否已经存在
	var count int64
	err := service.db.GetDB().Table("client_details").Count(&count).Error

	if err != nil {
		fmt.Printf("查询失败: %v\n", err)
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("客户端信息已存在")
	}
	err = service.db.Insert(&clientDetails)
	if err != nil {
		fmt.Printf("插入失败: %v\n", err)
		return nil, err
	}

	return &clientDetails, nil
}

// 构建一个ClientSecret
func CreateClientSecret() string {
	uuid1, _ := uuid.NewV4()
	uuid2, _ := uuid.NewV4()
	return uuid1.String() + "-" + uuid2.String()
}
