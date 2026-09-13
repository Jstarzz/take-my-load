package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type Server struct {
	registry WorkerRepository
	planner  *Planner
	jobs     JobRepository
	version  string
	mux      *http.ServeMux
}

func NewServer(registry WorkerRepository, version string) *Server {
	policy, _ := ParseTargetPolicy("")
	return NewServerWithPolicy(registry, version, policy)
}

func NewServerWithPolicy(registry WorkerRepository, version string, policy *TargetPolicy) *Server {
	return NewServerWithRepositories(registry, NewJobStore(), version, policy)
}

func NewServerWithRepositories(registry WorkerRepository, jobs JobRepository, version string, policy *TargetPolicy) *Server {
	s := &Server{
		registry: registry,
		planner:  NewPlanner(registry, policy),
		jobs:     jobs,
		version:  version,
		mux:      http.NewServeMux(),
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
	s.mux.HandleFunc("POST /api/v1/workers/{id}/heartbeat", s.handleHeartbeat)
	s.mux.HandleFunc("GET /api/v1/workers/{id}/assignments", s.handleAssignments)
	s.mux.HandleFunc("POST /api/v1/workers/{worker_id}/assignments/{assignment_id}/result", s.handleAssignmentResult)
	s.mux.HandleFunc("POST /api/v1/workers/{worker_id}/assignments/{assignment_id}/{action}", s.handleAssignmentAction)
	s.mux.HandleFunc("GET /api/v1/capacity", s.handleCapacity)
	s.mux.HandleFunc("POST /api/v1/tests/plan", s.handlePlanTest)
	s.mux.HandleFunc("POST /api/v1/tests", s.handleSubmitTest)
	s.mux.HandleFunc("GET /api/v1/tests/{id}", s.handleGetTest)
	s.mux.HandleFunc("POST /api/v1/tests/{id}/cancel", s.handleCancelTest)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, protocol.InfoResponse{Name: "take-my-load", Version: s.version})
}

func (s *Server) handleWorkers(w http.ResponseWriter, _ *http.Request) {
	workers, err := s.registry.ListWorkers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workers")
		return
	}
	writeJSON(w, http.StatusOK, workers)
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
	worker, err := s.registry.RegisterWorker(reg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register worker")
		return
	}
	writeJSON(w, http.StatusCreated, worker)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hb protocol.WorkerHeartbeat
	if err := decodeJSON(r, &hb); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	worker, err := s.registry.HeartbeatWorker(r.PathValue("id"), hb)
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

func (s *Server) handleAssignments(w http.ResponseWriter, r *http.Request) {
	workerID := r.PathValue("id")
	_, ok, err := s.registry.GetWorker(workerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read worker")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, ErrWorkerNotFound.Error())
		return
	}
	assignments, err := s.jobs.ListAssignments(workerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}
	writeJSON(w, http.StatusOK, assignments)
}

func (s *Server) handleAssignmentAction(w http.ResponseWriter, r *http.Request) {
	states := map[string]protocol.AssignmentState{
		"ready":     protocol.AssignmentStateReady,
		"started":   protocol.AssignmentStateRunning,
		"completed": protocol.AssignmentStateCompleted,
		"failed":    protocol.AssignmentStateFailed,
	}
	next, ok := states[r.PathValue("action")]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown assignment action")
		return
	}
	job, err := s.jobs.TransitionAssignment(r.PathValue("worker_id"), r.PathValue("assignment_id"), next)
	writeAssignmentMutation(w, job, err)
}

func (s *Server) handleAssignmentResult(w http.ResponseWriter, r *http.Request) {
	var result protocol.ExecutionSummary
	if err := decodeJSON(r, &result); err != nil {
		writeError(w, http.StatusBadRequest, "invalid execution result")
		return
	}
	job, err := s.jobs.CompleteAssignment(r.PathValue("worker_id"), r.PathValue("assignment_id"), result)
	writeAssignmentMutation(w, job, err)
}

func writeAssignmentMutation(w http.ResponseWriter, job protocol.TestJob, err error) {
	switch {
	case errors.Is(err, ErrAssignmentNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrAssignmentOwner):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case errors.Is(err, ErrInvalidResult):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, ErrInvalidTransition), errors.Is(err, ErrStartTimeNotReached):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "assignment mutation failed")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	if engine == "" {
		writeError(w, http.StatusBadRequest, "engine query parameter is required")
		return
	}
	capacity, err := s.planner.Capacity(engine)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to calculate capacity")
		return
	}
	writeJSON(w, http.StatusOK, capacity)
}

func (s *Server) handlePlanTest(w http.ResponseWriter, r *http.Request) {
	req, ok := decodePlanRequest(w, r)
	if !ok {
		return
	}
	plan, err := s.planner.Plan(req)
	if err != nil {
		writePlanError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, plan)
}

func (s *Server) handleSubmitTest(w http.ResponseWriter, r *http.Request) {
	req, ok := decodePlanRequest(w, r)
	if !ok {
		return
	}
	plan, err := s.planner.Plan(req)
	if err != nil {
		writePlanError(w, err)
		return
	}
	job, err := s.jobs.CreateJob(plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create test job")
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (s *Server) handleGetTest(w http.ResponseWriter, r *http.Request) {
	job, err := s.jobs.GetJob(r.PathValue("id"))
	if errors.Is(err, ErrJobNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read test job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelTest(w http.ResponseWriter, r *http.Request) {
	job, err := s.jobs.CancelJob(r.PathValue("id"))
	switch {
	case errors.Is(err, ErrJobNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrInvalidTransition):
		writeError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to cancel test job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func decodePlanRequest(w http.ResponseWriter, r *http.Request) (protocol.TestPlanRequest, bool) {
	var req protocol.TestPlanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return protocol.TestPlanRequest{}, false
	}
	return req, true
}

func writePlanError(w http.ResponseWriter, err error) {
	var capacityErr *CapacityError
	switch {
	case errors.Is(err, ErrInvalidPlan):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrTargetNotAllowed):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.As(err, &capacityErr):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":         ErrInsufficientCapacity.Error(),
			"available_rps": capacityErr.AvailableRPS,
		})
	default:
		writeError(w, http.StatusInternalServerError, "failed to build execution plan")
	}
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
