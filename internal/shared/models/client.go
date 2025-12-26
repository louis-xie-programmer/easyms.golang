package models

import "strings"

// ClientDetails 客户端详情模型
type ClientDetails struct {
	// client 的标识
	ClientId string `json:"client_id" gorm:"column:client_id;type:varchar(255)"`
	// client 的密钥
	ClientSecret string `json:"-" gorm:"column:client_secret;type:varchar(255)"` // 阻止密钥在 JSON 中序列化
	// 访问令牌有效时间，秒
	AccessTokenValiditySeconds int `json:"access_token_validity_seconds" gorm:"column:access_token_validity_seconds"`
	// 刷新令牌有效时间，秒
	RefreshTokenValiditySeconds int `json:"refresh_token_validity_seconds" gorm:"column:refresh_token_validity_seconds"`
	// 重定向地址，授权码类型中使用
	RegisteredRedirectUri string `json:"registered_redirect_uri" gorm:"column:registered_redirect_uri;type:varchar(255)"`
	// 可以使用的授权类型，多个类型用逗号分隔
	AuthorizedGrantTypes string `json:"authorized_grant_types" gorm:"column:authorized_grant_types;type:varchar(255)"`
	// 客户端运行的scope，多个scope用逗号分隔
	AllowedAuthorities string `json:"allowed_scopes" gorm:"column:allowed_scopes;type:varchar(255)"`
	// 默认的scope，多个scope用逗号分隔
	DefaultAuthorities string `json:"default_scope" gorm:"column:default_scope;type:varchar(255)"`
}

// TableName 设置ClientDetails模型对应的表名
func (ClientDetails) TableName() string {
	return "client_details"
}

// GetAuthorizedGrantTypes 获取授权类型列表
func (c *ClientDetails) GetAuthorizedGrantTypes() []string {
	if c.AuthorizedGrantTypes == "" {
		return []string{}
	}
	return strings.Split(c.AuthorizedGrantTypes, ",")
}

// SetAuthorizedGrantTypes 设置授权类型列表
func (c *ClientDetails) SetAuthorizedGrantTypes(types []string) {
	c.AuthorizedGrantTypes = strings.Join(types, ",")
}

// GetAuthorities 获取权限列表
func (c *ClientDetails) GetAuthorities() []string {
	if c.DefaultAuthorities == "" {
		return []string{}
	}
	return strings.Split(c.DefaultAuthorities, ",")
}

// SetAuthorities 设置权限列表
func (c *ClientDetails) SetAuthorities(authorities []string) {
	c.DefaultAuthorities = strings.Join(authorities, ",")
}

// GetAllowedScopes 获取允许的scope列表
func (c *ClientDetails) GetAllowedScopes() []string {
	if c.AllowedAuthorities == "" {
		return []string{}
	}
	return strings.Split(c.AllowedAuthorities, ",")
}

// SetAllowedScopes 设置允许的scope列表
func (c *ClientDetails) SetAllowedScopes(scopes []string) {
	c.AllowedAuthorities = strings.Join(scopes, ",")
}
