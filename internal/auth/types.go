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

type challengeRequest struct {
	Email    string `json:"email"`
	TenantID string `json:"tenant_id,omitempty"`
}

type signinRequest struct {
	Email           string `json:"email"`
	SignedChallenge string `json:"signed_challenge"`
	TfaCode         string `json:"tfa_code,omitempty"`
	TenantID        string `json:"tenant_id,omitempty"`
}

type IAMChallengeResponse struct {
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
}

type IAMSigninResponse struct {
	Token        string `json:"token"`
	Exp          int64  `json:"exp"`
	ExpDate      string `json:"exp_date"`
	RefreshToken string `json:"-"`
}

type IAMAccount struct {
	ID              string     `json:"id"`
	EndpointGateway string     `json:"endpoint_gateway"`
	Emails          []IAMEmail `json:"emails"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	TenantID        string     `json:"tenant_id"`
}

type IAMEmail struct {
	Email     string `json:"email"`
	IsDefault bool   `json:"default"`
}

type IAMProject struct {
	ID    string    `json:"project_id"`
	Name  string    `json:"project_name"`
	Users []IAMUser `json:"users"`
}

type IAMUser struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	IsRoot   bool   `json:"is_root"`
}

type IAMForgeJWTResponse struct {
	Token   string `json:"token"`
	Exp     int64  `json:"exp"`
	ExpDate string `json:"exp_date"`
}

type IAMApiKey struct {
	Name      string `json:"name"`
	ApiKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

type Service struct {
	client *httpClient
	store  store.Store
}

func NewService(s store.Store) *Service {
	return &Service{
		client: newHTTPClient(),
		store:  s,
	}
}

// RestoreSession seeds the client with a coordinator URL and refresh cookie
// recovered from persistent storage, so refresh/forge work after app restart.
func (s *Service) RestoreSession(coordinatorURL, refreshToken string) {
	if coordinatorURL != "" {
		s.client.SetBaseURL(coordinatorURL)
	}
	s.client.RestoreRefreshCookie(refreshToken)
}

// CurrentRefreshToken returns the latest _refresh cookie value observed
// across all auth calls (rotated by the server on every refresh/forge).
func (s *Service) CurrentRefreshToken() string {
	return s.client.GetRefreshCookie()
}
