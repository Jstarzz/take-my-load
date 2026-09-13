CREATE DATABASE IF NOT EXISTS tml;

CREATE TABLE IF NOT EXISTS tml.metric_windows
(
    job_id String,
    worker_id String,
    ts DateTime64(3, 'UTC'),
    sent UInt64,
    received UInt64,
    errors UInt64,
    bytes_sent UInt64,
    bytes_received UInt64,
    p50_us UInt64,
    p90_us UInt64,
    p95_us UInt64,
    p99_us UInt64,
    max_us UInt64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ts)
ORDER BY (job_id, worker_id, ts)
TTL toDateTime(ts) + INTERVAL 180 DAY;

CREATE TABLE IF NOT EXISTS tml.run_summaries
(
    job_id String,
    recorded_at DateTime64(3, 'UTC'),
    requested_rps UInt64,
    achieved_rps Float64,
    total_requests UInt64,
    total_errors UInt64,
    p50_us UInt64,
    p95_us UInt64,
    p99_us UInt64,
    duration_ms UInt64,
    worker_count UInt32,
    engine LowCardinality(String)
)
ENGINE = ReplacingMergeTree(recorded_at)
ORDER BY job_id;
