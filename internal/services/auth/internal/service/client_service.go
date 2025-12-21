package service

import (
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	. "easyms/internal/shared/models"
	"fmt"

	"github.com/gofrs/uuid"
	"gorm.io/gorm"
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
	var client ClientDetails
	err := service.db.GetDB().Where("client_id = ?", clientId).First(&client).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("client with id %s not found", clientId)
		}
		logger.Error(err, "Failed to query client details", "auth-svc", "clientId", clientId)
		return nil, err
	}
	return &client, nil
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
	var count int64
	err := service.db.GetDB().Model(&ClientDetails{}).Where("client_id = ?", clientId).Count(&count).Error
	if err != nil {
		logger.Error(err, "Failed to check if client exists", "auth-svc", "clientId", clientId)
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("client with id %s already exists", clientId)
	}

	clientDetails := ClientDetails{
		ClientId:                    clientId,
		ClientSecret:                CreateClientSecret(),
		AccessTokenValiditySeconds:  DefaultAccessTokenValiditySeconds,
		RefreshTokenValiditySeconds: DefaultRefreshTokenValiditySeconds,
		AuthorizedGrantTypes:        DefaultAuthorizedGrantTypes,
		AllowedAuthorities:          DefaultAllowedAuthorities,
		DefaultAuthorities:          DefaultDefaultAuthorities,
	}

	err = service.db.Insert(&clientDetails)
	if err != nil {
		logger.Error(err, "Failed to insert new client", "auth-svc", "clientId", clientId)
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
