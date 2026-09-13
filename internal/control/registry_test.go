package control

import (
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestRegistryRegisterAndHeartbeat(t *testing.T) {
	registry := NewRegistry()
	clock := time.Date(2026, 9, 12, 20, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return clock }

	worker, err := registry.RegisterWorker(protocol.WorkerRegistration{
		ID: "worker-1", Name: "Worker 1", CapacityRPS: 100_000, Engines: []string{"blast"},
	})
	if err != nil {
		t.Fatalf("RegisterWorker() error = %v", err)
	}
	if worker.LastSeen != clock {
		t.Fatalf("LastSeen = %v, want %v", worker.LastSeen, clock)
	}

	clock = clock.Add(time.Second)
	worker, err = registry.HeartbeatWorker("worker-1", protocol.WorkerHeartbeat{CapacityRPS: 120_000})
	if err != nil {
		t.Fatalf("HeartbeatWorker() error = %v", err)
	}
	if worker.CapacityRPS != 120_000 {
		t.Fatalf("CapacityRPS = %d, want 120000", worker.CapacityRPS)
	}
	if worker.LastSeen != clock {
		t.Fatalf("LastSeen = %v, want %v", worker.LastSeen, clock)
	}
}

func TestRegistryHeartbeatUnknownWorker(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.HeartbeatWorker("missing", protocol.WorkerHeartbeat{}); err != ErrWorkerNotFound {
		t.Fatalf("HeartbeatWorker() error = %v, want %v", err, ErrWorkerNotFound)
	}
}
