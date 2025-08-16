package config

import (
	"net"
	"regexp"
	"sync"
)

// AppConfigStore 用于解析 app.yaml 中的 StoreType 和 Consul 配置
type AppConfigStore struct {
	StoreType string       `yaml:"store_type"`
	Consul    ConsulConfig `yaml:"consul"`
}

// ConsulConfig Consul配置信息
type ConsulConfig struct {
	Host            string `yaml:"host"`
	KeyPath         string `yaml:"key_path"`
	ReloadOnChanges bool   `yaml:"reload_on_changes"`
}

// AppConfig 定义应用核心配置
type AppConfig struct {
	Log       LogConfig        `yaml:"log,omitempty"`
	Loki      LokiConfig       `yaml:"loki,omitempty"`
	Server    ServerConfig     `yaml:"server,omitempty"`
	Database  DatabaseConfig   `yaml:"database,omitempty"`
	RateLimit *RateLimitConfig `yaml:"rate_limit" json:"rate_limit"`

	// 添加配置锁，防止并发读写
	ConfigLock sync.RWMutex `yaml:"-"`
}

// ServerConfig 定义服务器配置
type ServerConfig struct {
	Host string    `yaml:"host"`
	Port int       `yaml:"port"`
	Tls  TLSConfig `yaml:"tls"`
}

// TLSConfig 配置TLS
type TLSConfig struct {
	Enable bool   `yaml:"enable"`
	Cert   string `yaml:"cert"`
	Key    string `yaml:"key"`
}

// LogConfig 定义日志配置
type LogConfig struct {
	LogLevel string `yaml:"log_level"`
	LogType  string `yaml:"log_type"` // 日志后端类型(zerolog/loki)
}

// LokiConfig 定义Loki日志系统配置
type LokiConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// DatabaseConfig 定义数据库配置
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	UserName string `yaml:"user"`
	Password string `yaml:"password"`
}

type RateLimitConfig struct {
	IPLimits     []IPLimitRule `yaml:"ip_limits" json:"ip_limits"`
	UALimits     []UALimitRule `yaml:"ua_limits" json:"ua_limits"`
	DefaultRate  float64       `yaml:"default_rate" json:"default_rate"`
	DefaultBurst int           `yaml:"default_burst" json:"default_burst"`
}

type IPLimitRule struct {
	CIDR  string  `json:"cidr"`
	Rate  float64 `json:"rate"`
	Burst int     `json:"burst"`
	Net   *net.IPNet
}

// UserAgent限流规则
type UALimitRule struct {
	Pattern string  `json:"pattern"`
	Rate    float64 `json:"rate"`
	Burst   int     `json:"burst"`
	Regexp  *regexp.Regexp
}
