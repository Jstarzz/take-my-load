package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

const (
	maxPlannedRPS      int64 = 1_000_000
	maxDurationSeconds int64 = 3_600
)

var ErrInvalidPlan = errors.New("invalid test plan")

type CapacityError struct {
	AvailableRPS int64
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf("%s: available_rps=%d", ErrInsufficientCapacity, e.AvailableRPS)
}

func (e *CapacityError) Unwrap() error { return ErrInsufficientCapacity }

type Planner struct {
	registry  *Registry
	policy    *TargetPolicy
	scheduler Scheduler
	now       func() time.Time
}

func NewPlanner(registry *Registry, policy *TargetPolicy) *Planner {
	return &Planner{
		registry: registry,
		policy:   policy,
		now:      time.Now,
	}
}

func (p *Planner) Capacity(engine string) protocol.CapacityResponse {
	return p.scheduler.Capacity(p.registry.List(), strings.TrimSpace(engine))
}

func (p *Planner) Plan(req protocol.TestPlanRequest) (protocol.TestPlan, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Target = strings.TrimSpace(req.Target)
	req.Engine = strings.TrimSpace(req.Engine)
	if req.Target == "" || req.Engine == "" {
		return protocol.TestPlan{}, fmt.Errorf("%w: target and engine are required", ErrInvalidPlan)
	}
	if req.RequestsPerSecond <= 0 || req.RequestsPerSecond > maxPlannedRPS {
		return protocol.TestPlan{}, fmt.Errorf("%w: requests_per_second must be between 1 and 1000000", ErrInvalidPlan)
	}
	if req.DurationSeconds <= 0 || req.DurationSeconds > maxDurationSeconds {
		return protocol.TestPlan{}, fmt.Errorf("%w: duration_seconds must be between 1 and 3600", ErrInvalidPlan)
	}
	if err := p.policy.Authorize(req.Target); err != nil {
		return protocol.TestPlan{}, err
	}

	shards, capacity, err := p.scheduler.Shard(p.registry.List(), req.Engine, req.RequestsPerSecond)
	if errors.Is(err, ErrInsufficientCapacity) {
		return protocol.TestPlan{}, &CapacityError{AvailableRPS: capacity}
	}
	if err != nil {
		return protocol.TestPlan{}, err
	}
	id, err := newID()
	if err != nil {
		return protocol.TestPlan{}, fmt.Errorf("create plan id: %w", err)
	}
	return protocol.TestPlan{
		ID:                id,
		Name:              req.Name,
		Target:            req.Target,
		Engine:            req.Engine,
		RequestsPerSecond: req.RequestsPerSecond,
		DurationSeconds:   req.DurationSeconds,
		AvailableRPS:      capacity,
		Shards:            shards,
		CreatedAt:         p.now().UTC(),
	}, nil
}
