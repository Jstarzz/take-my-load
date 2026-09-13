//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestStorePersistsDistributedJobLifecycle(t *testing.T) {
	dsn := os.Getenv("TML_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TML_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		if store != nil {
			store.Close()
		}
	}()

	if _, err := store.pool.Exec(ctx, `TRUNCATE worker_assignments, test_jobs, test_plans, workers RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate test tables: %v", err)
	}

	for _, worker := range []protocol.WorkerRegistration{
		{ID: "worker-a", Name: "Worker A", CapacityRPS: 75_000, Engines: []string{"blast"}},
		{ID: "worker-b", Name: "Worker B", CapacityRPS: 75_000, Engines: []string{"blast"}},
	} {
		if _, err := store.RegisterWorker(worker); err != nil {
			t.Fatalf("RegisterWorker(%s) error = %v", worker.ID, err)
		}
	}

	base := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return base }
	store.scheduleLead = 250 * time.Millisecond

	plan := protocol.TestPlan{
		ID:                "integration-plan",
		Name:              "postgres lifecycle",
		Target:            "http://10.250.0.10:8080",
		Engine:            "blast",
		RequestsPerSecond: 100_000,
		DurationSeconds:   30,
		AvailableRPS:      150_000,
		Shards: []protocol.WorkerShard{
			{WorkerID: "worker-a", RequestsPerSecond: 50_000},
			{WorkerID: "worker-b", RequestsPerSecond: 50_000},
		},
		CreatedAt: base,
	}
	job, err := store.CreateJob(plan)
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if job.State != protocol.TestStatePreparing || len(job.Assignments) != 2 {
		t.Fatalf("created job state=%s assignments=%d", job.State, len(job.Assignments))
	}

	assignmentsA, err := store.ListAssignments("worker-a")
	if err != nil || len(assignmentsA) != 1 {
		t.Fatalf("ListAssignments(worker-a) len=%d err=%v", len(assignmentsA), err)
	}
	assignmentsB, err := store.ListAssignments("worker-b")
	if err != nil || len(assignmentsB) != 1 {
		t.Fatalf("ListAssignments(worker-b) len=%d err=%v", len(assignmentsB), err)
	}

	if _, err := store.TransitionAssignment("worker-a", assignmentsA[0].ID, protocol.AssignmentStateReady); err != nil {
		t.Fatalf("worker-a ready: %v", err)
	}
	job, err = store.TransitionAssignment("worker-b", assignmentsB[0].ID, protocol.AssignmentStateReady)
	if err != nil {
		t.Fatalf("worker-b ready: %v", err)
	}
	if job.State != protocol.TestStateScheduled || job.StartAt == nil {
		t.Fatalf("scheduled job state=%s start_at=%v", job.State, job.StartAt)
	}

	startAt := *job.StartAt
	store.now = func() time.Time { return startAt.Add(time.Millisecond) }
	if _, err := store.TransitionAssignment("worker-a", assignmentsA[0].ID, protocol.AssignmentStateRunning); err != nil {
		t.Fatalf("worker-a running: %v", err)
	}
	if _, err := store.TransitionAssignment("worker-b", assignmentsB[0].ID, protocol.AssignmentStateRunning); err != nil {
		t.Fatalf("worker-b running: %v", err)
	}
	if _, err := store.TransitionAssignment("worker-a", assignmentsA[0].ID, protocol.AssignmentStateCompleted); err != nil {
		t.Fatalf("worker-a completed: %v", err)
	}
	job, err = store.TransitionAssignment("worker-b", assignmentsB[0].ID, protocol.AssignmentStateCompleted)
	if err != nil {
		t.Fatalf("worker-b completed: %v", err)
	}
	if job.State != protocol.TestStateCompleted {
		t.Fatalf("final state=%s, want completed", job.State)
	}

	store.Close()
	store = nil
	reopened, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	persisted, err := reopened.GetJob(plan.ID)
	if err != nil {
		t.Fatalf("GetJob() after reopen: %v", err)
	}
	if persisted.State != protocol.TestStateCompleted || len(persisted.Assignments) != 2 {
		t.Fatalf("persisted state=%s assignments=%d", persisted.State, len(persisted.Assignments))
	}
}
