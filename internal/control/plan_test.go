package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestPlanTestBuildsAuthorizedCapacityAwarePlan(t *testing.T) {
	registry := NewRegistry()
	mustRegister(t, registry, protocol.WorkerRegistration{ID: "a", Name: "A", CapacityRPS: 80_000, Engines: []string{"blast"}})
	mustRegister(t, registry, protocol.WorkerRegistration{ID: "b", Name: "B", CapacityRPS: 70_000, Engines: []string{"blast"}})
	policy, err := ParseTargetPolicy("10.250.0.0/24")
	if err != nil {
		t.Fatalf("ParseTargetPolicy() error = %v", err)
	}
	server := NewServerWithPolicy(registry, "test", policy)

	body := []byte(`{"name":"smoke","target":"http://10.250.0.10:8080","engine":"blast","requests_per_second":100000,"duration_seconds":30}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tests/plan", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", res.Code, http.StatusCreated, res.Body.String())
	}

	var plan protocol.TestPlan
	if err := json.NewDecoder(res.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.RequestsPerSecond != 100_000 || plan.AvailableRPS != 150_000 {
		t.Fatalf("plan rates = requested %d available %d", plan.RequestsPerSecond, plan.AvailableRPS)
	}
	var total int64
	for _, shard := range plan.Shards {
		total += shard.RequestsPerSecond
	}
	if total != 100_000 {
		t.Fatalf("shards total = %d, want 100000", total)
	}
}

func TestPlanTestRejectsUnauthorizedTarget(t *testing.T) {
	registry := NewRegistry()
	mustRegister(t, registry, protocol.WorkerRegistration{ID: "a", Name: "A", CapacityRPS: 100_000, Engines: []string{"blast"}})
	policy, _ := ParseTargetPolicy("10.250.0.0/24")
	server := NewServerWithPolicy(registry, "test", policy)

	body := []byte(`{"target":"https://example.com","engine":"blast","requests_per_second":1000,"duration_seconds":30}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tests/plan", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d: %s", res.Code, http.StatusForbidden, res.Body.String())
	}
}

func TestPlanTestRejectsMoreThanAggregateCapacity(t *testing.T) {
	registry := NewRegistry()
	mustRegister(t, registry, protocol.WorkerRegistration{ID: "a", Name: "A", CapacityRPS: 10_000, Engines: []string{"blast"}})
	policy, _ := ParseTargetPolicy("10.250.0.0/24")
	server := NewServerWithPolicy(registry, "test", policy)

	body := []byte(`{"target":"http://10.250.0.10","engine":"blast","requests_per_second":10001,"duration_seconds":30}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tests/plan", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", res.Code, http.StatusConflict, res.Body.String())
	}
}

func TestPlannerExcludesWorkersPastHeartbeatLease(t *testing.T) {
	clock := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	registry := NewRegistry()
	registry.now = func() time.Time { return clock }
	mustRegister(t, registry, protocol.WorkerRegistration{ID: "live", Name: "Live", CapacityRPS: 80_000, Engines: []string{"blast"}})
	mustRegister(t, registry, protocol.WorkerRegistration{ID: "stale", Name: "Stale", CapacityRPS: 100_000, Engines: []string{"blast"}})

	policy, _ := ParseTargetPolicy("10.250.0.0/24")
	planner := NewPlanner(registry, policy)
	planner.now = func() time.Time { return clock }
	planner.workerTTL = 20 * time.Second

	capacity, err := planner.Capacity("blast")
	if err != nil {
		t.Fatalf("Capacity() error = %v", err)
	}
	if capacity.AvailableRPS != 180_000 {
		t.Fatalf("initial capacity = %d, want 180000", capacity.AvailableRPS)
	}

	clock = clock.Add(15 * time.Second)
	if _, err := registry.HeartbeatWorker("live", protocol.WorkerHeartbeat{}); err != nil {
		t.Fatalf("HeartbeatWorker() error = %v", err)
	}
	clock = clock.Add(10 * time.Second)

	capacity, err = planner.Capacity("blast")
	if err != nil {
		t.Fatalf("Capacity() after expiry error = %v", err)
	}
	if capacity.Workers != 1 || capacity.AvailableRPS != 80_000 {
		t.Fatalf("capacity after expiry = %+v, want one 80000-rps worker", capacity)
	}
}

func mustRegister(t *testing.T, registry *Registry, registration protocol.WorkerRegistration) {
	t.Helper()
	if _, err := registry.RegisterWorker(registration); err != nil {
		t.Fatalf("RegisterWorker() error = %v", err)
	}
}
