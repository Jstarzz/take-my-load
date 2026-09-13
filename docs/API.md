# Control-plane API

The foundation API is deliberately small. It exposes worker discovery and **planning**, but does not yet remotely execute traffic.

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

```json
{
  "engine": "blast",
  "workers": 2,
  "available_rps": 200000
}
```

Capacity is advertised worker capacity, not a benchmark claim. Later milestones will calibrate and age these values.

## Plan a test

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

Example response:

```json
{
  "id": "38a10f4528d46bc1",
  "name": "health endpoint 100k",
  "target": "http://10.250.0.10:8080/healthz",
  "engine": "blast",
  "requests_per_second": 100000,
  "duration_seconds": 30,
  "available_rps": 200000,
  "shards": [
    {"worker_id": "worker-a", "requests_per_second": 50000},
    {"worker_id": "worker-b", "requests_per_second": 50000}
  ],
  "created_at": "2026-09-12T20:00:00Z"
}
```

Current hard planning limits are 1,000,000 RPS and 3,600 seconds. Execution will add stricter per-project/operator policy rather than treating those values as permission to send traffic.
