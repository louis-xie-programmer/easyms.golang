package entities

import (
	"fmt"
	"net"
	"regexp"
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

// CircuitBreakerServiceConfig 熔断器服务配置
type CircuitBreakerServiceConfig struct {
	CounterResetInterval int64   `yaml:"counter_reset_interval"` // 计数器重置间隔(秒)
	HalfOpenMaxSuccesses int64   `yaml:"half_open_max_successes"` // 半开状态最大成功数
	FailureRateWindow    int64   `yaml:"failure_rate_window"`     // 失败率统计窗口
	FailureRateThreshold float64 `yaml:"failure_rate_threshold"`  // 失败率阈值
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Services map[string]*CircuitBreakerServiceConfig `yaml:"services"` // 各服务的熔断器配置
}

// RouteRule 路由规则配置
type RouteRule struct {
	ServiceName   string            `yaml:"service_name" json:"service_name"`
	PathPrefix    string            `yaml:"path_prefix" json:"path_prefix"`
	StripPrefix   bool              `yaml:"strip_prefix" json:"strip_prefix"`
	PathRewrite   string            `yaml:"path_rewrite" json:"path_rewrite"`
	RewriteTarget string            `yaml:"rewrite_target" json:"rewrite_target"`
	AddHeaders    map[string]string `yaml:"add_headers" json:"add_headers"`
	RemoveHeaders []string          `yaml:"remove_headers" json:"remove_headers"`
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	RouteRules []*RouteRule `yaml:"route_rules" json:"route_rules"`
}

// AppConfig 定义应用核心配置
type AppConfig struct {
	Log            LogConfig            `yaml:"log,omitempty"`
	Loki           LokiConfig           `yaml:"loki,omitempty"`
	Server         ServerConfig         `yaml:"server,omitempty"`
	Database       DatabaseConfig       `yaml:"database,omitempty"`
	RateLimit      *RateLimitConfig     `yaml:"rate_limit" json:"rate_limit"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker" json:"circuit_breaker"`
	Gateway        *GatewayConfig       `yaml:"gateway" json:"gateway"`

	// 添加配置锁，防止并发读写
	ConfigLock sync.RWMutex `yaml:"-"`

	OAuth2 OAuth2Config `yaml:"oauth2"`
}

// Validate 验证 AppConfig 配置的有效性
func (c *AppConfig) Validate(isGateway bool) error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	// 网关服务不需要数据库配置
	if !isGateway {
		if err := c.Database.Validate(); err != nil {
			return fmt.Errorf("database config validation failed: %w", err)
		}
	}

	if c.RateLimit != nil {
		if err := c.RateLimit.Validate(); err != nil {
			return fmt.Errorf("rate limit config validation failed: %w", err)
		}
	}

	return nil
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

// Validate 验证 DatabaseConfig 配置的有效性
func (d *DatabaseConfig) Validate() error {
	if d.Type == "" {
		return fmt.Errorf("database type is required")
	}

	if d.Host == "" {
		return fmt.Errorf("database host is required")
	}

	if d.Port <= 0 || d.Port > 65535 {
		return fmt.Errorf("invalid database port: %d, port must be between 1 and 65535", d.Port)
	}

	if d.UserName == "" {
		return fmt.Errorf("database username is required")
	}

	if d.Database == "" {
		return fmt.Errorf("database name is required")
	}

	return nil
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	IPLimits     []IPLimitRule `yaml:"ip_limits" json:"ip_limits"`
	UALimits     []UALimitRule `yaml:"ua_limits" json:"ua_limits"`
	DefaultRate  float64       `yaml:"default_rate" json:"default_rate"`
	DefaultBurst int           `yaml:"default_burst" json:"default_burst"`
}

// Validate 验证 RateLimitConfig 配置的有效性
func (r *RateLimitConfig) Validate() error {
	if r.DefaultRate < 0 {
		return fmt.Errorf("default rate must be non-negative, got: %f", r.DefaultRate)
	}

	if r.DefaultBurst < 0 {
		return fmt.Errorf("default burst must be non-negative, got: %d", r.DefaultBurst)
	}

	if r.DefaultBurst < int(r.DefaultRate) {
		return fmt.Errorf("default burst (%d) must be greater than or equal to default rate (%f)", r.DefaultBurst, r.DefaultRate)
	}

	for i, rule := range r.IPLimits {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("ip limit rule #%d validation failed: %w", i+1, err)
		}
	}

	for i, rule := range r.UALimits {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("ua limit rule #%d validation failed: %w", i+1, err)
		}
	}

	return nil
}

type IPLimitRule struct {
	CIDR  string  `json:"cidr"`
	Rate  float64 `json:"rate"`
	Burst int     `json:"burst"`
	Net   *net.IPNet
}

// Validate 验证 IPLimitRule 配置的有效性
func (i *IPLimitRule) Validate() error {
	if i.CIDR == "" {
		return fmt.Errorf("cidr is required")
	}

	if _, _, err := net.ParseCIDR(i.CIDR); err != nil {
		return fmt.Errorf("invalid cidr format: %s, error: %w", i.CIDR, err)
	}

	if i.Rate < 0 {
		return fmt.Errorf("rate must be non-negative, got: %f", i.Rate)
	}

	if i.Burst < 0 {
		return fmt.Errorf("burst must be non-negative, got: %d", i.Burst)
	}

	if i.Burst < int(i.Rate) {
		return fmt.Errorf("burst (%d) must be greater than or equal to rate (%f)", i.Burst, i.Rate)
	}

	return nil
}

// UserAgent限流规则
type UALimitRule struct {
	Pattern string  `json:"pattern"`
	Rate    float64 `json:"rate"`
	Burst   int     `json:"burst"`
	Regexp  *regexp.Regexp
}

// Validate 验证 UALimitRule 配置的有效性
func (u *UALimitRule) Validate() error {
	if u.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}

	if _, err := regexp.Compile(u.Pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %s, error: %w", u.Pattern, err)
	}

	if u.Rate < 0 {
		return fmt.Errorf("rate must be non-negative, got: %f", u.Rate)
	}

	if u.Burst < 0 {
		return fmt.Errorf("burst must be non-negative, got: %d", u.Burst)
	}

	if u.Burst < int(u.Rate) {
		return fmt.Errorf("burst (%d) must be greater than or equal to rate (%f)", u.Burst, u.Rate)
	}

	return nil
}

// ConfigVersion 配置版本信息
type ConfigVersion struct {
	VersionID   string    `json:"version_id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	ConfigData  string    `json:"config_data"`
}