package control

import (
	"errors"
	"testing"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestSchedulerShardsRequestedRateAcrossEligibleWorkers(t *testing.T) {
	workers := []protocol.WorkerSnapshot{
		{ID: "worker-a", CapacityRPS: 100_000, Engines: []string{"blast"}},
		{ID: "worker-b", CapacityRPS: 50_000, Engines: []string{"blast"}},
		{ID: "worker-c", CapacityRPS: 999_999, Engines: []string{"k6"}},
	}

	shards, capacity, err := (Scheduler{}).Shard(workers, "blast", 120_000)
	if err != nil {
		t.Fatalf("Shard() error = %v", err)
	}
	if capacity != 150_000 {
		t.Fatalf("capacity = %d, want 150000", capacity)
	}
	var total int64
	for _, shard := range shards {
		total += shard.RequestsPerSecond
	}
	if total != 120_000 {
		t.Fatalf("sharded RPS = %d, want 120000", total)
	}
	if len(shards) != 2 {
		t.Fatalf("len(shards) = %d, want 2", len(shards))
	}
}

func TestSchedulerRejectsInsufficientCapacity(t *testing.T) {
	workers := []protocol.WorkerSnapshot{{ID: "worker-a", CapacityRPS: 10, Engines: []string{"blast"}}}
	_, capacity, err := (Scheduler{}).Shard(workers, "blast", 11)
	if !errors.Is(err, ErrInsufficientCapacity) {
		t.Fatalf("Shard() error = %v, want ErrInsufficientCapacity", err)
	}
	if capacity != 10 {
		t.Fatalf("capacity = %d, want 10", capacity)
	}
}

func TestSchedulerNeverExceedsWorkerCapacity(t *testing.T) {
	workers := []protocol.WorkerSnapshot{
		{ID: "a", CapacityRPS: 1, Engines: []string{"blast"}},
		{ID: "b", CapacityRPS: 1, Engines: []string{"blast"}},
		{ID: "c", CapacityRPS: 100, Engines: []string{"blast"}},
	}
	shards, _, err := (Scheduler{}).Shard(workers, "blast", 101)
	if err != nil {
		t.Fatalf("Shard() error = %v", err)
	}
	capacities := map[string]int64{"a": 1, "b": 1, "c": 100}
	for _, shard := range shards {
		if shard.RequestsPerSecond > capacities[shard.WorkerID] {
			t.Fatalf("worker %s assigned %d > capacity %d", shard.WorkerID, shard.RequestsPerSecond, capacities[shard.WorkerID])
		}
	}
}
