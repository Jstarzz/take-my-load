CREATE TABLE IF NOT EXISTS workers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT,
    capacity_rps BIGINT NOT NULL DEFAULT 0 CHECK (capacity_rps >= 0),
    engines TEXT[] NOT NULL DEFAULT '{}',
    last_seen TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS test_plans (
    id TEXT PRIMARY KEY,
    name TEXT,
    target TEXT NOT NULL,
    engine TEXT NOT NULL,
    requests_per_second BIGINT NOT NULL CHECK (requests_per_second > 0),
    duration_seconds BIGINT NOT NULL CHECK (duration_seconds > 0),
    available_rps BIGINT NOT NULL CHECK (available_rps >= 0),
    plan JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS test_jobs (
    id TEXT PRIMARY KEY REFERENCES test_plans(id) ON DELETE RESTRICT,
    state TEXT NOT NULL CHECK (state IN ('preparing', 'scheduled', 'running', 'completed', 'failed', 'cancelled')),
    start_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS worker_assignments (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES test_jobs(id) ON DELETE CASCADE,
    worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE RESTRICT,
    target TEXT NOT NULL,
    engine TEXT NOT NULL,
    requests_per_second BIGINT NOT NULL CHECK (requests_per_second > 0),
    duration_seconds BIGINT NOT NULL CHECK (duration_seconds > 0),
    state TEXT NOT NULL CHECK (state IN ('pending', 'ready', 'scheduled', 'running', 'completed', 'failed', 'cancelled')),
    start_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_worker_assignments_worker_state
    ON worker_assignments(worker_id, state);
CREATE INDEX IF NOT EXISTS idx_worker_assignments_job
    ON worker_assignments(job_id);
CREATE INDEX IF NOT EXISTS idx_test_jobs_state_updated
    ON test_jobs(state, updated_at);

CREATE TABLE IF NOT EXISTS target_policies (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pattern TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    job_id TEXT,
    worker_id TEXT,
    target TEXT,
    details JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_events_occurred_at ON audit_events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_job ON audit_events(job_id) WHERE job_id IS NOT NULL;
