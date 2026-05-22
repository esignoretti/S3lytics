package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func (s *Service) Login(ctx context.Context, req *LoginRequest) (*IAMSigninResponse, error) {
	if req.APIURL != "" {
		s.client.SetBaseURL(req.APIURL)
	} else if s.client.BaseURL() == "" {
		s.client.SetBaseURL(defaultCoordinatorURL)
	}

	challengeResp := &IAMChallengeResponse{}
	challengeData, err := s.client.doRequest(ctx, "POST", "/iam/v1/auth/signin/challenge",
		challengeRequest{Email: req.Email, TenantID: req.TenantID}, "")
	if err != nil {
		return nil, fmt.Errorf("challenge request: %w", err)
	}
	if err := json.Unmarshal(challengeData, challengeResp); err != nil {
		return nil, fmt.Errorf("parse challenge response: %w", err)
	}

	seed := sha256.Sum256(append([]byte(req.Password), []byte(challengeResp.Salt)...))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	signature := ed25519.Sign(privateKey, []byte(challengeResp.Challenge))
	signedChallenge := base64.StdEncoding.EncodeToString(signature)

	signinResp := &IAMSigninResponse{}
	signinData, err := s.client.doRequest(ctx, "POST", "/iam/v1/auth/signin",
		signinRequest{
			Email:           req.Email,
			SignedChallenge: signedChallenge,
			TfaCode:         req.TfaCode,
			TenantID:        req.TenantID,
		}, "")
	if err != nil {
		return nil, fmt.Errorf("signin request: %w", err)
	}
	if err := json.Unmarshal(signinData, signinResp); err != nil {
		return nil, fmt.Errorf("parse signin response: %w", err)
	}
	signinResp.RefreshToken = s.client.GetRefreshCookie()
	if signinResp.RefreshToken == "" {
		return nil, fmt.Errorf("signin succeeded but no _refresh cookie was received")
	}

	return signinResp, nil
}
