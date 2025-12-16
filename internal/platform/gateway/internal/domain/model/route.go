package model

// GatewayConfig 网关配置
type GatewayConfig struct {
	RouteRules     []*RouteRule          `yaml:"route_rules" json:"route_rules"`
	RateLimit      *RateLimitConfig      `yaml:"rate_limit" json:"rate_limit"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker" json:"circuit_breaker"`
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
