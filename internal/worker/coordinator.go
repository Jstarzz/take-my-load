package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type ControlClient interface {
	Assignments(context.Context, string) ([]protocol.WorkerAssignment, error)
	Transition(context.Context, string, string, string) (protocol.TestJob, error)
}

type Coordinator struct {
	client      ControlClient
	workerID    string
	now         func() time.Time
	executor    Executor
	mu          sync.Mutex
	active      map[string]context.CancelFunc
	asyncErrors chan error
}

func NewCoordinator(client ControlClient, workerID string) *Coordinator {
	return NewCoordinatorWithExecutor(client, workerID, nil)
}

func NewCoordinatorWithExecutor(client ControlClient, workerID string, executor Executor) *Coordinator {
	return &Coordinator{
		client:      client,
		workerID:    workerID,
		now:         time.Now,
		executor:    executor,
		active:      make(map[string]context.CancelFunc),
		asyncErrors: make(chan error, 16),
	}
}

func (c *Coordinator) TickSimulation(ctx context.Context) error {
	assignments, err := c.client.Assignments(ctx, c.workerID)
	if err != nil {
		return fmt.Errorf("fetch assignments: %w", err)
	}
	for _, assignment := range assignments {
		if err := c.advanceSimulation(ctx, assignment); err != nil {
			return err
		}
	}
	return nil
}

func (c *Coordinator) TickExecution(ctx context.Context) error {
	if c.executor == nil {
		return fmt.Errorf("execution coordinator has no executor")
	}
	select {
	case err := <-c.asyncErrors:
		return err
	default:
	}

	assignments, err := c.client.Assignments(ctx, c.workerID)
	if err != nil {
		return fmt.Errorf("fetch assignments: %w", err)
	}
	visible := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		visible[assignment.ID] = struct{}{}
	}
	c.cancelMissing(visible)

	for _, assignment := range assignments {
		switch assignment.State {
		case protocol.AssignmentStatePending:
			if _, err := c.client.Transition(ctx, c.workerID, assignment.ID, "ready"); err != nil {
				return fmt.Errorf("assignment %s ready: %w", assignment.ID, err)
			}
		case protocol.AssignmentStateScheduled:
			if assignment.StartAt == nil || c.now().UTC().Before(*assignment.StartAt) {
				continue
			}
			c.launch(ctx, assignment)
		case protocol.AssignmentStateRunning:
			if !c.isActive(assignment.ID) {
				if _, err := c.client.Transition(ctx, c.workerID, assignment.ID, "failed"); err != nil {
					return fmt.Errorf("assignment %s recover running state: %w", assignment.ID, err)
				}
			}
		}
	}
	return nil
}

func (c *Coordinator) advanceSimulation(ctx context.Context, assignment protocol.WorkerAssignment) error {
	var action string
	switch assignment.State {
	case protocol.AssignmentStatePending:
		action = "ready"
	case protocol.AssignmentStateScheduled:
		if assignment.StartAt == nil || c.now().UTC().Before(*assignment.StartAt) {
			return nil
		}
		action = "started"
	case protocol.AssignmentStateRunning:
		action = "completed"
	case protocol.AssignmentStateReady:
		return nil
	default:
		return nil
	}
	if _, err := c.client.Transition(ctx, c.workerID, assignment.ID, action); err != nil {
		return fmt.Errorf("assignment %s action %s: %w", assignment.ID, action, err)
	}
	return nil
}

func (c *Coordinator) launch(parent context.Context, assignment protocol.WorkerAssignment) {
	ctx, cancel := context.WithCancel(parent)
	c.mu.Lock()
	if _, exists := c.active[assignment.ID]; exists {
		c.mu.Unlock()
		cancel()
		return
	}
	c.active[assignment.ID] = cancel
	c.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			c.mu.Lock()
			delete(c.active, assignment.ID)
			c.mu.Unlock()
		}()

		if _, err := c.client.Transition(ctx, c.workerID, assignment.ID, "started"); err != nil {
			c.reportAsync(fmt.Errorf("assignment %s start: %w", assignment.ID, err))
			return
		}
		if err := c.executor.Run(ctx, assignment); err != nil {
			if ctx.Err() != nil {
				return
			}
			if _, transitionErr := c.client.Transition(ctx, c.workerID, assignment.ID, "failed"); transitionErr != nil {
				c.reportAsync(fmt.Errorf("assignment %s failed (%v), transition: %w", assignment.ID, err, transitionErr))
				return
			}
			c.reportAsync(fmt.Errorf("assignment %s execution failed: %w", assignment.ID, err))
			return
		}
		if _, err := c.client.Transition(ctx, c.workerID, assignment.ID, "completed"); err != nil {
			c.reportAsync(fmt.Errorf("assignment %s complete: %w", assignment.ID, err))
		}
	}()
}

func (c *Coordinator) cancelMissing(visible map[string]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, cancel := range c.active {
		if _, ok := visible[id]; !ok {
			cancel()
		}
	}
}

func (c *Coordinator) isActive(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.active[id]
	return ok
}

func (c *Coordinator) reportAsync(err error) {
	select {
	case c.asyncErrors <- err:
	default:
	}
}
