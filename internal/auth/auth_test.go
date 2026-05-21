package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/esignoretti/s3lytics/internal/store"
	"golang.org/x/crypto/ed25519"
)

func newTestStore(t *testing.T) *store.BadgerStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "s3lytics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := store.NewBadgerStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestKeyDerivation(t *testing.T) {
	password := "test-password"
	saltB64 := "c2FsdHlzYWx0"

	saltBytes, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		t.Fatal(err)
	}

	seed := sha256.Sum256(append([]byte(password), saltBytes...))
	privateKey := ed25519.NewKeyFromSeed(seed[:])

	if len(privateKey) != ed25519.PrivateKeySize {
		t.Errorf("expected private key size %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	msg := []byte("test-challenge")
	sig := ed25519.Sign(privateKey, msg)
	if !ed25519.Verify(privateKey.Public().(ed25519.PublicKey), msg, sig) {
		t.Error("signature verification failed")
	}
}

func TestSessionManagerLifecycle(t *testing.T) {
	s := newTestStore(t)
	authService := NewService(s)
	sm := NewSessionManager(s, authService)
	ctx := context.Background()

	if sm.IsLoggedIn(ctx) {
		t.Error("expected not logged in initially")
	}

	signinResp := &IAMSigninResponse{
		JWT:          "test-jwt",
		RefreshToken: "test-refresh",
	}
	account := &IAMAccount{
		ID:              "user-1",
		Email:           "test@example.com",
		EndpointGateway: "https://s3.cubbit.eu",
	}

	if err := sm.SaveLogin(ctx, signinResp, account); err != nil {
		t.Fatal(err)
	}

	if !sm.IsLoggedIn(ctx) {
		t.Error("expected logged in after SaveLogin")
	}

	jwt, err := sm.GetValidJWT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if jwt != "test-jwt" {
		t.Errorf("expected test-jwt, got %s", jwt)
	}

	if err := sm.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if sm.IsLoggedIn(ctx) {
		t.Error("expected not logged in after logout")
	}
}

func TestJWTRefreshOnExpiry(t *testing.T) {
	s := newTestStore(t)
	authService := NewService(s)
	sm := NewSessionManager(s, authService)
	ctx := context.Background()

	authService.client.SetBaseURL("http://localhost:9999")

	expiredSession := &store.SessionData{
		JWT:          "expired-jwt",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	if err := s.SaveSession(ctx, expiredSession); err != nil {
		t.Fatal(err)
	}

	account := &store.AccountData{
		UserID:          "user-1",
		Email:           "test@example.com",
		EndpointGateway: "https://s3.cubbit.eu",
	}
	if err := s.SaveAccount(ctx, account); err != nil {
		t.Fatal(err)
	}

	_, err := sm.GetValidJWT(ctx)
	if err == nil {
		t.Error("expected error when refreshing expired JWT without server")
	}

	if sm.IsLoggedIn(ctx) {
		t.Error("expected session cleared after failed refresh")
	}
}
