package auth

import (
	"encoding/json"
	"fmt"
)

func (s *Service) GetAccount(jwt string) (*IAMAccount, error) {
	data, err := s.client.doRequest("GET", "/accounts/me", nil, jwt)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	account := &IAMAccount{}
	if err := json.Unmarshal(data, account); err != nil {
		return nil, fmt.Errorf("parse account: %w", err)
	}
	return account, nil
}

func (s *Service) GetProjects(jwt string) ([]IAMProject, error) {
	data, err := s.client.doRequest("GET", "/projects", nil, jwt)
	if err != nil {
		return nil, fmt.Errorf("get projects: %w", err)
	}

	var projects []IAMProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("parse projects: %w", err)
	}
	return projects, nil
}

func (s *Service) ForgeJWT(userID string) (*IAMForgeJWTResponse, error) {
	path := fmt.Sprintf("/forge-jwt?user_id=%s", userID)
	data, err := s.client.doRequest("GET", path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("forge jwt: %w", err)
	}

	resp := &IAMForgeJWTResponse{}
	if err := json.Unmarshal(data, resp); err != nil {
		return nil, fmt.Errorf("parse forge jwt: %w", err)
	}
	return resp, nil
}

func (s *Service) CreateApiKey(name, userID, iamJWT string) (*IAMApiKey, error) {
	path := fmt.Sprintf("/keys/%s?user_id=%s", name, userID)
	data, err := s.client.doRequest("POST", path, nil, iamJWT)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	key := &IAMApiKey{}
	if err := json.Unmarshal(data, key); err != nil {
		return nil, fmt.Errorf("parse api key: %w", err)
	}
	return key, nil
}
