package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

func (s *Service) GetAccount(ctx context.Context, jwt string) (*IAMAccount, error) {
	data, err := s.client.doRequest(ctx, "GET", "/iam/v1/accounts/me", nil, jwt)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	account := &IAMAccount{}
	if err := json.Unmarshal(data, account); err != nil {
		return nil, fmt.Errorf("parse account: %w", err)
	}
	return account, nil
}

func (s *Service) GetProjects(ctx context.Context, jwt string) ([]IAMProject, error) {
	data, err := s.client.doRequest(ctx, "GET", "/composer-hub/v1/projects", nil, jwt)
	if err != nil {
		return nil, fmt.Errorf("get projects: %w", err)
	}
	preview := string(data)
	if len(preview) > 1000 {
		preview = preview[:1000] + "...(truncated)"
	}
	fmt.Printf("auth: /composer-hub/v1/projects raw response: %s\n", preview)
	var projects []IAMProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("parse projects: %w", err)
	}
	return projects, nil
}

// RefreshToken calls GET /iam/v1/auth/refresh/access. The _refresh cookie
// is sent automatically by the cookie jar; the server rotates it on the response.
func (s *Service) RefreshToken(ctx context.Context) (string, error) {
	data, err := s.client.doRequest(ctx, "GET", "/iam/v1/auth/refresh/access", nil, "")
	if err != nil {
		return "", fmt.Errorf("refresh token: %w", err)
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse refresh response: %w", err)
	}
	return resp.Token, nil
}

// ForgeJWT calls GET /iam/v1/auth/forge/access?user_id=... using the _refresh
// cookie (sent by the jar) for authentication.
func (s *Service) ForgeJWT(ctx context.Context, userID string) (*IAMForgeJWTResponse, error) {
	path := "/iam/v1/auth/forge/access?" + url.Values{"user_id": {userID}}.Encode()
	data, err := s.client.doRequest(ctx, "GET", path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("forge jwt: %w", err)
	}
	resp := &IAMForgeJWTResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, fmt.Errorf("parse forge jwt: %w", err)
	}
	return resp, nil
}

// ListApiKeys lists API keys for a given IAM user, using a forge JWT as Bearer.
func (s *Service) ListApiKeys(ctx context.Context, userID, forgeJWT string) ([]IAMApiKey, error) {
	path := "/keyvault/api/v3/keys?" + url.Values{"user_id": {userID}}.Encode()
	data, err := s.client.doRequest(ctx, "GET", path, nil, forgeJWT)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	var keys []IAMApiKey
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse api keys: %w", err)
	}
	return keys, nil
}

// CreateApiKey creates a new API key for a given IAM user, using a forge JWT
// as Bearer. Per Swift reference: POST /keyvault/api/v3/keys/<name>?user_id=...
// with NO body. Returns the key including the secret (only revealed here).
func (s *Service) CreateApiKey(ctx context.Context, name, userID, forgeJWT string) (*IAMApiKey, error) {
	path := "/keyvault/api/v3/keys/" + url.PathEscape(name) + "?" + url.Values{"user_id": {userID}}.Encode()
	data, err := s.client.doRequest(ctx, "POST", path, nil, forgeJWT)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	var key IAMApiKey
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("parse api key: %w", err)
	}
	return &key, nil
}

// DeleteApiKey removes a key by its api_key (public) value.
func (s *Service) DeleteApiKey(ctx context.Context, apiKey, userID, forgeJWT string) error {
	path := "/keyvault/api/v3/keys/" + url.PathEscape(apiKey) + "?" + url.Values{"user_id": {userID}}.Encode()
	_, err := s.client.doRequest(ctx, "DELETE", path, nil, forgeJWT)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	return nil
}
