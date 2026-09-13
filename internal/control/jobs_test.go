package control

import (
	"errors"
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestJobStoreSchedulesAllWorkersOnSharedStart(t *testing.T) {
	clock := time.Date(2026, 9, 12, 21, 0, 0, 0, time.UTC)
	store := NewJobStore()
	store.now = func() time.Time { return clock }
	store.scheduleLead = 2 * time.Second

	job, err := store.Create(protocol.TestPlan{
		ID: "job-1", Target: "http://10.250.0.10", Engine: "blast", DurationSeconds: 30,
		Shards: []protocol.WorkerShard{
			{WorkerID: "a", RequestsPerSecond: 60_000},
			{WorkerID: "b", RequestsPerSecond: 40_000},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if job.State != protocol.TestStatePreparing {
		t.Fatalf("state = %s, want preparing", job.State)
	}

	a := assignmentFor(t, job, "a")
	b := assignmentFor(t, job, "b")
	job, err = store.Transition("a", a.ID, protocol.AssignmentStateReady)
	if err != nil {
		t.Fatalf("first ready: %v", err)
	}
	if job.State != protocol.TestStatePreparing {
		t.Fatalf("state after one ready = %s, want preparing", job.State)
	}

	job, err = store.Transition("b", b.ID, protocol.AssignmentStateReady)
	if err != nil {
		t.Fatalf("second ready: %v", err)
	}
	wantStart := clock.Add(2 * time.Second)
	if job.State != protocol.TestStateScheduled || job.StartAt == nil || !job.StartAt.Equal(wantStart) {
		t.Fatalf("scheduled job = %#v, want start %v", job, wantStart)
	}
	for _, assignment := range job.Assignments {
		if assignment.State != protocol.AssignmentStateScheduled || assignment.StartAt == nil || !assignment.StartAt.Equal(wantStart) {
			t.Fatalf("assignment not synchronized: %#v", assignment)
		}
	}

	if _, err := store.Transition("a", a.ID, protocol.AssignmentStateRunning); !errors.Is(err, ErrStartTimeNotReached) {
		t.Fatalf("early start error = %v, want ErrStartTimeNotReached", err)
	}
	clock = wantStart
	if _, err := store.Transition("a", a.ID, protocol.AssignmentStateRunning); err != nil {
		t.Fatalf("start a: %v", err)
	}
	if _, err := store.Transition("b", b.ID, protocol.AssignmentStateRunning); err != nil {
		t.Fatalf("start b: %v", err)
	}
	if _, err := store.Transition("a", a.ID, protocol.AssignmentStateCompleted); err != nil {
		t.Fatalf("complete a: %v", err)
	}
	job, err = store.Transition("b", b.ID, protocol.AssignmentStateCompleted)
	if err != nil {
		t.Fatalf("complete b: %v", err)
	}
	if job.State != protocol.TestStateCompleted {
		t.Fatalf("final state = %s, want completed", job.State)
	}
}

func TestJobStoreFailureCancelsPeers(t *testing.T) {
	store := NewJobStore()
	job, err := store.Create(protocol.TestPlan{
		ID: "job-2", Target: "http://10.250.0.10", Engine: "blast", DurationSeconds: 10,
		Shards: []protocol.WorkerShard{
			{WorkerID: "a", RequestsPerSecond: 10},
			{WorkerID: "b", RequestsPerSecond: 10},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	a := assignmentFor(t, job, "a")
	job, err = store.Transition("a", a.ID, protocol.AssignmentStateFailed)
	if err != nil {
		t.Fatalf("fail a: %v", err)
	}
	if job.State != protocol.TestStateFailed {
		t.Fatalf("state = %s, want failed", job.State)
	}
	for _, assignment := range job.Assignments {
		if assignment.WorkerID == "b" && assignment.State != protocol.AssignmentStateCancelled {
			t.Fatalf("peer state = %s, want cancelled", assignment.State)
		}
	}
}

func assignmentFor(t *testing.T, job protocol.TestJob, workerID string) protocol.WorkerAssignment {
	t.Helper()
	for _, assignment := range job.Assignments {
		if assignment.WorkerID == workerID {
			return assignment
		}
	}
	t.Fatalf("assignment for worker %s not found", workerID)
	return protocol.WorkerAssignment{}
}
