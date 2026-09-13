package control

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

var (
	ErrJobNotFound         = errors.New("test job not found")
	ErrJobAlreadyExists    = errors.New("test job already exists")
	ErrAssignmentNotFound  = errors.New("assignment not found")
	ErrAssignmentOwner     = errors.New("assignment does not belong to worker")
	ErrInvalidTransition   = errors.New("invalid assignment state transition")
	ErrStartTimeNotReached = errors.New("scheduled start time has not been reached")
	ErrInvalidResult       = errors.New("invalid assignment execution result")
)

type JobStore struct {
	mu            sync.RWMutex
	jobs          map[string]*protocol.TestJob
	assignmentJob map[string]string
	now           func() time.Time
	scheduleLead  time.Duration
}

func NewJobStore() *JobStore {
	return &JobStore{
		jobs:          make(map[string]*protocol.TestJob),
		assignmentJob: make(map[string]string),
		now:           time.Now,
		scheduleLead:  3 * time.Second,
	}
}

func (s *JobStore) CreateJob(plan protocol.TestPlan) (protocol.TestJob, error) {
	job, err := BuildJob(plan, s.now())
	if err != nil {
		return protocol.TestJob{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return protocol.TestJob{}, ErrJobAlreadyExists
	}
	s.jobs[job.ID] = &job
	for _, assignment := range job.Assignments {
		s.assignmentJob[assignment.ID] = job.ID
	}
	return cloneJob(job), nil
}

func (s *JobStore) GetJob(id string) (protocol.TestJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return protocol.TestJob{}, ErrJobNotFound
	}
	return cloneJob(*job), nil
}

func (s *JobStore) ListAssignments(workerID string) ([]protocol.WorkerAssignment, error) {
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
	return result, nil
}

func (s *JobStore) CancelJob(id string) (protocol.TestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return protocol.TestJob{}, ErrJobNotFound
	}
	if err := ApplyJobCancellation(job, s.now()); err != nil {
		return protocol.TestJob{}, err
	}
	return cloneJob(*job), nil
}

func (s *JobStore) TransitionAssignment(workerID, assignmentID string, next protocol.AssignmentState) (protocol.TestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID, ok := s.assignmentJob[assignmentID]
	if !ok {
		return protocol.TestJob{}, ErrAssignmentNotFound
	}
	job := s.jobs[jobID]
	if err := ApplyAssignmentTransition(job, workerID, assignmentID, next, s.now(), s.scheduleLead); err != nil {
		return protocol.TestJob{}, err
	}
	return cloneJob(*job), nil
}

func (s *JobStore) CompleteAssignment(workerID, assignmentID string, result protocol.ExecutionSummary) (protocol.TestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID, ok := s.assignmentJob[assignmentID]
	if !ok {
		return protocol.TestJob{}, ErrAssignmentNotFound
	}
	job := s.jobs[jobID]
	if err := ApplyAssignmentCompletion(job, workerID, assignmentID, result, s.now()); err != nil {
		return protocol.TestJob{}, err
	}
	return cloneJob(*job), nil
}
