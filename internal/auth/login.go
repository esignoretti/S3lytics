package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/ed25519"
)

func (s *Service) Login(req *LoginRequest) (*IAMSigninResponse, error) {
	apiURL := req.APIURL
	if apiURL == "" {
		apiURL = "https://iam.cubbit.eu"
	}
	s.client.SetBaseURL(apiURL)

	challengeBody := map[string]string{
		"email": req.Email,
	}
	if req.TenantID != "" {
		challengeBody["tenant_id"] = req.TenantID
	}

	challengeResp := &IAMChallengeResponse{}
	data, err := s.client.doRequest("POST", "/challenge", challengeBody, "")
	if err != nil {
		return nil, fmt.Errorf("challenge request: %w", err)
	}
	if err := json.Unmarshal(data, challengeResp); err != nil {
		return nil, fmt.Errorf("parse challenge response: %w", err)
	}

	saltBytes, err := base64.StdEncoding.DecodeString(challengeResp.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	seed := sha256.Sum256(append([]byte(req.Password), saltBytes...))
	privateKey := ed25519.NewKeyFromSeed(seed[:])

	signature := ed25519.Sign(privateKey, []byte(challengeResp.Challenge))
	signedChallenge := base64.StdEncoding.EncodeToString(signature)

	signinBody := map[string]string{
		"email":            req.Email,
		"signed_challenge": signedChallenge,
	}
	if req.TfaCode != "" {
		signinBody["tfa_code"] = req.TfaCode
	}
	if req.TenantID != "" {
		signinBody["tenant_id"] = req.TenantID
	}

	signinResp := &IAMSigninResponse{}
	data, err = s.client.doRequest("POST", "/signin", signinBody, "")
	if err != nil {
		return nil, fmt.Errorf("signin request: %w", err)
	}
	if err := json.Unmarshal(data, signinResp); err != nil {
		return nil, fmt.Errorf("parse signin response: %w", err)
	}

	return signinResp, nil
}
