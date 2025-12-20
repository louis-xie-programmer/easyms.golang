package model

type CheckTokenRequest struct {
	Token         string `json:"token"`
	ClientDetails ClientDetails
}

type CheckTokenResponse struct {
	OAuthDetails *OAuth2Details `json:"o_auth_details"`
	Error        string         `json:"error"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type TokenResponse struct {
	AccessToken  *OAuth2Token `json:"access_token"`
	RefreshToken *OAuth2Token `json:"refresh_token,omitempty"`
	Error        string       `json:"error"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterClientRequest struct {
	ClientId string `json:"client_id"`
}

type RegisterClientResponse struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type RegisterUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ClientTokenRequest struct {
	GrantType    string `json:"grant_type"`
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	RefreshToken string `json:"refresh_token"`
}
