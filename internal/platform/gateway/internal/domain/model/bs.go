package model

// CircuitBreakerServiceConfig 熔断器服务配置
type CircuitBreakerServiceConfig struct {
	CounterResetInterval int64   `yaml:"counter_reset_interval"`  // 计数器重置间隔(秒)
	HalfOpenMaxSuccesses int64   `yaml:"half_open_max_successes"` // 半开状态最大成功数
	FailureRateWindow    int64   `yaml:"failure_rate_window"`     // 失败率统计窗口
	FailureRateThreshold float64 `yaml:"failure_rate_threshold"`  // 失败率阈值
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Services map[string]*CircuitBreakerServiceConfig `yaml:"services"` // 各服务的熔断器配置
}
