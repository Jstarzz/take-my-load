package control

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

const (
	maxPlannedRPS      int64 = 1_000_000
	maxDurationSeconds int64 = 3_600
)

type Server struct {
	registry  *Registry
	policy    *TargetPolicy
	scheduler Scheduler
	version   string
	mux       *http.ServeMux
	now       func() time.Time
}

func NewServer(registry *Registry, version string) *Server {
	policy, _ := ParseTargetPolicy("")
	return NewServerWithPolicy(registry, version, policy)
}

func NewServerWithPolicy(registry *Registry, version string, policy *TargetPolicy) *Server {
	s := &Server{
		registry: registry,
		policy:   policy,
		version:  version,
		mux:      http.NewServeMux(),
		now:      time.Now,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/info", s.handleInfo)
	s.mux.HandleFunc("GET /api/v1/workers", s.handleWorkers)
	s.mux.HandleFunc("POST /api/v1/workers/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/v1/workers/", s.handleWorkerAction)
	s.mux.HandleFunc("GET /api/v1/capacity", s.handleCapacity)
	s.mux.HandleFunc("POST /api/v1/tests/plan", s.handlePlanTest)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, protocol.InfoResponse{Name: "take-my-load", Version: s.version})
}

func (s *Server) handleWorkers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.List())
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var reg protocol.WorkerRegistration
	if err := decodeJSON(r, &reg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(reg.ID) == "" || strings.TrimSpace(reg.Name) == "" {
		writeError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	if len(reg.Engines) == 0 {
		writeError(w, http.StatusBadRequest, "at least one engine is required")
		return
	}
	writeJSON(w, http.StatusCreated, s.registry.Register(reg))
}

func (s *Server) handleWorkerAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/workers/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "heartbeat" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}

	var hb protocol.WorkerHeartbeat
	if err := decodeJSON(r, &hb); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	worker, err := s.registry.Heartbeat(parts[0], hb)
	if errors.Is(err, ErrWorkerNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

func (s *Server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	if engine == "" {
		writeError(w, http.StatusBadRequest, "engine query parameter is required")
		return
	}
	writeJSON(w, http.StatusOK, s.scheduler.Capacity(s.registry.List(), engine))
}

func (s *Server) handlePlanTest(w http.ResponseWriter, r *http.Request) {
	var req protocol.TestPlanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Target = strings.TrimSpace(req.Target)
	req.Engine = strings.TrimSpace(req.Engine)
	if req.Target == "" || req.Engine == "" {
		writeError(w, http.StatusBadRequest, "target and engine are required")
		return
	}
	if req.RequestsPerSecond <= 0 || req.RequestsPerSecond > maxPlannedRPS {
		writeError(w, http.StatusBadRequest, "requests_per_second must be between 1 and 1000000")
		return
	}
	if req.DurationSeconds <= 0 || req.DurationSeconds > maxDurationSeconds {
		writeError(w, http.StatusBadRequest, "duration_seconds must be between 1 and 3600")
		return
	}
	if err := s.policy.Authorize(req.Target); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	shards, capacity, err := s.scheduler.Shard(s.registry.List(), req.Engine, req.RequestsPerSecond)
	if errors.Is(err, ErrInsufficientCapacity) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":         err.Error(),
			"available_rps": capacity,
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build execution plan")
		return
	}

	id, err := newPlanID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create plan id")
		return
	}
	plan := protocol.TestPlan{
		ID:                id,
		Name:              strings.TrimSpace(req.Name),
		Target:            req.Target,
		Engine:            req.Engine,
		RequestsPerSecond: req.RequestsPerSecond,
		DurationSeconds:   req.DurationSeconds,
		AvailableRPS:      capacity,
		Shards:            shards,
		CreatedAt:         s.now().UTC(),
	}
	writeJSON(w, http.StatusCreated, plan)
}

func newPlanID() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
