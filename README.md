# Take My Load

A distributed load-generation and performance-testing platform built to scale from a single Proxmox node to a multi-node worker fabric.

The design target is **10k+ concurrent virtual users**, **100k+ RPS-class local generation**, and an eventual **1M aggregate RPS** across enough workers and network capacity. Those are architecture targets, **not benchmark claims**.

## Foundation status

The first slice provides:

- Go control-plane HTTP service with health, worker registration, heartbeat, and inventory endpoints.
- Go worker agent with retrying registration and periodic heartbeats.
- Rust `tml-blast` engine shell and capability contract.
- Docker Compose topology with one controller and two logical workers.
- k3s/Kubernetes manifests for the same topology.
- Go tests, Rust checks, and GitHub Actions CI.
- Architecture, Proxmox networking, security, roadmap, and language-boundary documentation.

Traffic execution is intentionally **not** wired into the foundation slice. The next milestone adds the test/job model, target authorization, scheduling, and an executable engine contract before any high-rate traffic path is enabled.

## Architecture

```text
                         Web / CLI
                            |
                      Go Control Plane
                  API + scheduler + policy
                            |
                    control/event plane
                            |
             +--------------+--------------+
             |              |              |
          Go Agent       Go Agent       Go Agent
             |              |              |
       Rust / k6 /     Rust / k6 /     Rust / k6 /
       h2load / wrk2   h2load / wrk2   h2load / wrk2
             \              |              /
              +-------------+-------------+
                            |
                   authorized target
```

The current deployment can live entirely on one physical Proxmox host. Logical distribution is kept from day one so adding physical workers later does not require redesigning the control plane.

## Quick start

Requires Go 1.23+.

```bash
go test ./...
go run ./cmd/controller
```

In another shell:

```bash
TML_WORKER_ID=worker-1 \
TML_WORKER_NAME='Worker 1' \
TML_WORKER_CAPACITY_RPS=100000 \
go run ./cmd/worker
```

Inspect the registry:

```bash
curl http://127.0.0.1:8080/api/v1/workers
```

Or run two logical workers with Docker Compose:

```bash
docker compose up --build
```

## Repository layout

```text
cmd/controller/        Go control-plane process
cmd/worker/            Go worker-agent process
internal/control/      registry and HTTP control API
internal/protocol/     shared wire models
internal/worker/       worker-side control-plane client
engine/blast/          Rust high-throughput engine

deploy/docker/         container images
deploy/k8s/            single-node-ready k3s manifests

docs/                  architecture, security, deployment and ADRs
```

## Planned platform components

The architecture intentionally leaves room for NATS JetStream, PostgreSQL + read replicas, PgBouncer, Redis for ephemeral coordination, VictoriaMetrics, ClickHouse, MinIO, OpenTelemetry, eBPF/XDP instrumentation, SvelteKit, k6, h2load, wrk2, and a custom Rust traffic engine.

They are added only when a concrete interface needs them. No decorative infrastructure.

## Safety model

Take My Load is for systems you own or are explicitly authorized to test. The execution layer is being designed around default-deny target authorization, explicit opt-in for mutating HTTP methods, rate ceilings, audit logs, and worker identity. See [`docs/SECURITY.md`](docs/SECURITY.md).
