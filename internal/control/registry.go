package control

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

var ErrWorkerNotFound = errors.New("worker not found")

type Registry struct {
	mu      sync.RWMutex
	workers map[string]protocol.WorkerSnapshot
	now     func() time.Time
}

func NewRegistry() *Registry {
	return &Registry{
		workers: make(map[string]protocol.WorkerSnapshot),
		now:     time.Now,
	}
}

func (r *Registry) Register(reg protocol.WorkerRegistration) (protocol.WorkerSnapshot, error) {
	now := r.now().UTC()
	worker := protocol.WorkerSnapshot{
		ID:          reg.ID,
		Name:        reg.Name,
		Address:     reg.Address,
		CapacityRPS: reg.CapacityRPS,
		Engines:     append([]string(nil), reg.Engines...),
		LastSeen:    now,
	}

	r.mu.Lock()
	r.workers[worker.ID] = worker
	r.mu.Unlock()

	return worker, nil
}

func (r *Registry) Heartbeat(id string, hb protocol.WorkerHeartbeat) (protocol.WorkerSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	worker, ok := r.workers[id]
	if !ok {
		return protocol.WorkerSnapshot{}, ErrWorkerNotFound
	}
	if hb.CapacityRPS > 0 {
		worker.CapacityRPS = hb.CapacityRPS
	}
	worker.LastSeen = r.now().UTC()
	r.workers[id] = worker
	return worker, nil
}

func (r *Registry) Get(id string) (protocol.WorkerSnapshot, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	worker, ok := r.workers[id]
	if !ok {
		return protocol.WorkerSnapshot{}, false, nil
	}
	worker.Engines = append([]string(nil), worker.Engines...)
	return worker, true, nil
}

func (r *Registry) List() ([]protocol.WorkerSnapshot, error) {
	r.mu.RLock()
	workers := make([]protocol.WorkerSnapshot, 0, len(r.workers))
	for _, worker := range r.workers {
		worker.Engines = append([]string(nil), worker.Engines...)
		workers = append(workers, worker)
	}
	r.mu.RUnlock()

	sort.Slice(workers, func(i, j int) bool { return workers[i].ID < workers[j].ID })
	return workers, nil
}
