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

type CapacityResponse struct {
	Engine       string `json:"engine"`
	Workers      int    `json:"workers"`
	AvailableRPS int64  `json:"available_rps"`
}

type InfoResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
