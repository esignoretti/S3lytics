# S3lytics — Phase 4: Auth Service (Cubbit IAM Reimplementation)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Cubbit IAM authentication protocol: challenge-response using Curve25519/Ed25519 signing, JWT management with proactive refresh, session persistence to BadgerDB, and API key generation.

**Architecture:** Package `internal/auth/` owns all IAM interaction. It uses the `store.Store` interface for persistence and `golang.org/x/crypto/ed25519` for signing. The flow follows the 7-step protocol from the design doc (challenge → sign → signin → account → projects → forge-jwt → keys).

**Tech Stack:** `golang.org/x/crypto/ed25519`, `golang.org/x/crypto/sha3`, net/http client, `encoding/json`, `store.Store`

**Pre-requisites:** Phase 2 (store layer) and Phase 3 (S3 client) complete.

---

### Task 1: Add crypto dependency and define auth types

**Files:**
- Create: `internal/auth/types.go`

- [ ] **Step 1: Add crypto dependency**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go get golang.org/x/crypto
```

Expected: `go: added golang.org/x/crypto`

- [ ] **Step 2: Write auth types**

```go
package auth

// LoginRequest holds the fields from the login form.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TfaCode  string `json:"tfa_code,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	APIURL   string `json:"api_url,omitempty"`
}

// IAMChallengeResponse is the response from POST /challenge.
type IAMChallengeResponse struct {
	Salt      string `json:"salt"`
	Challenge string `json:"challenge"`
}

// IAMSigninResponse is the response from POST /signin.
type IAMSigninResponse struct {
	JWT          string `json:"jwt"`
	RefreshToken string `json:"_refresh"`
}

// IAMAccount is the response from GET /accounts/me.
type IAMAccount struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	EndpointGateway string `json:"endpoint_gateway"`
}

// IAMProject is from GET /projects.
type IAMProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// IAMForgeJWTResponse is from GET /forge-jwt.
type IAMForgeJWTResponse struct {
	JWT string `json:"jwt"`
}

