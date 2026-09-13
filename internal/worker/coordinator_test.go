package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type fakeControlClient struct {
	mu          sync.Mutex
	assignments []protocol.WorkerAssignment
	actions     []string
}

func (f *fakeControlClient) Assignments(context.Context, string) ([]protocol.WorkerAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]protocol.WorkerAssignment(nil), f.assignments...), nil
}

func (f *fakeControlClient) Transition(_ context.Context, _, _ string, action string) (protocol.TestJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, action)
	return protocol.TestJob{}, nil
}

func (f *fakeControlClient) setAssignments(assignments []protocol.WorkerAssignment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assignments = append([]protocol.WorkerAssignment(nil), assignments...)
}

func (f *fakeControlClient) actionSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.actions...)
}

type fakeExecutor struct {
	started   chan protocol.WorkerAssignment
	release   chan struct{}
	cancelled chan struct{}
}

func (e *fakeExecutor) Run(ctx context.Context, assignment protocol.WorkerAssignment) error {
	e.started <- assignment
	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		close(e.cancelled)
		return ctx.Err()
	}
}

func TestCoordinatorSimulationAdvancesSafeLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 12, 21, 0, 0, 0, time.UTC)
	past := now.Add(-time.Second)
	future := now.Add(time.Second)
	client := &fakeControlClient{assignments: []protocol.WorkerAssignment{
		{ID: "pending", State: protocol.AssignmentStatePending},
		{ID: "scheduled-past", State: protocol.AssignmentStateScheduled, StartAt: &past},
		{ID: "scheduled-future", State: protocol.AssignmentStateScheduled, StartAt: &future},
		{ID: "running", State: protocol.AssignmentStateRunning},
		{ID: "ready", State: protocol.AssignmentStateReady},
	}}
	coordinator := NewCoordinator(client, "worker-a")
	coordinator.now = func() time.Time { return now }

	if err := coordinator.TickSimulation(context.Background()); err != nil {
		t.Fatalf("TickSimulation() error = %v", err)
	}
	want := []string{"ready", "started", "completed"}
	got := client.actionSnapshot()
	if len(got) != len(want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("actions = %v, want %v", got, want)
		}
	}
}

func TestCoordinatorExecutionLaunchesShardOnceAndCompletes(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Millisecond)
	assignment := protocol.WorkerAssignment{
		ID:                "assignment-a",
		WorkerID:          "worker-a",
		Target:            "http://10.250.0.10:8080/healthz",
		Engine:            "blast",
		RequestsPerSecond: 10_000,
		DurationSeconds:   1,
		State:             protocol.AssignmentStateScheduled,
		StartAt:           &past,
	}
	client := &fakeControlClient{assignments: []protocol.WorkerAssignment{assignment}}
	executor := &fakeExecutor{
		started:   make(chan protocol.WorkerAssignment, 2),
		release:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
	coordinator := NewCoordinatorWithExecutor(client, "worker-a", executor)
	coordinator.now = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := coordinator.TickExecution(ctx); err != nil {
		t.Fatalf("TickExecution() error = %v", err)
	}
	select {
	case got := <-executor.started:
		if got.ID != assignment.ID {
			t.Fatalf("executed assignment = %s, want %s", got.ID, assignment.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}

	if err := coordinator.TickExecution(ctx); err != nil {
		t.Fatalf("second TickExecution() error = %v", err)
	}
	select {
	case <-executor.started:
		t.Fatal("assignment launched twice")
	default:
	}

	close(executor.release)
	waitForAction(t, client, "completed")
	got := client.actionSnapshot()
	if got[0] != "started" || got[len(got)-1] != "completed" {
		t.Fatalf("actions = %v", got)
	}
}

func TestCoordinatorExecutionCancelsShardWhenAssignmentDisappears(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Millisecond)
	assignment := protocol.WorkerAssignment{
		ID:                "assignment-cancel",
		WorkerID:          "worker-a",
		Target:            "http://10.250.0.10:8080/healthz",
		Engine:            "blast",
		RequestsPerSecond: 10_000,
		DurationSeconds:   30,
		State:             protocol.AssignmentStateScheduled,
		StartAt:           &past,
	}
	client := &fakeControlClient{assignments: []protocol.WorkerAssignment{assignment}}
	executor := &fakeExecutor{
		started:   make(chan protocol.WorkerAssignment, 1),
		release:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
	coordinator := NewCoordinatorWithExecutor(client, "worker-a", executor)
	coordinator.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coordinator.TickExecution(ctx); err != nil {
		t.Fatalf("TickExecution() error = %v", err)
	}
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	client.setAssignments(nil)
	if err := coordinator.TickExecution(ctx); err != nil {
		t.Fatalf("cancellation TickExecution() error = %v", err)
	}
	select {
	case <-executor.cancelled:
	case <-time.After(time.Second):
		t.Fatal("executor was not cancelled")
	}
}

func waitForAction(t *testing.T, client *fakeControlClient, action string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, got := range client.actionSnapshot() {
			if got == action {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("action %q not observed; got %v", action, client.actionSnapshot())
}
