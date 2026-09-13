package control

import (
	"fmt"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func BuildJob(plan protocol.TestPlan, now time.Time) (protocol.TestJob, error) {
	assignments := make([]protocol.WorkerAssignment, len(plan.Shards))
	for i, shard := range plan.Shards {
		id, err := newID()
		if err != nil {
			return protocol.TestJob{}, fmt.Errorf("create assignment id: %w", err)
		}
		assignments[i] = protocol.WorkerAssignment{
			ID:                id,
			JobID:             plan.ID,
			WorkerID:          shard.WorkerID,
			Target:            plan.Target,
			Engine:            plan.Engine,
			RequestsPerSecond: shard.RequestsPerSecond,
			DurationSeconds:   plan.DurationSeconds,
			State:             protocol.AssignmentStatePending,
		}
	}

	now = now.UTC()
	return protocol.TestJob{
		ID:          plan.ID,
		Plan:        clonePlan(plan),
		State:       protocol.TestStatePreparing,
		Assignments: assignments,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func ApplyAssignmentTransition(job *protocol.TestJob, workerID, assignmentID string, next protocol.AssignmentState, now time.Time, scheduleLead time.Duration) error {
	assignment := findAssignment(job, assignmentID)
	if assignment == nil {
		return ErrAssignmentNotFound
	}
	if assignment.WorkerID != workerID {
		return ErrAssignmentOwner
	}

	now = now.UTC()
	switch next {
	case protocol.AssignmentStateReady:
		if assignment.State != protocol.AssignmentStatePending {
			return transitionError(assignment.State, next)
		}
		assignment.State = protocol.AssignmentStateReady
		if allAssignments(job, protocol.AssignmentStateReady) {
			startAt := now.Add(scheduleLead)
			job.StartAt = &startAt
			job.State = protocol.TestStateScheduled
			for i := range job.Assignments {
				job.Assignments[i].State = protocol.AssignmentStateScheduled
				startCopy := startAt
				job.Assignments[i].StartAt = &startCopy
			}
		}
	case protocol.AssignmentStateRunning:
		if assignment.State != protocol.AssignmentStateScheduled {
			return transitionError(assignment.State, next)
		}
		if assignment.StartAt == nil || now.Before(*assignment.StartAt) {
			return ErrStartTimeNotReached
		}
		assignment.State = protocol.AssignmentStateRunning
		job.State = protocol.TestStateRunning
	case protocol.AssignmentStateCompleted:
		if assignment.State != protocol.AssignmentStateRunning {
			return transitionError(assignment.State, next)
		}
		assignment.State = protocol.AssignmentStateCompleted
		if allAssignments(job, protocol.AssignmentStateCompleted) {
			job.State = protocol.TestStateCompleted
		}
	case protocol.AssignmentStateFailed:
		if assignmentTerminal(assignment.State) {
			return transitionError(assignment.State, next)
		}
		assignment.State = protocol.AssignmentStateFailed
		job.State = protocol.TestStateFailed
		for i := range job.Assignments {
			if job.Assignments[i].ID != assignment.ID && !assignmentTerminal(job.Assignments[i].State) {
				job.Assignments[i].State = protocol.AssignmentStateCancelled
			}
		}
	default:
		return transitionError(assignment.State, next)
	}
	job.UpdatedAt = now
	return nil
}

func ApplyAssignmentCompletion(job *protocol.TestJob, workerID, assignmentID string, result protocol.ExecutionSummary, now time.Time) error {
	assignment := findAssignment(job, assignmentID)
	if assignment == nil {
		return ErrAssignmentNotFound
	}
	if assignment.WorkerID != workerID {
		return ErrAssignmentOwner
	}
	if assignment.State != protocol.AssignmentStateRunning {
		return transitionError(assignment.State, protocol.AssignmentStateCompleted)
	}
	if err := validateExecutionSummary(*assignment, result); err != nil {
		return err
	}
	resultCopy := result
	assignment.Result = &resultCopy
	return ApplyAssignmentTransition(job, workerID, assignmentID, protocol.AssignmentStateCompleted, now, 0)
}

func validateExecutionSummary(assignment protocol.WorkerAssignment, result protocol.ExecutionSummary) error {
	if result.Engine != assignment.Engine {
		return fmt.Errorf("%w: engine %q does not match %q", ErrInvalidResult, result.Engine, assignment.Engine)
	}
	if result.Target != assignment.Target {
		return fmt.Errorf("%w: target does not match assignment", ErrInvalidResult)
	}
	if result.RequestedRPS != assignment.RequestsPerSecond {
		return fmt.Errorf("%w: requested_rps %d does not match %d", ErrInvalidResult, result.RequestedRPS, assignment.RequestsPerSecond)
	}
	values := []int64{
		result.DurationMS,
		result.Concurrency,
		result.Scheduled,
		result.Started,
		result.Completed,
		result.Failed,
		result.Backpressured,
		result.BytesReceived,
		result.LatencySamples,
		result.LatencyMinUS,
		result.LatencyP50US,
		result.LatencyP95US,
		result.LatencyP99US,
		result.LatencyMaxUS,
		result.Status1xx,
		result.Status2xx,
		result.Status3xx,
		result.Status4xx,
		result.Status5xx,
		result.StatusOther,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("%w: counters cannot be negative", ErrInvalidResult)
		}
	}
	if result.DurationMS == 0 || result.Concurrency == 0 || result.ActualRPS < 0 {
		return fmt.Errorf("%w: duration, concurrency and actual_rps must be positive/non-negative", ErrInvalidResult)
	}
	if result.Started > result.Scheduled || result.Completed+result.Failed > result.Started {
		return fmt.Errorf("%w: inconsistent request counters", ErrInvalidResult)
	}
	return nil
}

func ApplyJobCancellation(job *protocol.TestJob, now time.Time) error {
	if jobTerminal(job.State) {
		return fmt.Errorf("%w: job is already %s", ErrInvalidTransition, job.State)
	}
	for i := range job.Assignments {
		if !assignmentTerminal(job.Assignments[i].State) {
			job.Assignments[i].State = protocol.AssignmentStateCancelled
		}
	}
	job.State = protocol.TestStateCancelled
	job.UpdatedAt = now.UTC()
	return nil
}

func findAssignment(job *protocol.TestJob, assignmentID string) *protocol.WorkerAssignment {
	for i := range job.Assignments {
		if job.Assignments[i].ID == assignmentID {
			return &job.Assignments[i]
		}
	}
	return nil
}

func transitionError(from, to protocol.AssignmentState) error {
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
}

func allAssignments(job *protocol.TestJob, state protocol.AssignmentState) bool {
	if len(job.Assignments) == 0 {
		return false
	}
	for _, assignment := range job.Assignments {
		if assignment.State != state {
			return false
		}
	}
	return true
}

func assignmentTerminal(state protocol.AssignmentState) bool {
	return state == protocol.AssignmentStateCompleted || state == protocol.AssignmentStateFailed || state == protocol.AssignmentStateCancelled
}

func jobTerminal(state protocol.TestState) bool {
	return state == protocol.TestStateCompleted || state == protocol.TestStateFailed || state == protocol.TestStateCancelled
}

func clonePlan(plan protocol.TestPlan) protocol.TestPlan {
	plan.Shards = append([]protocol.WorkerShard(nil), plan.Shards...)
	return plan
}

func cloneAssignment(assignment protocol.WorkerAssignment) protocol.WorkerAssignment {
	if assignment.StartAt != nil {
		startAt := *assignment.StartAt
		assignment.StartAt = &startAt
	}
	if assignment.Result != nil {
		result := *assignment.Result
		assignment.Result = &result
	}
	return assignment
}

func cloneJob(job protocol.TestJob) protocol.TestJob {
	job.Plan = clonePlan(job.Plan)
	job.Assignments = append([]protocol.WorkerAssignment(nil), job.Assignments...)
	for i := range job.Assignments {
		job.Assignments[i] = cloneAssignment(job.Assignments[i])
	}
	if job.StartAt != nil {
		startAt := *job.StartAt
		job.StartAt = &startAt
	}
	return job
}
