package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Jstarzz/take-my-load/internal/control"
	pgstore "github.com/Jstarzz/take-my-load/internal/persistence/postgres"
)

var version = "dev"

func main() {
	addr := getenv("TML_LISTEN_ADDR", ":8080")
	policy, err := control.ParseTargetPolicy(os.Getenv("TML_ALLOWED_TARGETS"))
	if err != nil {
		log.Fatalf("invalid TML_ALLOWED_TARGETS: %v", err)
	}

	workers, jobs, cleanup := repositories()
	defer cleanup()
	server := control.NewServerWithRepositories(workers, jobs, version, policy)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("controller shutdown failed: %v", err)
		}
	}()

	log.Printf("take-my-load controller version=%s listening=%s", version, addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func repositories() (control.WorkerRepository, control.JobRepository, func()) {
	dsn := strings.TrimSpace(os.Getenv("TML_DATABASE_URL"))
	if dsn == "" {
		log.Printf("storage backend=in-memory")
		return control.NewRegistry(), control.NewJobStore(), func() {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := pgstore.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("open postgres store: %v", err)
	}
	log.Printf("storage backend=postgres")
	return store, store, store.Close
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
