package models

import (
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"
)

// --- Service Specific Models ---

type OutboxConfig struct {
	RelayInterval time.Duration `yaml:"relay_interval"`
	BatchSize     int           `yaml:"batch_size"`
}

type ProxyConfig struct {
	ConnectTimeout        time.Duration `yaml:"connect_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	MaxIdleConns          int           `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout"`
}

type GatewayConfig struct {
	RouteRules     []*RouteRule          `yaml:"route_rules"`
	RateLimit      *RateLimitConfig      `yaml:"rate_limit"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker"`
	Auth           *AuthConfig           `yaml:"auth"`
	Proxy          *ProxyConfig          `yaml:"proxy,omitempty"`
}

// ... (rest of the file is the same)

// --- Shared Application Models ---

type AppConfig struct {
	Log      LogConfig      `yaml:"log,omitempty"`
	Loki     LokiConfig     `yaml:"loki,omitempty"`
	Server   ServerConfig   `yaml:"server,omitempty"`
	Database DatabaseConfig `yaml:"database,omitempty"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq,omitempty"`
	Tracing  TracingConfig  `yaml:"tracing,omitempty"`
	OAuth2   OAuth2Config   `yaml:"oauth2,omitempty"`
	Cache    struct {
		Redis RedisConfig `yaml:"redis,omitempty"`
	} `yaml:"cache,omitempty"`
	Consul ConsulConfig `yaml:"consul,omitempty"`

	// Service-specific configurations
	Gateway *GatewayConfig `yaml:"gateway,omitempty"`
	Outbox  *OutboxConfig  `yaml:"outbox,omitempty"` // Added Outbox config

	ConfigLock sync.RWMutex `yaml:"-"`
}

// ... (rest of the file from ServerConfig down is the same)
// I will append the rest of the file content to ensure it's complete.
type RouteRule struct {
	ServiceName   string            `yaml:"service_name"`
	PathPrefix    string            `yaml:"path_prefix"`
	StripPrefix   bool              `yaml:"strip_prefix"`
	PathRewrite   string            `yaml:"path_rewrite,omitempty"`
	RewriteTarget string            `yaml:"rewrite_target,omitempty"`
	AddHeaders    map[string]string `yaml:"add_headers,omitempty"`
	RemoveHeaders []string          `yaml:"remove_headers,omitempty"`
}

type RateLimitConfig struct {
	IPLimits     []IPLimitRule `yaml:"ip_limits"`
	UALimits     []UALimitRule `yaml:"ua_limits"`
	DefaultRate  float64       `yaml:"default_rate"`
	DefaultBurst int           `yaml:"default_burst"`
}

type IPLimitRule struct {
	CIDR  string     `yaml:"cidr"`
	Rate  float64    `yaml:"rate"`
	Burst int        `yaml:"burst"`
	Net   *net.IPNet `yaml:"-"`
}

type UALimitRule struct {
	Pattern string         `yaml:"pattern"`
	Rate    float64        `yaml:"rate"`
	Burst   int            `yaml:"burst"`
	Regexp  *regexp.Regexp `yaml:"-"`
}

type CircuitBreakerConfig struct {
	Services map[string]*CircuitBreakerServiceConfig `yaml:"services"`
}

type CircuitBreakerServiceConfig struct {
	CounterResetInterval int64   `yaml:"counter_reset_interval"`
	HalfOpenMaxSuccesses int64   `yaml:"half_open_max_successes"`
	FailureRateWindow    int64   `yaml:"failure_rate_window"`
	FailureRateThreshold float64 `yaml:"failure_rate_threshold"`
}

type AuthConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type OAuth2Config struct {
	JWTSecret string `yaml:"jwt_secret"`
	Issuer    string `yaml:"issuer"`
}

type AppConfigStore struct {
	Env       string       `yaml:"env"`
	StoreType string       `yaml:"store_type"`
	Consul    ConsulConfig `yaml:"consul"`
}

type ConsulConfig struct {
	Host            string `yaml:"host"`
	KeyPath         string `yaml:"key_path"`
	ReloadOnChanges bool   `yaml:"reload_on_changes"`
}

type ServerConfig struct {
	Host     string    `yaml:"host"`
	Port     int       `yaml:"port"`
	GrpcPort int       `yaml:"grpc_port"`
	Tls      TLSConfig `yaml:"tls"`
}

func (s *ServerConfig) Validate() error {
	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", s.Port)
	}
	if s.GrpcPort != 0 && (s.GrpcPort <= 0 || s.GrpcPort > 65535) {
		return fmt.Errorf("invalid gRPC port: %d", s.GrpcPort)
	}
	return nil
}

type TLSConfig struct {
	Enable bool   `yaml:"enable"`
	Cert   string `yaml:"cert"`
	Key    string `yaml:"key"`
}

type LogConfig struct {
	LogLevel string `yaml:"log_level"`
	LogType  string `yaml:"log_type"`
}

type LokiConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type RedisConfig struct {
	Address         string `yaml:"address"`
	Password        string `yaml:"password"`
	DB              int    `yaml:"db"`
	NullCacheExpire int    `yaml:"null_cache_expire"`
	MutexExpire     int    `yaml:"mutex_expire"`
}

type DatabaseConfig struct {
	Type            string `yaml:"type"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	UserName        string `yaml:"user"`
	Password        string `yaml:"password"`
	Database        string `yaml:"database"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime int    `yaml:"conn_max_idle_time"`
}

type RabbitMQConfig struct {
	URL string `yaml:"url"`
}

type TracingConfig struct {
	Enable   bool   `yaml:"enable"`
	Endpoint string `yaml:"endpoint"`
}

type ConfigVersion struct {
	VersionID   string    `json:"version_id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	ConfigData  string    `json:"config_data"`
}