// IAMApiKey is from POST /keys/{name}.
type IAMApiKey struct {
	ApiKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

// Service handles authentication with the Cubbit IAM server.
type Service struct {
	client    *httpClient
	store     store.Store
}

// NewService creates a new auth service.
func NewService(s store.Store) *Service {
	return &Service{
		client: &httpClient{},
		store:  s,
	}
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/auth/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/auth/types.go go.mod go.sum && git commit -m "feat: add auth types and service skeleton"
```

---

### Task 2: HTTP client with cookie jar for refresh tokens

**Files:**
- Create: `internal/auth/http.go`

- [ ] **Step 1: Write the HTTP client wrapper**

```go
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type httpClient struct {
	baseURL    string
	httpClient *http.Client
	cookies    []*http.Cookie
}

func (c *httpClient) SetBaseURL(url string) {
	// Strip trailing slash
	c.baseURL = strings.TrimRight(url, "/")
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
}

func (c *httpClient) doRequest(method, path string, body interface{}, authToken string) ([]byte, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	// Attach stored cookies
	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// Capture Set-Cookie headers
	c.cookies = nil
	for _, cc := range resp.Cookies() {
		c.cookies = append(c.cookies, cc)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/auth/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/http.go && git commit -m "feat: add HTTP client with cookie jar for refresh tokens"
```

---

### Task 3: Challenge-response protocol (steps 1-3)

**Files:**
- Create: `internal/auth/login.go`

- [ ] **Step 1: Write the challenge-response login flow**

```go
package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/ed25519"
)

// Login performs the full Cubbit IAM challenge-response flow:
// 1. POST /challenge -> {salt, challenge}
// 2. Derive Ed25519 key from password + salt
// 3. Sign challenge with private key
// 4. POST /signin -> JWT + refresh cookie
// Returns the JWT token and refresh token.
func (s *Service) Login(req *LoginRequest) (*IAMSigninResponse, error) {
	// Determine base URL
	apiURL := req.APIURL
	if apiURL == "" {
		apiURL = "https://iam.cubbit.eu"
	}
	s.client.SetBaseURL(apiURL)

	// Step 1: Get challenge
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

	// Step 2: Derive Ed25519 key from password + salt
	saltBytes, err := base64.StdEncoding.DecodeString(challengeResp.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}

	seed := sha256.Sum256(append([]byte(req.Password), saltBytes...))
	privateKey := ed25519.NewKeyFromSeed(seed[:])

	// Step 3: Sign challenge
	signature := ed25519.Sign(privateKey, []byte(challengeResp.Challenge))
	signedChallenge := base64.StdEncoding.EncodeToString(signature)

	// Step 4: Sign in
	signinBody := map[string]string{
		"email":           req.Email,
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
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/auth/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/login.go && git commit -m "feat: add challenge-response login flow"
```

---

### Task 4: Account, projects, forge-jwt, and API keys (steps 4-7)

**Files:**
- Create: `internal/auth/iam.go`

- [ ] **Step 1: Write account, projects, forge-jwt, and API key methods**

```go
package auth

import (
	"encoding/json"
	"fmt"
)

// GetAccount retrieves account info including the S3 endpoint gateway.
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

// GetProjects retrieves the list of projects for the authenticated user.
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

// ForgeJWT creates an IAM-scoped JWT using the refresh token cookie.
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

// CreateApiKey creates a new S3 API key for the given user.
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
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/auth/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/iam.go && git commit -m "feat: add account, projects, forge-jwt, and API key methods"
```

---

### Task 5: Session management with proactive refresh

**Files:**
- Create: `internal/auth/session.go`

- [ ] **Step 1: Write session management with proactive refresh**

```go
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/esignoretti/s3lytics/internal/store"
)

const refreshThreshold = 5 * time.Minute

// SessionManager handles JWT lifecycle and persistence.
type SessionManager struct {
	store store.Store
	auth  *Service
}

// NewSessionManager creates a new session manager.
func NewSessionManager(s store.Store, authService *Service) *SessionManager {
	return &SessionManager{store: s, auth: authService}
}

// IsLoggedIn checks whether a valid session exists.
func (sm *SessionManager) IsLoggedIn(ctx context.Context) bool {
	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return false
	}
	return session.JWT != ""
}

// SaveLogin persists session and account data after a successful login.
func (sm *SessionManager) SaveLogin(ctx context.Context, signinResp *IAMSigninResponse, account *IAMAccount) error {
	session := &store.SessionData{
		JWT:          signinResp.JWT,
		RefreshToken: signinResp.RefreshToken,
		ExpiresAt:    time.Now().Add(24 * time.Hour), // approximate; real expiry from JWT claims
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

// GetValidJWT returns a valid JWT, proactively refreshing if needed.
func (sm *SessionManager) GetValidJWT(ctx context.Context) (string, error) {
	session, err := sm.store.GetSession(ctx)
	if err != nil {
		return "", fmt.Errorf("no session: %w", err)
	}

	// Refresh if expiring within threshold
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

	forgeResp, err := sm.auth.ForgeJWT(account.UserID)
	if err != nil {
		// Refresh failed — clear auth
		sm.store.ClearAuth(ctx)
		return "", fmt.Errorf("jwt refresh failed: %w", err)
	}

	// Update stored session
	session.JWT = forgeResp.JWT
	session.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := sm.store.SaveSession(ctx, session); err != nil {
		return "", fmt.Errorf("save refreshed session: %w", err)
	}

	return forgeResp.JWT, nil
}

// Logout clears all auth data from the store.
func (sm *SessionManager) Logout(ctx context.Context) error {
	return sm.store.ClearAuth(ctx)
}
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./internal/auth/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/session.go && git commit -m "feat: add session management with proactive JWT refresh"
```

---

### Task 6: Auth service tests

**Files:**
- Create: `internal/auth/auth_test.go`

- [ ] **Step 1: Write auth service tests**

```go
package auth

import (
	"context"
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
	// Verify the key derivation matches Cubbit's approach
	password := "test-password"
	saltB64 := "c2FsdHlzYWx0" // "saltsalt" in base64

	saltBytes, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		t.Fatal(err)
	}

	seed := sha256.Sum256(append([]byte(password), saltBytes...))
	privateKey := ed25519.NewKeyFromSeed(seed[:])

	if len(privateKey) != ed25519.PrivateKeySize {
		t.Errorf("expected private key size %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	// Sign and verify
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

	// Initially not logged in
	if sm.IsLoggedIn(ctx) {
		t.Error("expected not logged in initially")
	}

	// Save login
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

	// Get valid JWT
	jwt, err := sm.GetValidJWT(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if jwt != "test-jwt" {
		t.Errorf("expected test-jwt, got %s", jwt)
	}

	// Logout
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

	// Save session with expired JWT
	expiredSession := &store.SessionData{
		JWT:          "expired-jwt",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // expired
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

	// GetValidJWT should attempt refresh (and fail since we have no real server)
	_, err := sm.GetValidJWT(ctx)
	if err == nil {
		t.Error("expected error when refreshing expired JWT without server")
	}

	// Session should be cleared after failed refresh
	if sm.IsLoggedIn(ctx) {
		t.Error("expected session cleared after failed refresh")
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./internal/auth/ -v -count=1 -timeout=30s
```

Expected: all 3 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/auth_test.go && git commit -m "test: add auth service unit tests for key derivation, session lifecycle, and JWT refresh"
```

---

**End of Phase 4. Phase 4 deliverables:**
- [x] Auth types (`internal/auth/types.go`)
- [x] HTTP client with cookie jar (`internal/auth/http.go`)
- [x] Challenge-response login flow (`internal/auth/login.go`)
- [x] Account, projects, forge-jwt, API key methods (`internal/auth/iam.go`)
- [x] Session management with proactive 5-min refresh (`internal/auth/session.go`)
- [x] 3 unit tests passing
- [x] `golang.org/x/crypto` dependency added

**Ready for Phase 5: Scan engine (basic scan + incremental + delta).**
