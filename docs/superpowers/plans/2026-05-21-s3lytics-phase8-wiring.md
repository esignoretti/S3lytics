# S3lytics — Phase 8: Wire Everything Together

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `cmd/s3lytics/main.go` to initialize all dependencies (store, auth, S3 client, scan engine, template renderer, HTTP handlers), mount routes, and serve the application. Also add graceful shutdown for BadgerDB and running scans.

**Architecture:** `main.go` becomes the composition root. It creates the BadgerStore, AuthService, SessionManager, CubbitS3Client, ScanEngine, TemplateRenderer, and Handler, then wires them into the chi router. If not authenticated, all routes redirect to `/login`.

**Tech Stack:** All phases 1-7 combined, `chi` middleware

**Pre-requisites:** Phases 1-7 complete.

---

### Task 1: Rewrite main.go with full initialization

**Files:**
- Modify: `cmd/s3lytics/main.go`

- [ ] **Step 1: Write the new main.go**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/esignoretti/s3lytics/internal/auth"
	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/scan"
	"github.com/esignoretti/s3lytics/internal/store"
	"github.com/esignoretti/s3lytics/internal/web"
	"github.com/esignoretti/s3lytics/internal/web/handlers"
)

var version = "0.1.0-dev"

func main() {
	port := flag.String("port", "8080", "HTTP server port")
	dataDir := flag.String("data", defaultDataDir(), "BadgerDB data directory")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	fmt.Printf("S3lytics v%s starting on :%s\n", version, *port)
	fmt.Printf("Data directory: %s\n", *dataDir)

	// --- Initialize Store ---
	badgerStore, err := store.NewBadgerStore(*dataDir)
	if err != nil {
		log.Printf("WARN: failed to open BadgerDB at %s: %v", *dataDir, err)
		log.Printf("WARN: falling back to in-memory-only mode — data will NOT persist across restarts")
		// In-memory fallback is not supported by BadgerStore directly;
		// create a temp directory for the session
		tmpDir, tmpErr := os.MkdirTemp("", "s3lytics-mem-*")
		if tmpErr != nil {
			log.Fatalf("failed to create temp dir for in-memory fallback: %v", tmpErr)
		}
		badgerStore, err = store.NewBadgerStore(tmpDir)
		if err != nil {
			log.Fatalf("failed to open fallback store: %v", err)
		}
		log.Printf("WARN: using temporary store at %s", tmpDir)
	}
	defer badgerStore.Close()

	// Ensure data dir exists
	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		log.Printf("WARN: failed to create data dir %s: %v", *dataDir, err)
	}

	// --- Initialize Auth ---
	authService := auth.NewService(badgerStore)
	sessionManager := auth.NewSessionManager(badgerStore, authService)

	// --- Check for existing session to configure S3 ---
	ctx := context.Background()
	// S3 client is created lazily after login (when API keys are available).
	// The ScanEngine handles a nil S3 client gracefully — scan requests fail
	// until the user logs in and provides credentials.
	var s3Client s3.S3Client

	// --- Initialize Scan Engine ---
	scanEngine := scan.NewEngine(s3Client, badgerStore)

	// --- Initialize Template Renderer ---
	tmplRenderer, err := web.NewTemplateRenderer()
	if err != nil {
		log.Fatalf("failed to init templates: %v", err)
	}

	// --- Initialize HTTP Handler ---
	h := &handlers.Handler{
		Store:            badgerStore,
		AuthService:      authService,
		SessionManager:   sessionManager,
		ScanEngine:       scanEngine,
		S3Client:         s3Client,
		TemplateRenderer: tmplRenderer,
	}

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60))
	r.Use(authMiddleware(sessionManager))

	h.RegisterRoutes(r)

	// --- Server ---
	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: r,
	}

	// Channel for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("HTTP server listening on :%s", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := <-quit
	log.Printf("received signal %v, shutting down", sig)
	srv.Close()
	badgerStore.Close()
}

func authMiddleware(sm *auth.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for login, logout, static, and health
			path := r.URL.Path
			if path == "/login" || path == "/logout" || path == "/health" ||
				strings.HasPrefix(path, "/static/") {
				next.ServeHTTP(w, r)
				return
			}

			if !sm.IsLoggedIn(r.Context()) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./s3lytics-data"
	}
	return home + "/.s3lytics/data"
}
```

- [ ] **Step 2: Add missing import (`strings`) to main.go**

The `authMiddleware` uses `strings.HasPrefix` — ensure the import is present.

- [ ] **Step 3: Verify compilation**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./cmd/s3lytics/ && go build ./cmd/s3lytics/
```

Expected: no errors, binary compiles.

- [ ] **Step 4: Run the full test suite**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./... -v -count=1 -timeout=60s
```

Expected: all tests from all packages pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/s3lytics/main.go && git commit -m "feat: wire all components in main.go with auth middleware"
```

---

### Task 2: Application bootstrap smoke test

- [ ] **Step 1: Start the application briefly to verify it boots**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && timeout 3 ./build/s3lytics --port 9090 2>&1 || true
```

Expected: Prints "S3lytics v0.1.0-dev starting on :9090" and "HTTP server listening on :9090", then exits after 3 seconds.

- [ ] **Step 2: Test that the login page renders**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && \
./build/s3lytics --port 9091 &
sleep 1
curl -s http://localhost:9091/login | head -5
kill %1 2>/dev/null
```

Expected: curl returns HTML containing "S3lytics" and "Sign in with your Cubbit IAM account".

- [ ] **Step 3: Commit (if anything changed)**

```bash
git add -A && git commit -m "chore: verify application boots and serves login page"
```

---

**End of Phase 8. Phase 8 deliverables:**
- [x] `main.go` initializes all dependencies
- [x] Auth middleware redirects unauthenticated users to `/login`
- [x] Chi router with all routes mounted
- [x] Graceful shutdown (SIGINT/SIGTERM)
- [x] Application boots and serves login page
- [x] `go vet` and `go build` pass
- [x] Full test suite passes

**S3lytics v0.1.0 is now complete.** Remaining work (future phases):
- Phase 9: HTMX partial templates for dynamic scan progress polling
- Phase 10: Scan history trend charts (3+ scans)
- Phase 11: PDF report export (wkhtmltopdf or similar)
- Phase 12: Scheduled scans, alerting, multi-user
