# Data ownership and event flow

## Durable truth

PostgreSQL is the source of truth for worker inventory snapshots, test plans, jobs, assignments, target policy, and audit events. The controller must be able to rebuild its in-memory view from PostgreSQL after restart.

Read-only/reporting queries should be routable to a PostgreSQL replica. The scheduler never depends on asynchronous replica state for correctness-sensitive transitions.

## Event bus

NATS JetStream is the delivery plane, not the source of truth.

Planned subjects:

```text
tml.worker.<worker-id>.command
tml.worker.<worker-id>.event
tml.job.<job-id>.event
tml.audit.event
```

Commands include prepare, start, cancel, and stop. Worker events include ready, started, metric-window, completed, failed, and heartbeat-derived lifecycle events.

Messages carry stable job/assignment IDs so consumers can be idempotent. A redelivered event must not create a second state transition.

## Redis

Redis is intentionally limited to ephemeral coordination:

- worker leases with TTLs
- short-lived scheduling locks
- hot capacity snapshots
- UI/API caches
- bounded transient counters

Anything whose loss would make us unable to explain what test ran does not belong only in Redis.

## Analytics

Workers aggregate request telemetry locally into bounded time windows. Aggregate windows flow into ClickHouse and time-series metrics into VictoriaMetrics. We do not emit one message/database row per request at high RPS.
