package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/Jstarzz/take-my-load/internal/protocol"
)

type ControlClient interface {
	Assignments(context.Context, string) ([]protocol.WorkerAssignment, error)
	Transition(context.Context, string, string, string) (protocol.TestJob, error)
}

type Coordinator struct {
	client   ControlClient
	workerID string
	now      func() time.Time
}

func NewCoordinator(client ControlClient, workerID string) *Coordinator {
	return &Coordinator{
		client:   client,
		workerID: workerID,
		now:      time.Now,
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
