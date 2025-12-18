// auth.go
// 包含与认证相关的数据模型
package model

// AuthConfig 认证配置，用于网关向auth-svc进行自身认证
type AuthConfig struct {
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
}
