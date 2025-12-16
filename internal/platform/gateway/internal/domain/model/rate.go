package model

import (
	"net"
	"regexp"
)

// RateLimitConfig 限流配置
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

type UALimitRule struct {
	Pattern string  `json:"pattern"`
	Rate    float64 `json:"rate"`
	Burst   int     `json:"burst"`
	Regexp  *regexp.Regexp
}
