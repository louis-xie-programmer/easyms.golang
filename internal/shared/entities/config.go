package entities

import (
	"fmt"
	"sync"
	"time"
)

// AppConfigStore 用于解析 app.yaml 中的 StoreType 和 Consul 配置
type AppConfigStore struct {
	Env       string       `yaml:"env"`
	StoreType string       `yaml:"store_type"`
	Consul    ConsulConfig `yaml:"consul"`
}

// ConsulConfig Consul配置信息
type ConsulConfig struct {
	Host            string `yaml:"host"`
	KeyPath         string `yaml:"key_path"`
	ReloadOnChanges bool   `yaml:"reload_on_changes"`
}

// OAuth2Config 定义OAuth2相关配置
type OAuth2Config struct {
	JWTSecret string `yaml:"jwt_secret"`
	Issuer    string `yaml:"issuer"`
}

// AppConfig 定义应用核心配置
type AppConfig struct {
	Log      LogConfig      `yaml:"log,omitempty"`
	Loki     LokiConfig     `yaml:"loki,omitempty"`
	Server   ServerConfig   `yaml:"server,omitempty"`
	Database DatabaseConfig `yaml:"database,omitempty"`

	// 添加配置锁，防止并发读写
	ConfigLock sync.RWMutex `yaml:"-"`

	OAuth2 OAuth2Config `yaml:"oauth2"`

	Cache struct {
		Redis RedisConfig `yaml:"redis,omitempty"`
	} `yaml:"cache,omitempty"`

	Redis RedisConfig `yaml:"redis,omitempty"`
}

// ServerConfig 定义服务器配置
type ServerConfig struct {
	Host string    `yaml:"host"`
	Port int       `yaml:"port"`
	Tls  TLSConfig `yaml:"tls"`
}

// Validate 验证 ServerConfig 配置的有效性
func (s *ServerConfig) Validate() error {
	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("invalid server port: %d, port must be between 1 and 65535", s.Port)
	}
	return nil
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

// RedisConfig 定义Redis配置
type RedisConfig struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	// 缓存防护配置
	NullCacheExpire int `yaml:"null_cache_expire"` // 空值缓存过期时间(秒)
	MutexExpire     int `yaml:"mutex_expire"`      // 互斥锁过期时间(秒)
}

// DatabaseConfig 定义数据库配置
type DatabaseConfig struct {
	Type     string `yaml:"type"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	UserName string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	// 连接池配置
	MaxIdleConns    int `yaml:"max_idle_conns"`     // 最大空闲连接数
	MaxOpenConns    int `yaml:"max_open_conns"`     // 最大打开连接数
	ConnMaxLifetime int `yaml:"conn_max_lifetime"`  // 连接最大生命周期(秒)
	ConnMaxIdleTime int `yaml:"conn_max_idle_time"` // 连接最大空闲时间(秒)
}

// ConfigVersion 配置版本信息
type ConfigVersion struct {
	VersionID   string    `json:"version_id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	ConfigData  string    `json:"config_data"`
}
