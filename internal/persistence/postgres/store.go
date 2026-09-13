package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jstarzz/take-my-load/internal/control"
	"github.com/Jstarzz/take-my-load/internal/protocol"
)

const defaultTimeout = 5 * time.Second

type Store struct {
	pool         *pgxpool.Pool
	timeout      time.Duration
	now          func() time.Time
	scheduleLead time.Duration
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return New(pool), nil
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:         pool,
		timeout:      defaultTimeout,
		now:          time.Now,
		scheduleLead: 3 * time.Second,
	}
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) RegisterWorker(reg protocol.WorkerRegistration) (protocol.WorkerSnapshot, error) {
	ctx, cancel := s.context()
	defer cancel()
	now := s.now().UTC()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO workers (id, name, address, capacity_rps, engines, last_seen, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			address = EXCLUDED.address,
			capacity_rps = EXCLUDED.capacity_rps,
			engines = EXCLUDED.engines,
			last_seen = EXCLUDED.last_seen,
			updated_at = EXCLUDED.updated_at
		RETURNING id, name, address, capacity_rps, engines, last_seen`,
		reg.ID, reg.Name, reg.Address, reg.CapacityRPS, reg.Engines, now,
	)
	worker, err := scanWorker(row)
	if err != nil {
		return protocol.WorkerSnapshot{}, fmt.Errorf("register worker: %w", err)
	}
	return worker, nil
}

func (s *Store) HeartbeatWorker(id string, hb protocol.WorkerHeartbeat) (protocol.WorkerSnapshot, error) {
	ctx, cancel := s.context()
	defer cancel()
	now := s.now().UTC()
	row := s.pool.QueryRow(ctx, `
		UPDATE workers
		SET capacity_rps = CASE WHEN $2 > 0 THEN $2 ELSE capacity_rps END,
			last_seen = $3,
			updated_at = $3
		WHERE id = $1
		RETURNING id, name, address, capacity_rps, engines, last_seen`,
		id, hb.CapacityRPS, now,
	)
	worker, err := scanWorker(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return protocol.WorkerSnapshot{}, control.ErrWorkerNotFound
	}
	if err != nil {
		return protocol.WorkerSnapshot{}, fmt.Errorf("heartbeat worker: %w", err)
	}
	return worker, nil
}

func (s *Store) GetWorker(id string) (protocol.WorkerSnapshot, bool, error) {
	ctx, cancel := s.context()
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, address, capacity_rps, engines, last_seen
		FROM workers WHERE id = $1`, id)
	worker, err := scanWorker(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return protocol.WorkerSnapshot{}, false, nil
	}
	if err != nil {
		return protocol.WorkerSnapshot{}, false, fmt.Errorf("get worker: %w", err)
	}
	return worker, true, nil
}

