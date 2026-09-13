package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type fakeControlClient struct {
	assignments []protocol.WorkerAssignment
	actions     []string
}

func (f *fakeControlClient) Assignments(context.Context, string) ([]protocol.WorkerAssignment, error) {
	return append([]protocol.WorkerAssignment(nil), f.assignments...), nil
}

func (f *fakeControlClient) Transition(_ context.Context, _, _ string, action string) (protocol.TestJob, error) {
	f.actions = append(f.actions, action)
	return protocol.TestJob{}, nil
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
	if len(client.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", client.actions, want)
	}
	for i := range want {
		if client.actions[i] != want[i] {
			t.Fatalf("actions = %v, want %v", client.actions, want)
		}
	}
}
