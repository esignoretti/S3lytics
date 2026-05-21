package auth

import (
	"github.com/esignoretti/s3lytics/internal/store"
)

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TfaCode  string `json:"tfa_code,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	APIURL   string `json:"api_url,omitempty"`
}

type IAMChallengeResponse struct {
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
}

type IAMSigninResponse struct {
	JWT          string `json:"jwt"`
	RefreshToken string `json:"_refresh"`
}

type IAMAccount struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	EndpointGateway string `json:"endpoint_gateway"`
}

type IAMProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type IAMForgeJWTResponse struct {
	JWT string `json:"jwt"`
}

type IAMApiKey struct {
	ApiKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

type Service struct {
	client *httpClient
	store  store.Store
}

func NewService(s store.Store) *Service {
	return &Service{
		client: &httpClient{},
		store:  s,
	}
}
