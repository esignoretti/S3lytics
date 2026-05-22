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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/esignoretti/s3lytics/internal/auth"
	"github.com/esignoretti/s3lytics/internal/scan"
	"github.com/esignoretti/s3lytics/internal/scan/deep"
	"github.com/esignoretti/s3lytics/internal/store"
	"github.com/esignoretti/s3lytics/internal/web"
	"github.com/esignoretti/s3lytics/internal/web/handlers"
)

var version = "0.1.0-dev"

func main() {
	port := flag.String("port", "8080", "HTTP server port")
	dataDir := flag.String("data", defaultDataDir(), "BadgerDB data directory")
	scanWorkers := flag.Int("scan-workers", 4, "Parallel prefix scanners (1-32)")
	scanBatchSize := flag.Int("scan-batch-size", 500, "Objects per DB write batch (100-5000)")
	scanPrefixTimeout := flag.Int("scan-prefix-timeout", 30, "Prefix discovery timeout in seconds")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	fmt.Printf("S3lytics v%s starting on :%s\n", version, *port)
	fmt.Printf("Data directory: %s\n", *dataDir)

	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}

	// Initialize BadgerDB store with fallback
	badgerStore, err := store.NewBadgerStore(*dataDir)
	if err != nil {
		log.Printf("WARN: failed to open BadgerDB at %s: %v", *dataDir, err)
		log.Printf("WARN: falling back to in-memory-only mode")
		tmpDir, tmpErr := os.MkdirTemp("", "s3lytics-mem-*")
		if tmpErr != nil {
			log.Fatalf("failed to create temp dir for fallback: %v", tmpErr)
		}
		badgerStore, err = store.NewBadgerStore(tmpDir)
		if err != nil {
			log.Fatalf("failed to open fallback store: %v", err)
		}
		log.Printf("WARN: using temporary store at %s", tmpDir)
	}
	defer badgerStore.Close()

	// Initialize auth service and session manager
	authService := auth.NewService(badgerStore)
	sessionManager := auth.NewSessionManager(badgerStore, authService)

	// Scan engine starts with a nil client; the handler swaps in the right
	// per-project S3 client before each scan.
	scanEngine := scan.NewEngine(nil, badgerStore)

	// Initialize template renderer
	renderer, err := web.NewTemplateRenderer()
	if err != nil {
		log.Fatalf("failed to init templates: %v", err)
	}

	// Initialize HTTP handler
	h := &handlers.Handler{
		Store:          badgerStore,
		AuthService:    authService,
		SessionManager: sessionManager,
		ScanEngine:     scanEngine,
		Renderer:       renderer,
		DeepConfig: deep.Config{
			EnableDuplicates:      true,
			EnableMultipart:       true,
			EnableAccessAudit:     true,
			EnableEncryption:      true,
			EnableVersioning:      true,
			EnableLargeFiles:      true,
			EnableNaming:          true,
			EnableCostEstimate:    true,
			EnableVirusScan:       false,
			LargeFileThresholdMB:  100,
			VirusConfig:           deep.VirusScanConfig{ClamdSocket: "/var/run/clamav/clamd.sock"},
			CostOverrides:         nil,
		},
	}

	// Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(authMiddleware(sessionManager))

	h.ScanEngine.SetConfig(scan.Config{
		Workers:       clamp(*scanWorkers, 1, 32),
		BatchSize:     clamp(*scanBatchSize, 100, 5000),
		PrefixTimeout: time.Duration(clamp(*scanPrefixTimeout, 10, 120)) * time.Second,
	})

	h.RegisterRoutes(r)

	// Rebuild per-project S3 clients from cached credentials so the app is
	// usable after restart without forcing a re-login.
	h.RestoreS3Clients(context.Background())

	// Server
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func authMiddleware(sm *auth.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
