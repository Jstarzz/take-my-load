package control

import "github.com/Jstarzz/take-my-load/internal/protocol"

type WorkerRepository interface {
	RegisterWorker(protocol.WorkerRegistration) (protocol.WorkerSnapshot, error)
	HeartbeatWorker(string, protocol.WorkerHeartbeat) (protocol.WorkerSnapshot, error)
	GetWorker(string) (protocol.WorkerSnapshot, bool, error)
	ListWorkers() ([]protocol.WorkerSnapshot, error)
}

type JobRepository interface {
	CreateJob(protocol.TestPlan) (protocol.TestJob, error)
	GetJob(string) (protocol.TestJob, error)
	ListAssignments(string) ([]protocol.WorkerAssignment, error)
	CancelJob(string) (protocol.TestJob, error)
	TransitionAssignment(string, string, protocol.AssignmentState) (protocol.TestJob, error)
	CompleteAssignment(string, string, protocol.ExecutionSummary) (protocol.TestJob, error)
}
