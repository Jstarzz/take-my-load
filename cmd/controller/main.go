package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Jstarzz/take-my-load/internal/control"
)

var version = "dev"

func main() {
	addr := getenv("TML_LISTEN_ADDR", ":8080")
	policy, err := control.ParseTargetPolicy(os.Getenv("TML_ALLOWED_TARGETS"))
	if err != nil {
		log.Fatalf("invalid TML_ALLOWED_TARGETS: %v", err)
	}
	registry := control.NewRegistry()
	server := control.NewServerWithPolicy(registry, version, policy)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("take-my-load controller version=%s listening=%s", version, addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
