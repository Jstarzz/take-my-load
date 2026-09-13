package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type Server struct {
	registry *Registry
	version  string
	mux      *http.ServeMux
}

func NewServer(registry *Registry, version string) *Server {
	s := &Server{registry: registry, version: version, mux: http.NewServeMux()}
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
