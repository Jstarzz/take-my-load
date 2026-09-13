package control

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

var (
	ErrJobNotFound        = errors.New("test job not found")
	ErrJobAlreadyExists   = errors.New("test job already exists")
	ErrAssignmentNotFound = errors.New("assignment not found")
	ErrAssignmentOwner    = errors.New("assignment does not belong to worker")
	ErrInvalidTransition  = errors.New("invalid assignment state transition")
	ErrStartTimeNotReached = errors.New("scheduled start time has not been reached")
)

type assignmentRef struct {
	jobID string
	index int
}

type JobStore struct {
	mu           sync.RWMutex
	jobs         map[string]*protocol.TestJob
	assignments  map[string]assignmentRef
	now          func() time.Time
	scheduleLead time.Duration
}

func NewJobStore() *JobStore {
	return &JobStore{
		jobs:         make(map[string]*protocol.TestJob),
		assignments:  make(map[string]assignmentRef),
		now:          time.Now,
		scheduleLead: 3 * time.Second,
	}
}

func (s *JobStore) Create(plan protocol.TestPlan) (protocol.TestJob, error) {
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

	now := s.now().UTC()
	job := &protocol.TestJob{
		ID:          plan.ID,
		Plan:        clonePlan(plan),
		State:       protocol.TestStatePreparing,
		Assignments: assignments,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return protocol.TestJob{}, ErrJobAlreadyExists
	}
	s.jobs[job.ID] = job
	for i, assignment := range job.Assignments {
		s.assignments[assignment.ID] = assignmentRef{jobID: job.ID, index: i}
	}
	return cloneJob(*job), nil
}

func (s *JobStore) Get(id string) (protocol.TestJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return protocol.TestJob{}, ErrJobNotFound
	}
	return cloneJob(*job), nil
}

func (s *JobStore) Assignments(workerID string) []protocol.WorkerAssignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]protocol.WorkerAssignment, 0)
	for _, job := range s.jobs {
		for _, assignment := range job.Assignments {
			if assignment.WorkerID == workerID && !assignmentTerminal(assignment.State) {
				result = append(result, cloneAssignment(assignment))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *JobStore) Cancel(id string) (protocol.TestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return protocol.TestJob{}, ErrJobNotFound
	}
	if jobTerminal(job.State) {
		return protocol.TestJob{}, fmt.Errorf("%w: job is already %s", ErrInvalidTransition, job.State)
	}
	for i := range job.Assignments {
		if !assignmentTerminal(job.Assignments[i].State) {
			job.Assignments[i].State = protocol.AssignmentStateCancelled
		}
	}
	job.State = protocol.TestStateCancelled
	job.UpdatedAt = s.now().UTC()
	return cloneJob(*job), nil
}

func (s *JobStore) Transition(workerID, assignmentID string, next protocol.AssignmentState) (protocol.TestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ref, ok := s.assignments[assignmentID]
	if !ok {
		return protocol.TestJob{}, ErrAssignmentNotFound
	}
	job := s.jobs[ref.jobID]
	assignment := &job.Assignments[ref.index]
	if assignment.WorkerID != workerID {
		return protocol.TestJob{}, ErrAssignmentOwner
	}

	now := s.now().UTC()
	switch next {
	case protocol.AssignmentStateReady:
		if assignment.State != protocol.AssignmentStatePending {
			return protocol.TestJob{}, transitionError(assignment.State, next)
		}
		assignment.State = protocol.AssignmentStateReady
		if allAssignments(job, protocol.AssignmentStateReady) {
			startAt := now.Add(s.scheduleLead)
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
			return protocol.TestJob{}, transitionError(assignment.State, next)
		}
		if assignment.StartAt == nil || now.Before(*assignment.StartAt) {
			return protocol.TestJob{}, ErrStartTimeNotReached
		}
		assignment.State = protocol.AssignmentStateRunning
		job.State = protocol.TestStateRunning
	case protocol.AssignmentStateCompleted:
		if assignment.State != protocol.AssignmentStateRunning {
			return protocol.TestJob{}, transitionError(assignment.State, next)
		}
		assignment.State = protocol.AssignmentStateCompleted
		if allAssignments(job, protocol.AssignmentStateCompleted) {
			job.State = protocol.TestStateCompleted
		}
	case protocol.AssignmentStateFailed:
		if assignmentTerminal(assignment.State) {
			return protocol.TestJob{}, transitionError(assignment.State, next)
		}
		assignment.State = protocol.AssignmentStateFailed
		job.State = protocol.TestStateFailed
		for i := range job.Assignments {
			if job.Assignments[i].ID != assignment.ID && !assignmentTerminal(job.Assignments[i].State) {
				job.Assignments[i].State = protocol.AssignmentStateCancelled
			}
		}
	default:
		return protocol.TestJob{}, transitionError(assignment.State, next)
	}
	job.UpdatedAt = now
	return cloneJob(*job), nil
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
