// Package service 包含了认证服务的核心业务逻辑。
package service

import (
	"context"
	"easyms/internal/shared/db"
	"easyms/internal/shared/logger"
	. "easyms/internal/shared/models"
	"fmt"

	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// ClientDetailsService 定义了客户端详情服务的接口。
// 在 OAuth2 术语中，"客户端" 指的是请求访问受保护资源的应用程序 (例如前端 Web 应用、移动 App)。
type ClientDetailsService interface {
	// LoadClientByClientId 根据客户端 ID 加载客户端的详细信息。
	LoadClientByClientId(ctx context.Context, clientId string) (*ClientDetails, error)
	// CreateClientDetails 创建一个新的客户端详情记录。
	CreateClientDetails(ctx context.Context, clientId string) (*ClientDetails, error)
}

// PostgresClientDetailsService 是 ClientDetailsService 接口基于 PostgreSQL 的实现。
type PostgresClientDetailsService struct {
	db db.Database
}

// NewPostgresClientDetailsService 创建一个新的 PostgresClientDetailsService 实例。
func NewPostgresClientDetailsService(db db.Database) ClientDetailsService {
	return &PostgresClientDetailsService{db: db}
}

// LoadClientByClientId 根据客户端 ID 从数据库中加载客户端详情。
// 这是认证流程中的关键一步，用于验证发起请求的客户端是否合法。
func (service *PostgresClientDetailsService) LoadClientByClientId(ctx context.Context, clientId string) (*ClientDetails, error) {
	var client ClientDetails
	// 使用 GORM 的 First 方法进行查询
	err := service.db.GetDB().WithContext(ctx).Where("client_id = ?", clientId).First(&client).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("ID 为 %s 的客户端未找到", clientId)
		}
		logger.Error(err, "查询客户端详情失败", "auth-svc", "clientId", clientId)
		return nil, err
	}
	return &client, nil
}

// 定义了创建新客户端时的默认值常量
const (
	DefaultAccessTokenValiditySeconds  = 3600     // 默认的访问令牌有效期 (1小时)
	DefaultRefreshTokenValiditySeconds = 7200     // 默认的刷新令牌有效期 (2小时)
	DefaultAuthorizedGrantTypes        = "password,refresh_token" // 默认支持的授权类型
	DefaultDefaultAuthorities          = "ROLE_USER" // 默认的权限
	DefaultAllowedAuthorities          = "ROLE_USER" // 允许的权限
)

// CreateClientDetails 在数据库中创建一个新的客户端记录。
// 通常在系统初始化或后台管理时调用。
func (service *PostgresClientDetailsService) CreateClientDetails(ctx context.Context, clientId string) (*ClientDetails, error) {
	// 1. 检查客户端 ID 是否已存在
	var count int64
	err := service.db.GetDB().WithContext(ctx).Model(&ClientDetails{}).Where("client_id = ?", clientId).Count(&count).Error
	if err != nil {
		logger.Error(err, "检查客户端是否存在失败", "auth-svc", "clientId", clientId)
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("ID 为 %s 的客户端已存在", clientId)
	}

	// 2. 构建新的客户端详情对象，并设置默认值
	clientDetails := ClientDetails{
		ClientId:                    clientId,
		ClientSecret:                CreateClientSecret(), // 生成一个随机的客户端密钥
		AccessTokenValiditySeconds:  DefaultAccessTokenValiditySeconds,
		RefreshTokenValiditySeconds: DefaultRefreshTokenValiditySeconds,
		AuthorizedGrantTypes:        DefaultAuthorizedGrantTypes,
		AllowedAuthorities:          DefaultAllowedAuthorities,
		DefaultAuthorities:          DefaultDefaultAuthorities,
	}

	// 3. 将新客户端插入数据库
	err = service.db.Insert(ctx, &clientDetails)
	if err != nil {
		logger.Error(err, "插入新客户端失败", "auth-svc", "clientId", clientId)
		return nil, err
	}

	return &clientDetails, nil
}

// CreateClientSecret 生成一个随机的、唯一的客户端密钥。
func CreateClientSecret() string {
	uuid1, _ := uuid.NewV4()
	uuid2, _ := uuid.NewV4()
	return uuid1.String() + "-" + uuid2.String()
}