func (s *Store) ListWorkers() ([]protocol.WorkerSnapshot, error) {
	ctx, cancel := s.context()
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, address, capacity_rps, engines, last_seen
		FROM workers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	defer rows.Close()

	workers := make([]protocol.WorkerSnapshot, 0)
	for rows.Next() {
		worker, err := scanWorker(rows)
		if err != nil {
			return nil, fmt.Errorf("scan worker: %w", err)
		}
		workers = append(workers, worker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workers: %w", err)
	}
	return workers, nil
}

func (s *Store) CreateJob(plan protocol.TestPlan) (protocol.TestJob, error) {
	job, err := control.BuildJob(plan, s.now())
	if err != nil {
		return protocol.TestJob{}, err
	}
	ctx, cancel := s.context()
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return protocol.TestJob{}, fmt.Errorf("begin create job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	planJSON, err := json.Marshal(plan)
	if err != nil {
		return protocol.TestJob{}, fmt.Errorf("marshal plan: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO test_plans
			(id, name, target, engine, requests_per_second, duration_seconds, available_rps, plan, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)`,
		plan.ID, plan.Name, plan.Target, plan.Engine, plan.RequestsPerSecond,
		plan.DurationSeconds, plan.AvailableRPS, string(planJSON), plan.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return protocol.TestJob{}, control.ErrJobAlreadyExists
		}
		return protocol.TestJob{}, fmt.Errorf("insert test plan: %w", err)
	}
	if err := persistNewJob(ctx, tx, job); err != nil {
		return protocol.TestJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return protocol.TestJob{}, fmt.Errorf("commit create job transaction: %w", err)
	}
	return job, nil
}

func (s *Store) GetJob(id string) (protocol.TestJob, error) {
	ctx, cancel := s.context()
	defer cancel()
	return loadJob(ctx, s.pool, id, false)
}

func (s *Store) ListAssignments(workerID string) ([]protocol.WorkerAssignment, error) {
	ctx, cancel := s.context()
	defer cancel()
	rows, err := s.pool.Query(ctx, `
		SELECT snapshot
		FROM worker_assignments
		WHERE worker_id = $1
		  AND state NOT IN ('completed', 'failed', 'cancelled')
		ORDER BY id`, workerID)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	defer rows.Close()

	assignments := make([]protocol.WorkerAssignment, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		var assignment protocol.WorkerAssignment
		if err := json.Unmarshal(raw, &assignment); err != nil {
			return nil, fmt.Errorf("decode assignment: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assignments: %w", err)
	}
	return assignments, nil
}

func (s *Store) CancelJob(id string) (protocol.TestJob, error) {
	return s.mutateJob(id, func(job *protocol.TestJob) error {
		return control.ApplyJobCancellation(job, s.now())
	})
}

func (s *Store) TransitionAssignment(workerID, assignmentID string, next protocol.AssignmentState) (protocol.TestJob, error) {
	ctx, cancel := s.context()
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return protocol.TestJob{}, fmt.Errorf("begin transition transaction: %w", err)
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
	if err := control.ApplyAssignmentTransition(&job, workerID, assignmentID, next, s.now(), s.scheduleLead); err != nil {
		return protocol.TestJob{}, err
	}
	if err := persistExistingJob(ctx, tx, job); err != nil {
		return protocol.TestJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return protocol.TestJob{}, fmt.Errorf("commit transition transaction: %w", err)
	}
	return job, nil
}

func (s *Store) mutateJob(id string, mutate func(*protocol.TestJob) error) (protocol.TestJob, error) {
	ctx, cancel := s.context()
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return protocol.TestJob{}, fmt.Errorf("begin job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	job, err := loadJob(ctx, tx, id, true)
	if err != nil {
		return protocol.TestJob{}, err
	}
	if err := mutate(&job); err != nil {
		return protocol.TestJob{}, err
	}
	if err := persistExistingJob(ctx, tx, job); err != nil {
		return protocol.TestJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return protocol.TestJob{}, fmt.Errorf("commit job transaction: %w", err)
	}
	return job, nil
}

func (s *Store) context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.timeout)
}

type scanner interface {
	Scan(...any) error
}

func scanWorker(row scanner) (protocol.WorkerSnapshot, error) {
	var worker protocol.WorkerSnapshot
	if err := row.Scan(&worker.ID, &worker.Name, &worker.Address, &worker.CapacityRPS, &worker.Engines, &worker.LastSeen); err != nil {
		return protocol.WorkerSnapshot{}, err
	}
	return worker, nil
}

type jobQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadJob(ctx context.Context, db jobQuerier, id string, forUpdate bool) (protocol.TestJob, error) {
	query := `SELECT snapshot FROM test_jobs WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var raw []byte
	if err := db.QueryRow(ctx, query, id).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return protocol.TestJob{}, control.ErrJobNotFound
		}
		return protocol.TestJob{}, fmt.Errorf("load job: %w", err)
	}
	var job protocol.TestJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return protocol.TestJob{}, fmt.Errorf("decode job snapshot: %w", err)
	}
	return job, nil
}

func persistNewJob(ctx context.Context, tx pgx.Tx, job protocol.TestJob) error {
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO test_jobs (id, state, start_at, snapshot, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)`,
		job.ID, job.State, nullableTime(job.StartAt), string(jobJSON), job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	for _, assignment := range job.Assignments {
		assignmentJSON, err := json.Marshal(assignment)
		if err != nil {
			return fmt.Errorf("marshal assignment: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO worker_assignments
				(id, job_id, worker_id, target, engine, requests_per_second, duration_seconds, state, start_at, snapshot, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $11)`,
			assignment.ID, assignment.JobID, assignment.WorkerID, assignment.Target,
			assignment.Engine, assignment.RequestsPerSecond, assignment.DurationSeconds,
			assignment.State, nullableTime(assignment.StartAt), string(assignmentJSON), job.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("insert assignment %s: %w", assignment.ID, err)
		}
	}
	return nil
}

func persistExistingJob(ctx context.Context, tx pgx.Tx, job protocol.TestJob) error {
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE test_jobs
		SET state = $2, start_at = $3, snapshot = $4::jsonb, updated_at = $5
		WHERE id = $1`, job.ID, job.State, nullableTime(job.StartAt), string(jobJSON), job.UpdatedAt); err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	for _, assignment := range job.Assignments {
		assignmentJSON, err := json.Marshal(assignment)
		if err != nil {
			return fmt.Errorf("marshal assignment: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE worker_assignments
			SET state = $2, start_at = $3, snapshot = $4::jsonb, updated_at = $5
			WHERE id = $1`, assignment.ID, assignment.State, nullableTime(assignment.StartAt), string(assignmentJSON), job.UpdatedAt); err != nil {
			return fmt.Errorf("update assignment %s: %w", assignment.ID, err)
		}
	}
	return nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
