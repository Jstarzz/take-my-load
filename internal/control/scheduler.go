package control

import (
	"errors"
	"sort"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

var ErrInsufficientCapacity = errors.New("insufficient worker capacity")

type Scheduler struct{}

func (Scheduler) Capacity(workers []protocol.WorkerSnapshot, engine string) protocol.CapacityResponse {
	eligible := eligibleWorkers(workers, engine)
	var total int64
	for _, worker := range eligible {
		total += worker.CapacityRPS
	}
	return protocol.CapacityResponse{Engine: engine, Workers: len(eligible), AvailableRPS: total}
}

func (Scheduler) Shard(workers []protocol.WorkerSnapshot, engine string, requestedRPS int64) ([]protocol.WorkerShard, int64, error) {
	eligible := eligibleWorkers(workers, engine)
	var total int64
	for _, worker := range eligible {
		total += worker.CapacityRPS
	}
	if requestedRPS <= 0 || total < requestedRPS {
		return nil, total, ErrInsufficientCapacity
	}

	allocations := make([]int64, len(eligible))
	var allocated int64
	for i, worker := range eligible {
		share := requestedRPS * worker.CapacityRPS / total
		allocations[i] = share
		allocated += share
	}

	for remaining := requestedRPS - allocated; remaining > 0; remaining-- {
		assigned := false
		for i, worker := range eligible {
			if allocations[i] < worker.CapacityRPS {
				allocations[i]++
				assigned = true
				break
			}
		}
		if !assigned {
			return nil, total, ErrInsufficientCapacity
		}
	}

	shards := make([]protocol.WorkerShard, 0, len(eligible))
	for i, worker := range eligible {
		if allocations[i] == 0 {
			continue
		}
		shards = append(shards, protocol.WorkerShard{
			WorkerID:          worker.ID,
			RequestsPerSecond: allocations[i],
		})
	}
	return shards, total, nil
}

func eligibleWorkers(workers []protocol.WorkerSnapshot, engine string) []protocol.WorkerSnapshot {
	eligible := make([]protocol.WorkerSnapshot, 0, len(workers))
	for _, worker := range workers {
		if worker.CapacityRPS <= 0 || !supportsEngine(worker.Engines, engine) {
			continue
		}
		eligible = append(eligible, worker)
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	return eligible
}

func supportsEngine(engines []string, wanted string) bool {
	for _, engine := range engines {
		if engine == wanted {
			return true
		}
	}
	return false
}
