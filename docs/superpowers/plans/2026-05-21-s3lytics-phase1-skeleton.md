# S3lytics — Phase 1: Project Skeleton

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Initialize the Go module, create the full directory structure, add a Makefile, `.gitignore`, and a compilable `main.go` that prints a banner.

**Architecture:** Single Go binary under `cmd/s3lytics/main.go`. All application logic lives under `internal/`. Templates and static assets are embedded via `embed.FS`. BadgerDB data directory default `~/.s3lytics/data/`.

**Tech Stack:** Go 1.22+, chi router, BadgerDB, aws-sdk-go-v2, HTMX + Chart.js (CDN-loaded in templates)

**Pre-requisites:** Go 1.22+ installed. Check with `go version`.

---

### Task 1: Initialize module, directory tree, and .gitignore

**Files:**
- Create: `S3lytics/go.mod`
- Create: `S3lytics/.gitignore`

- [ ] **Step 1: Initialize the Go module**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go mod init github.com/esignoretti/s3lytics
```
Expected output: `go: creating new go.mod: module github.com/esignoretti/s3lytics`

- [ ] **Step 2: Create the directory tree**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && \
mkdir -p cmd/s3lytics && \
mkdir -p internal/auth && \
mkdir -p internal/scan/deep && \
mkdir -p internal/store && \
mkdir -p internal/web/handlers && \
mkdir -p internal/web/templates && \
mkdir -p internal/web/static && \
mkdir -p internal/s3
```

Expected: no errors, directories created.

- [ ] **Step 3: Write .gitignore**

```bash
cat > .gitignore << 'EOF'
# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
*.out

# Go
*.test
*.test.exe
*.prof
go.work

# Dependencies
vendor/

# OS
.DS_Store
Thumbs.db

# IDE
.idea/
.vscode/
*.swp
*.swo

# S3lytics local data
s3lytics-data/
~/.s3lytics/

# Build artifacts
dist/
build/
EOF
```

- [ ] **Step 4: Commit**

```bash
git init && git add -A && git commit -m "chore: scaffold Go module and directory structure"
```

---

### Task 2: Makefile with common commands

**Files:**
- Create: `S3lytics/Makefile`

- [ ] **Step 1: Write the Makefile**

```makefile
# S3lytics Makefile

BINARY_NAME=s3lytics
BUILD_DIR=build
MAIN_PATH=./cmd/s3lytics
GO_FLAGS=-ldflags="-s -w"

.PHONY: all build run clean test lint fmt vet

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

run:
	go run $(MAIN_PATH)

clean:
	rm -rf $(BUILD_DIR)
	rm -rf s3lytics-data/

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./... 2>/dev/null || echo "golangci-lint not installed, skipping"

fmt:
	go fmt ./...

vet:
	go vet ./...
```

- [ ] **Step 2: Commit**

```bash
git add Makefile && git commit -m "chore: add Makefile with build, test, lint targets"
```

---

### Task 3: Compilable main.go skeleton

**Files:**
- Create: `S3lytics/cmd/s3lytics/main.go`

- [ ] **Step 1: Write main.go**

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var version = "0.1.0-dev"

func main() {
	port := flag.String("port", "8080", "HTTP server port")
	dataDir := flag.String("data", defaultDataDir(), "BadgerDB data directory")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	fmt.Printf("S3lytics v%s starting on :%s\n", version, *port)
	fmt.Printf("Data directory: %s\n", *dataDir)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h1>S3lytics</h1><p>Scanning in progress...</p>")
	})

	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: r,
	}

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
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./s3lytics-data"
	}
	return home + "/.s3lytics/data"
}
```

- [ ] **Step 2: Add chi dependency**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go get github.com/go-chi/chi/v5
```

Expected: `go: added github.com/go-chi/chi/v5 vX.Y.Z`

- [ ] **Step 3: Verify it compiles**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go build ./cmd/s3lytics/
```

Expected: no errors, a binary named `s3lytics` (or `s3lytics.exe` on Windows) appears.

- [ ] **Step 4: Verify Makefile build target works**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && make build
```

Expected: binary created in `build/s3lytics`.

- [ ] **Step 5: Commit**

```bash
git add cmd/s3lytics/main.go go.mod go.sum && git commit -m "feat: add compilable main.go skeleton with chi router"
```

---

### Task 4: Go vet and fmt pass

- [ ] **Step 1: Run vet**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go vet ./...
```

Expected: no errors.

- [ ] **Step 2: Run fmt**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go fmt ./...
```

Expected: no output (or file names if reformatted).

- [ ] **Step 3: Run tests (none yet, should pass trivially)**

```bash
cd /Users/esignoretti/Documents/OpenCode/S3lytics && go test ./... -v -count=1
```

Expected: `?    github.com/esignoretti/s3lytics [no test files]` for each package.

- [ ] **Step 4: Commit (if anything changed)**

```bash
git add -A && git commit -m "chore: go vet and go fmt pass"
```

---

**End of Phase 1. Phase 1 deliverables:**
- [x] `go.mod` initialized
- [x] Directory tree created (`cmd/s3lytics/`, `internal/*`)
- [x] `.gitignore` written
- [x] `Makefile` with build/run/test/clean targets
- [x] `main.go` compiles and serves HTTP on `:8080`
- [x] `go vet ./...` passes
- [x] First git commit

**Ready for Phase 2: BadgerDB store layer and data models.**
