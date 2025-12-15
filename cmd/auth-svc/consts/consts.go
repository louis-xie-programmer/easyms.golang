package consts

import "errors"

const (
	OAuth2ClientDetailsKey = "OAuth2ClientDetails"
	OAuth2UserDetailsKey   = "OAuth2UserDetails"
	OAuth2ErrorKey         = "OAuth2Error"
)

var (
	// ErrNotSupportGrantType               授权类型不支持错误
	ErrNotSupportGrantType = errors.New("grant type is not supported")
	// ErrNullToken                         令牌为空错误
	ErrNullToken = errors.New("client details is null")
	// ErrInvalidClient                       客户端信息错误错误
	ErrInvalidClient = errors.New("invalid client")
	// ErrInvalidUser                         用户信息错误错误
	ErrInvalidUser = errors.New("invalid user")
	// ErrInvalidUsernameAndPasswordRequest 用户名密码错误错误
	ErrInvalidUsernameAndPasswordRequest = errors.New("invalid username, password")
	// ErrInvalidTokenRequest               令牌错误错误
	ErrInvalidTokenRequest = errors.New("invalid token")
	// ErrExpiredToken                       令牌已过期错误
	ErrExpiredToken = errors.New("token is expired")
)
