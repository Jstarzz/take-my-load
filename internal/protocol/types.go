package protocol

import "time"

type WorkerRegistration struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Address     string   `json:"address,omitempty"`
	CapacityRPS int64    `json:"capacity_rps"`
	Engines     []string `json:"engines"`
}

type WorkerHeartbeat struct {
	CapacityRPS int64 `json:"capacity_rps,omitempty"`
}

type WorkerSnapshot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Address     string    `json:"address,omitempty"`
	CapacityRPS int64     `json:"capacity_rps"`
	Engines     []string  `json:"engines"`
	LastSeen    time.Time `json:"last_seen"`
}

type TestPlanRequest struct {
	Name              string `json:"name,omitempty"`
	Target            string `json:"target"`
	Engine            string `json:"engine"`
	RequestsPerSecond int64  `json:"requests_per_second"`
	DurationSeconds   int64  `json:"duration_seconds"`
}

type WorkerShard struct {
	WorkerID          string `json:"worker_id"`
	RequestsPerSecond int64  `json:"requests_per_second"`
}

type TestPlan struct {
	ID                string        `json:"id"`
	Name              string        `json:"name,omitempty"`
	Target            string        `json:"target"`
	Engine            string        `json:"engine"`
	RequestsPerSecond int64         `json:"requests_per_second"`
	DurationSeconds   int64         `json:"duration_seconds"`
	AvailableRPS      int64         `json:"available_rps"`
	Shards            []WorkerShard `json:"shards"`
	CreatedAt         time.Time     `json:"created_at"`
}

type TestState string

const (
	TestStatePreparing TestState = "preparing"
	TestStateScheduled TestState = "scheduled"
	TestStateRunning   TestState = "running"
	TestStateCompleted TestState = "completed"
	TestStateFailed    TestState = "failed"
	TestStateCancelled TestState = "cancelled"
)

type AssignmentState string

const (
	AssignmentStatePending   AssignmentState = "pending"
	AssignmentStateReady     AssignmentState = "ready"
	AssignmentStateScheduled AssignmentState = "scheduled"
	AssignmentStateRunning   AssignmentState = "running"
	AssignmentStateCompleted AssignmentState = "completed"
	AssignmentStateFailed    AssignmentState = "failed"
	AssignmentStateCancelled AssignmentState = "cancelled"
)

type WorkerAssignment struct {
	ID                string          `json:"id"`
	JobID             string          `json:"job_id"`
	WorkerID          string          `json:"worker_id"`
	Target            string          `json:"target"`
	Engine            string          `json:"engine"`
	RequestsPerSecond int64           `json:"requests_per_second"`
	DurationSeconds   int64           `json:"duration_seconds"`
	State             AssignmentState `json:"state"`
	StartAt           *time.Time      `json:"start_at,omitempty"`
}

type TestJob struct {
	ID          string             `json:"id"`
	Plan        TestPlan           `json:"plan"`
	State       TestState          `json:"state"`
	Assignments []WorkerAssignment `json:"assignments"`
	StartAt     *time.Time         `json:"start_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type CapacityResponse struct {
	Engine       string `json:"engine"`
	Workers      int    `json:"workers"`
	AvailableRPS int64  `json:"available_rps"`
}

type InfoResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
