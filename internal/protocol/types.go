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

type ExecutionSummary struct {
	Engine          string  `json:"engine"`
	Version         string  `json:"version"`
	Target          string  `json:"target"`
	RequestedRPS    int64   `json:"requested_rps"`
	DurationMS      int64   `json:"duration_ms"`
	Concurrency     int64   `json:"concurrency"`
	Scheduled       int64   `json:"scheduled"`
	Started         int64   `json:"started"`
	Completed       int64   `json:"completed"`
	Failed          int64   `json:"failed"`
	Backpressured   int64   `json:"backpressured"`
	BytesReceived   int64   `json:"bytes_received"`
	ActualRPS       float64 `json:"actual_rps"`
	LatencySamples  int64   `json:"latency_samples"`
	LatencyMinUS    int64   `json:"latency_min_us"`
	LatencyP50US    int64   `json:"latency_p50_us"`
	LatencyP95US    int64   `json:"latency_p95_us"`
	LatencyP99US    int64   `json:"latency_p99_us"`
	LatencyMaxUS    int64   `json:"latency_max_us"`
	Status1xx       int64   `json:"status_1xx"`
	Status2xx       int64   `json:"status_2xx"`
	Status3xx       int64   `json:"status_3xx"`
	Status4xx       int64   `json:"status_4xx"`
	Status5xx       int64   `json:"status_5xx"`
	StatusOther     int64   `json:"status_other"`
}

type WorkerAssignment struct {
	ID                string            `json:"id"`
	JobID             string            `json:"job_id"`
	WorkerID          string            `json:"worker_id"`
	Target            string            `json:"target"`
	Engine            string            `json:"engine"`
	RequestsPerSecond int64             `json:"requests_per_second"`
	DurationSeconds   int64             `json:"duration_seconds"`
	State             AssignmentState   `json:"state"`
	StartAt           *time.Time        `json:"start_at,omitempty"`
	Result            *ExecutionSummary `json:"result,omitempty"`
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
