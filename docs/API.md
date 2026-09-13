# Control-plane API

The control plane exposes worker discovery, safe test planning, and a distributed job/assignment state machine. Traffic execution is not remotely enabled yet.

## Environment

`TML_ALLOWED_TARGETS` is a comma-separated allowlist of exact hostnames, IP addresses, or CIDRs.

Example for the isolated Proxmox load network:

```text
TML_ALLOWED_TARGETS=10.250.0.0/24,app.loadtest.internal
```

An empty allowlist denies all test targets.

## Worker registration

`POST /api/v1/workers/register`

```json
{
  "id": "worker-a",
  "name": "Worker A",
  "capacity_rps": 100000,
  "engines": ["blast", "k6"]
}
```

Workers keep themselves live with `POST /api/v1/workers/{id}/heartbeat`.

## Capacity

`GET /api/v1/capacity?engine=blast`

Capacity is advertised worker capacity, not a benchmark claim. Later milestones will calibrate and age these values.

## Preview a plan

`POST /api/v1/tests/plan`

```json
{
  "name": "health endpoint 100k",
  "target": "http://10.250.0.10:8080/healthz",
  "engine": "blast",
  "requests_per_second": 100000,
  "duration_seconds": 30
}
```

The controller validates the target against the allowlist, checks the global planning ceiling, filters workers by engine support, and shards the requested rate proportionally to advertised worker capacity.

Current hard planning limits are 1,000,000 RPS and 3,600 seconds. Those values are ceilings for constructing a plan, not permission to send traffic.

## Submit a distributed job

`POST /api/v1/tests` accepts the same body as the planning endpoint. It creates a job in `preparing` state and one assignment per selected worker.

Workers discover their work through:

`GET /api/v1/workers/{worker_id}/assignments`

Each assignment begins in `pending`. Workers acknowledge preparation with:

`POST /api/v1/workers/{worker_id}/assignments/{assignment_id}/ready`

When every worker is ready, the controller atomically changes all assignments to `scheduled` and gives them the **same `start_at` timestamp**. This is the basis for synchronized distributed spikes and constant-rate tests.

At or after `start_at`, workers report:

```text
POST .../{assignment_id}/started
POST .../{assignment_id}/completed
```

A worker may report `failed` from any non-terminal state. Failure marks the job failed and cancels peer assignments so a partial distributed test does not continue silently.

## Read and cancel jobs

```text
GET  /api/v1/tests/{id}
POST /api/v1/tests/{id}/cancel
```

This state is currently in memory. PostgreSQL persistence and NATS-based delivery replace the in-memory transport in the durable-control-plane milestone while preserving these lifecycle semantics.
