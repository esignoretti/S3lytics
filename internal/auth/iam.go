package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

func (s *Service) GetAccount(ctx context.Context, jwt string) (*IAMAccount, error) {
	data, err := s.client.doRequest(ctx, "GET", "/accounts/me", nil, jwt)
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
	data, err := s.client.doRequest(ctx, "GET", "/projects", nil, jwt)
	if err != nil {
		return nil, fmt.Errorf("get projects: %w", err)
	}

	var projects []IAMProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("parse projects: %w", err)
	}
	return projects, nil
}

func (s *Service) ForgeJWT(ctx context.Context, userID string) (*IAMForgeJWTResponse, error) {
	path := "/forge-jwt?" + url.Values{"user_id": {userID}}.Encode()
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

func (s *Service) CreateApiKey(ctx context.Context, name, userID, iamJWT string) (*IAMApiKey, error) {
	path := "/keys/" + url.PathEscape(name) + "?" + url.Values{"user_id": {userID}}.Encode()
	data, err := s.client.doRequest(ctx, "POST", path, nil, iamJWT)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	key := &IAMApiKey{}
	if err := json.Unmarshal(data, key); err != nil {
		return nil, fmt.Errorf("parse api key: %w", err)
	}
	return key, nil
}
