package control

import (
	"errors"
	"testing"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestCompleteAssignmentStoresValidatedExecutionSummary(t *testing.T) {
	store, assignment := runningSingleAssignment(t)
	summary := validSummary(assignment)

	job, err := store.CompleteAssignment(assignment.WorkerID, assignment.ID, summary)
	if err != nil {
		t.Fatalf("CompleteAssignment() error = %v", err)
	}
	if job.State != protocol.TestStateCompleted {
		t.Fatalf("job state = %s, want completed", job.State)
	}
	completed := assignmentFor(t, job, assignment.WorkerID)
	if completed.Result == nil {
		t.Fatal("completed assignment has no execution result")
	}
	if completed.Result.ActualRPS != summary.ActualRPS || completed.Result.LatencyP99US != summary.LatencyP99US {
		t.Fatalf("stored result = %+v, want %+v", completed.Result, summary)
	}
}

func TestCompleteAssignmentRejectsResultThatDoesNotMatchAssignment(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*protocol.ExecutionSummary)
	}{
		{name: "engine", mutate: func(result *protocol.ExecutionSummary) { result.Engine = "other" }},
		{name: "target", mutate: func(result *protocol.ExecutionSummary) { result.Target = "http://10.250.0.99" }},
		{name: "rate", mutate: func(result *protocol.ExecutionSummary) { result.RequestedRPS++ }},
		{name: "counters", mutate: func(result *protocol.ExecutionSummary) { result.Completed = result.Started + 1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, assignment := runningSingleAssignment(t)
			result := validSummary(assignment)
			test.mutate(&result)
			if _, err := store.CompleteAssignment(assignment.WorkerID, assignment.ID, result); !errors.Is(err, ErrInvalidResult) {
				t.Fatalf("CompleteAssignment() error = %v, want ErrInvalidResult", err)
			}
		})
	}
}

func runningSingleAssignment(t *testing.T) (*JobStore, protocol.WorkerAssignment) {
	t.Helper()
	clock := time.Date(2026, time.September, 13, 12, 0, 0, 0, time.UTC)
	store := NewJobStore()
	store.now = func() time.Time { return clock }
	store.scheduleLead = time.Millisecond
	job, err := store.CreateJob(protocol.TestPlan{
		ID:                "result-job-" + t.Name(),
		Target:            "http://10.250.0.10:8080/healthz",
		Engine:            "blast",
		RequestsPerSecond: 20_000,
		DurationSeconds:   10,
		Shards: []protocol.WorkerShard{
			{WorkerID: "worker-a", RequestsPerSecond: 20_000},
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	assignment := assignmentFor(t, job, "worker-a")
	job, err = store.TransitionAssignment("worker-a", assignment.ID, protocol.AssignmentStateReady)
	if err != nil {
		t.Fatalf("ready assignment: %v", err)
	}
	clock = *job.StartAt
	job, err = store.TransitionAssignment("worker-a", assignment.ID, protocol.AssignmentStateRunning)
	if err != nil {
		t.Fatalf("start assignment: %v", err)
	}
	return store, assignmentFor(t, job, "worker-a")
}

func validSummary(assignment protocol.WorkerAssignment) protocol.ExecutionSummary {
	return protocol.ExecutionSummary{
		Engine:          assignment.Engine,
		Version:         "test",
		Target:          assignment.Target,
		RequestedRPS:    assignment.RequestsPerSecond,
		DurationMS:      assignment.DurationSeconds * 1_000,
		Concurrency:     256,
		Scheduled:       200_000,
		Started:         199_000,
		Completed:       198_900,
		Failed:          100,
		Backpressured:   1_000,
		BytesReceived:   19_890_000,
		ActualRPS:       19_890,
		LatencySamples:  199_000,
		LatencyMinUS:    100,
		LatencyP50US:    300,
		LatencyP95US:    800,
		LatencyP99US:    1_500,
		LatencyMaxUS:    8_000,
		Status2xx:       198_900,
		Status5xx:       100,
	}
}
