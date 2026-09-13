package control

import "github.com/Jstarzz/take-my-load/internal/protocol"

type WorkerRepository interface {
	Register(protocol.WorkerRegistration) (protocol.WorkerSnapshot, error)
	Heartbeat(string, protocol.WorkerHeartbeat) (protocol.WorkerSnapshot, error)
	Get(string) (protocol.WorkerSnapshot, bool, error)
	List() ([]protocol.WorkerSnapshot, error)
}

type JobRepository interface {
	Create(protocol.TestPlan) (protocol.TestJob, error)
	Get(string) (protocol.TestJob, error)
	Assignments(string) ([]protocol.WorkerAssignment, error)
	Cancel(string) (protocol.TestJob, error)
	Transition(string, string, protocol.AssignmentState) (protocol.TestJob, error)
}
