package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/esignoretti/s3lytics/internal/store"
)

const refreshThreshold = 5 * time.Minute

type SessionManager struct {
	mu    sync.Mutex
	store store.Store
	auth  *Service
}

func NewSessionManager(s store.Store, authService *Service) *SessionManager {
	return &SessionManager{store: s, auth: authService}
}

func (sm *SessionManager) IsLoggedIn(ctx context.Context) bool {
	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return false
	}
	return session.JWT != ""
}

func (sm *SessionManager) SaveLogin(ctx context.Context, signinResp *IAMSigninResponse, account *IAMAccount) error {
	session := &store.SessionData{
		JWT:          signinResp.JWT,
		RefreshToken: signinResp.RefreshToken,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	if err := sm.store.SaveSession(ctx, session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	acct := &store.AccountData{
		EndpointGateway: account.EndpointGateway,
		Email:           account.Email,
		UserID:          account.ID,
	}
	if err := sm.store.SaveAccount(ctx, acct); err != nil {
		return fmt.Errorf("save account: %w", err)
	}
	return nil
}

func (sm *SessionManager) GetValidJWT(ctx context.Context) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return "", fmt.Errorf("no session: %w", err)
	}

	if time.Now().Add(refreshThreshold).After(session.ExpiresAt) {
		return sm.refreshJWT(ctx, session)
	}

	return session.JWT, nil
}

func (sm *SessionManager) refreshJWT(ctx context.Context, session *store.SessionData) (string, error) {
	account, err := sm.store.GetAccount(ctx)
	if err != nil {
		return "", fmt.Errorf("no account for refresh: %w", err)
	}

	forgeResp, err := sm.auth.ForgeJWT(ctx, account.UserID)
	if err != nil {
		sm.store.ClearAuth(ctx)
		return "", fmt.Errorf("jwt refresh failed: %w", err)
	}

	session.JWT = forgeResp.JWT
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := sm.store.SaveSession(ctx, session); err != nil {
		return "", fmt.Errorf("save refreshed session: %w", err)
	}

	return forgeResp.JWT, nil
}

func (sm *SessionManager) Logout(ctx context.Context) error {
	return sm.store.ClearAuth(ctx)
}
