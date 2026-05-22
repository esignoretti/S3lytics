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
	sm := &SessionManager{store: s, auth: authService}
	// If a session exists from a previous run, restore the coordinator URL
	// and _refresh cookie into the in-memory http client so refresh/forge
	// continue to work without forcing a re-login.
	if session, err := s.GetSession(context.Background()); err == nil {
		authService.RestoreSession(session.CoordinatorURL, session.RefreshToken)
	}
	return sm
}

func (sm *SessionManager) IsLoggedIn(ctx context.Context) bool {
	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return false
	}
	return session.JWT != ""
}

func (sm *SessionManager) SaveLogin(ctx context.Context, signinResp *IAMSigninResponse, account *IAMAccount, coordinatorURL string) error {
	primaryEmail := ""
	if len(account.Emails) > 0 {
		primaryEmail = account.Emails[0].Email
		for _, e := range account.Emails {
			if e.IsDefault {
				primaryEmail = e.Email
				break
			}
		}
	}

	expiresAt := parseExpiry(signinResp.ExpDate, signinResp.Exp)
	session := &store.SessionData{
		JWT:            signinResp.Token,
		RefreshToken:   sm.auth.CurrentRefreshToken(),
		CoordinatorURL: coordinatorURL,
		ExpiresAt:      expiresAt,
	}
	if err := sm.store.SaveSession(ctx, session); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	acct := &store.AccountData{
		EndpointGateway: account.EndpointGateway,
		Email:           primaryEmail,
		UserID:          account.ID,
	}
	if err := sm.store.SaveAccount(ctx, acct); err != nil {
		return fmt.Errorf("save account: %w", err)
	}
	return nil
}

// SyncRefreshToken re-reads the latest _refresh cookie from the http client
// and persists it. Call after any operation that may rotate the cookie
// (RefreshToken, ForgeJWT).
func (sm *SessionManager) SyncRefreshToken(ctx context.Context) error {
	current := sm.auth.CurrentRefreshToken()
	if current == "" {
		return nil
	}
	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return nil
	}
	if session.RefreshToken == current {
		return nil
	}
	session.RefreshToken = current
	return sm.store.SaveSession(ctx, session)
}

func (sm *SessionManager) GetValidJWT(ctx context.Context) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return "", fmt.Errorf("no session: %w", err)
	}

	if time.Now().Add(refreshThreshold).After(session.ExpiresAt) {
		return sm.refreshLocked(ctx, session)
	}

	return session.JWT, nil
}

func (sm *SessionManager) refreshLocked(ctx context.Context, session *store.SessionData) (string, error) {
	newToken, err := sm.auth.RefreshToken(ctx)
	if err != nil {
		sm.store.ClearAuth(ctx)
		return "", fmt.Errorf("jwt refresh failed: %w", err)
	}

	session.JWT = newToken
	session.RefreshToken = sm.auth.CurrentRefreshToken()
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := sm.store.SaveSession(ctx, session); err != nil {
		return "", fmt.Errorf("save refreshed session: %w", err)
	}
	return newToken, nil
}

func (sm *SessionManager) Logout(ctx context.Context) error {
	return sm.store.ClearAuth(ctx)
}

// parseExpiry returns an absolute expiry time from the server-provided
// ExpDate (ISO 8601) or Exp (unix seconds), falling back to +24h.
func parseExpiry(expDate string, expUnix int64) time.Time {
	if expDate != "" {
		if t, err := time.Parse(time.RFC3339, expDate); err == nil {
			return t
		}
	}
	if expUnix > 0 {
		return time.Unix(expUnix, 0)
	}
	return time.Now().Add(24 * time.Hour)
}
