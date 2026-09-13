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

type InfoResponse struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
