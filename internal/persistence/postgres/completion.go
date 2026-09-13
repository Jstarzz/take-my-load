package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Jstarzz/take-my-load/internal/control"
	"github.com/Jstarzz/take-my-load/internal/protocol"
)

func (s *Store) CompleteAssignment(workerID, assignmentID string, result protocol.ExecutionSummary) (protocol.TestJob, error) {
	ctx, cancel := s.context()
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return protocol.TestJob{}, fmt.Errorf("begin completion transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var jobID string
	if err := tx.QueryRow(ctx, `SELECT job_id FROM worker_assignments WHERE id = $1`, assignmentID).Scan(&jobID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return protocol.TestJob{}, control.ErrAssignmentNotFound
		}
		return protocol.TestJob{}, fmt.Errorf("find assignment job: %w", err)
	}
	job, err := loadJob(ctx, tx, jobID, true)
	if err != nil {
		return protocol.TestJob{}, err
	}
	if err := control.ApplyAssignmentCompletion(&job, workerID, assignmentID, result, s.now()); err != nil {
		return protocol.TestJob{}, err
	}
	if err := persistExistingJob(ctx, tx, job); err != nil {
		return protocol.TestJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return protocol.TestJob{}, fmt.Errorf("commit completion transaction: %w", err)
	}
	return job, nil
}

var _ context.Context
