package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/esignoretti/s3lytics/internal/store"
)

type SessionManager struct {
	mu    sync.Mutex
	store store.Store
}

func NewSessionManager(s store.Store) *SessionManager {
	return &SessionManager{store: s}
}

func (sm *SessionManager) IsLoggedIn(ctx context.Context) bool {
	_, err := sm.store.GetSession(ctx)
	return err == nil
}

func (sm *SessionManager) SaveLogin(ctx context.Context, endpoint, region, accessKey, secretKey string) error {
	data := &store.SessionData{
		JWT:          accessKey,
		RefreshToken: secretKey,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	if err := sm.store.SaveSession(ctx, data); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	acct := &store.AccountData{
		EndpointGateway: endpoint,
		Email:           region,
	}
	if err := sm.store.SaveAccount(ctx, acct); err != nil {
		return fmt.Errorf("save account: %w", err)
	}
	return nil
}

func (sm *SessionManager) GetCredentials(ctx context.Context) (endpoint, region, accessKey, secretKey string, err error) {
	session, sErr := sm.store.GetSession(ctx)
	account, aErr := sm.store.GetAccount(ctx)
	if sErr != nil || aErr != nil {
		return "", "", "", "", fmt.Errorf("no saved credentials")
	}
	return account.EndpointGateway, account.Email, session.JWT, session.RefreshToken, nil
}

func (sm *SessionManager) Logout(ctx context.Context) error {
	return sm.store.ClearAuth(ctx)
}
