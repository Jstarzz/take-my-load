package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func TestRegisterAndListWorkers(t *testing.T) {
	server := NewServer(NewRegistry(), "test")
	body := []byte(`{"id":"worker-1","name":"Worker 1","capacity_rps":100000,"engines":["blast","k6"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers/register", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d: %s", res.Code, http.StatusCreated, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/workers", nil)
	res = httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", res.Code, http.StatusOK)
	}

	var workers []protocol.WorkerSnapshot
	if err := json.NewDecoder(res.Body).Decode(&workers); err != nil {
		t.Fatalf("decode workers: %v", err)
	}
	if len(workers) != 1 || workers[0].ID != "worker-1" {
		t.Fatalf("workers = %#v, want one worker-1", workers)
	}
}

func TestRegisterRejectsMissingEngines(t *testing.T) {
	server := NewServer(NewRegistry(), "test")
	body := []byte(`{"id":"worker-1","name":"Worker 1","engines":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers/register", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}
