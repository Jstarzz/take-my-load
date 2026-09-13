# Take My Load

A distributed load-generation and performance-testing platform built to scale from a single Proxmox node to a multi-node worker fabric.

The design target is **10k+ concurrent virtual users**, **100k+ RPS-class local generation**, and an eventual **1M aggregate RPS** across enough workers and network capacity. Those are architecture targets, **not benchmark claims**.

## Current status

The current foundation is a real distributed control path, not just a scaffold:

- Go control plane with health/readiness, worker registration, heartbeat, inventory, capacity, planning, job and assignment APIs.
- Default-deny target authorization with exact-host and CIDR allowlists.
- Deterministic capacity-weighted sharding with a 1,000,000 RPS planning ceiling.
- Distributed job state machine with ready barriers, synchronized `start_at`, cancellation and failure propagation.
- Go worker agent with coordination simulation and explicit real-execution modes.
- Rust `tml-blast` HTTP/1.1 fixed-rate engine with pooled connections, bounded concurrency, backpressure accounting and latency/status summaries.
- Optional PostgreSQL-backed durable control-plane state, with in-memory storage retained for lightweight local development.
- One-node platform stack including PostgreSQL primary + streaming read replica, PgBouncer, NATS JetStream, Redis, ClickHouse, VictoriaMetrics, OpenTelemetry and Grafana.
- Docker Compose and k3s/Kubernetes deployment foundations.
- Go unit tests, PostgreSQL integration tests, Rust fmt/clippy/tests, container builds and Compose validation in CI.

The million-RPS number remains a **distributed platform capacity target**. It has not been benchmarked on the current hardware.

## Architecture

```text
                         Web / CLI
                            |
                      Go Control Plane
               API + scheduler + target policy
                            |
              durable state / control contracts
                            |
             +--------------+--------------+
             |              |              |
          Go Agent       Go Agent       Go Agent
             |              |              |
        Rust blast      Rust blast      Rust blast
             \              |              /
              +-------------+-------------+
                            |
                   authorized target

       PostgreSQL / PgBouncer      NATS / Redis
       ClickHouse / VictoriaMetrics / OTel / Grafana
```

The current deployment can live entirely on one physical Proxmox host. Logical distribution is kept from day one so adding physical workers later does not require redesigning the control plane.

## Runtime modes

Workers deliberately do **nothing** with assignments unless one of these modes is enabled.

### Coordination simulation

```text
TML_COORDINATION_SIMULATION=true
TML_EXECUTION_ENABLED=false
```

Workers register, poll assignments, cross the ready barrier, honor the synchronized start time and complete the state machine without generating target traffic. This is the default Docker Compose behavior.

### Real blast execution

```text
TML_COORDINATION_SIMULATION=false
TML_EXECUTION_ENABLED=true
TML_BLAST_BINARY=/usr/local/bin/tml-blast
TML_BLAST_CONCURRENCY=4096
```

The execution-capable worker image contains `tml-blast`. A worker only executes assignments issued by the controller; target authorization therefore remains in the control-plane path rather than being bypassed by the worker.

`tml-blast` is currently alpha and supports explicit `http://` HTTP/1.1 targets. HTTPS, HTTP/2, HTTP/3 and richer request models are later milestones.

## Quick start

Requires Go 1.27.x. Rust stable is required when building `tml-blast` outside Docker.

```bash
go test ./...
go run ./cmd/controller
```

In another shell:

```bash
TML_WORKER_ID=worker-1 \
TML_WORKER_NAME='Worker 1' \
TML_WORKER_CAPACITY_RPS=100000 \
TML_COORDINATION_SIMULATION=true \
go run ./cmd/worker
```

Inspect the registry:

```bash
curl http://127.0.0.1:8080/api/v1/workers
```

### Base Compose: lightweight simulation

```bash
docker compose up --build
```

This starts the controller and two logical workers. The controller uses in-memory state and the workers stay in coordination-simulation mode.

### Full one-node platform

```bash
docker compose -f compose.yaml -f deploy/platform/compose.yaml up --build
```

The platform overlay injects `TML_DATABASE_URL`, routing controller persistence through PgBouncer to PostgreSQL. The same-node PostgreSQL replica is useful for read-path and replication development but **is not high availability**; both instances fail with the physical Proxmox node.

## Planning a test

The controller is default-deny. Configure `TML_ALLOWED_TARGETS` with hosts or CIDRs you are authorized to test, for example:

```text
TML_ALLOWED_TARGETS=10.250.0.0/24,127.0.0.1,localhost
```

A plan request looks like:

```json
{
  "name": "health-100k",
  "target": "http://10.250.0.10:8080/healthz",
  "engine": "blast",
  "requests_per_second": 100000,
  "duration_seconds": 30
}
```

The scheduler rejects unauthorized targets and jobs above currently registered aggregate worker capacity.

## Rust engine

From `engine/blast`:

```bash
cargo run -- --capabilities
cargo run -- run \
  --target http://127.0.0.1:8080/healthz \
  --rps 10000 \
  --duration-seconds 10 \
  --concurrency 4096
```

The engine prints one JSON summary containing scheduled/started/completed/failed requests, backpressure, response bytes, status classes, achieved RPS and approximate p50/p95/p99 latency.

## Repository layout

```text
cmd/controller/                    Go control-plane process
cmd/worker/                        Go worker agent
internal/control/                  scheduler, policy and job state machine
internal/persistence/postgres/     durable pgx repository
internal/protocol/                 shared wire/domain models
internal/worker/                   control client, coordinator and executors
engine/blast/                      Rust high-throughput HTTP engine

deploy/docker/                     controller and execution-capable worker images
deploy/k8s/                        single-node-ready k3s manifests
deploy/platform/                   durable/observability platform stack

docs/                              architecture, security, deployment and ADRs
```

## Technology ownership

- **Go**: control plane, scheduling, worker agents, persistence integration and orchestration.
- **Rust**: high-throughput userspace traffic generation.
- **C**: reserved for future eBPF/XDP and narrowly scoped kernel-facing instrumentation.
- **Python**: future offline statistics, regression analysis and notebooks; never the request hot path.
- **TypeScript/SvelteKit**: future interactive dashboard only.
- **C++**: intentionally not part of the baseline architecture.

## Safety model

Take My Load is for systems you own or are explicitly authorized to test. Execution is built around default-deny target authorization, explicit worker execution enablement, rate ceilings and auditable controller-issued assignments. See [`docs/SECURITY.md`](docs/SECURITY.md).
