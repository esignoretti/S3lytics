package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	_ = dataDir // used in Phase 2 when BadgerDB is wired in

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./s3lytics-data"
	}
	return home + "/.s3lytics/data"
}
