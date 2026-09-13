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

	summary := func(worker string) protocol.ExecutionSummary {
		return protocol.ExecutionSummary{
			Engine:         "blast",
			Version:        "integration",
			Target:         plan.Target,
			RequestedRPS:   50_000,
			DurationMS:     30_000,
			Concurrency:    4_096,
			Scheduled:      1_500_000,
			Started:        1_499_000,
			Completed:      1_498_900,
			Failed:         100,
			Backpressured:  1_000,
			BytesReceived:  149_890_000,
			ActualRPS:      49_963.33,
			LatencySamples: 1_499_000,
			LatencyMinUS:   120,
			LatencyP50US:   350,
			LatencyP95US:   900,
			LatencyP99US:   1_500,
			LatencyMaxUS:   12_000,
			Status2xx:      1_498_900,
			Status5xx:      100,
		}
	}
	if _, err := store.CompleteAssignment("worker-a", assignmentsA[0].ID, summary("worker-a")); err != nil {
		t.Fatalf("worker-a completed with result: %v", err)
	}
	job, err = store.CompleteAssignment("worker-b", assignmentsB[0].ID, summary("worker-b"))
	if err != nil {
		t.Fatalf("worker-b completed with result: %v", err)
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
	for _, assignment := range persisted.Assignments {
		if assignment.Result == nil {
			t.Fatalf("assignment %s lost execution result", assignment.ID)
		}
		if assignment.Result.Engine != "blast" || assignment.Result.RequestedRPS != 50_000 {
			t.Fatalf("assignment %s unexpected persisted result: %+v", assignment.ID, assignment.Result)
		}
	}
}
