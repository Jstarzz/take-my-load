package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
	workerclient "github.com/Jstarzz/take-my-load/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	hostname, _ := os.Hostname()
	id := getenv("TML_WORKER_ID", hostname)
	name := getenv("TML_WORKER_NAME", id)
	controller := getenv("TML_CONTROLLER_URL", "http://127.0.0.1:8080")
	capacity := getenvInt64("TML_WORKER_CAPACITY_RPS", 0)
	heartbeatInterval := getenvDuration("TML_HEARTBEAT_INTERVAL", 5*time.Second)
	assignmentInterval := getenvDuration("TML_ASSIGNMENT_POLL_INTERVAL", 500*time.Millisecond)
	simulation := getenvBool("TML_COORDINATION_SIMULATION", false)
	engines := splitCSV(getenv("TML_ENGINES", "blast,k6,h2load,wrk2"))

	registration := protocol.WorkerRegistration{
		ID:          id,
		Name:        name,
		CapacityRPS: capacity,
		Engines:     engines,
	}
	client := workerclient.NewClient(controller)

	for {
		if err := client.Register(ctx, registration); err == nil {
			break
		} else {
			log.Printf("register failed: %v; retrying", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
	log.Printf("worker registered id=%s controller=%s engines=%v simulation=%t", id, controller, engines, simulation)

	coordinator := workerclient.NewCoordinator(client, id)
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	assignmentTicker := time.NewTicker(assignmentInterval)
	defer heartbeatTicker.Stop()
	defer assignmentTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("worker stopping id=%s", id)
			return
		case <-heartbeatTicker.C:
			if err := client.Heartbeat(ctx, id, protocol.WorkerHeartbeat{CapacityRPS: capacity}); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
		case <-assignmentTicker.C:
			if !simulation {
				continue
			}
			if err := coordinator.TickSimulation(ctx); err != nil {
				log.Printf("coordination tick failed: %v", err)
			}
		}
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", key, value, err)
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", key, value, err)
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", key, value, err)
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
