# Roadmap

## M0 — Foundation

- [x] Go controller process
- [x] Go worker agent
- [x] worker registration and heartbeat
- [x] logical multi-worker local topology
- [x] Rust engine shell
- [x] CI and baseline tests
- [x] architecture/security/Proxmox docs

## M1 — Executable test model

- [x] target allowlist and policy evaluator
- [x] immutable capacity-aware execution-plan schema
- [x] job and assignment state machine
- [x] synchronized prepare/start lifecycle
- [x] failure/cancellation propagation
- [ ] engine capability discovery from actual binaries
- [ ] controller-to-worker durable command channel
- [ ] first safe HTTP execution path
- [ ] bounded local histogram/counter aggregation

## M2 — Durable distributed control plane

- [ ] PostgreSQL persistence
- [ ] PgBouncer
- [ ] NATS JetStream command/event bus
- [ ] Redis ephemeral coordination where justified
- [ ] worker leases and failure detection
- [ ] capacity-aware load sharding
- [ ] retry/idempotency model

## M3 — High-throughput engine

- [ ] Rust HTTP/1.1 engine
- [ ] connection pooling/reuse
- [ ] precise constant-rate scheduler
- [ ] CPU affinity support
- [ ] source-address pools for controlled lab networks
- [ ] local HDR-style latency histograms
- [ ] benchmark harness and profiler workflow

## M4 — Observability and analytics

- [ ] OpenTelemetry
- [ ] VictoriaMetrics
- [ ] Grafana dashboards
- [ ] ClickHouse historical analytics
- [ ] MinIO artifacts
- [ ] C/eBPF network and scheduler probes
- [ ] regression comparison reports

## M5 — UX and external engines

- [ ] SvelteKit control dashboard
- [ ] k6 adapter
- [ ] h2load adapter
- [ ] wrk2 adapter
- [ ] OpenAPI importer
- [ ] scenario DSL
- [ ] CI performance-test API

## M6 — Scale-out

- [ ] multi-physical-node worker joining
- [ ] topology-aware scheduling
- [ ] fault injection / worker-loss testing
- [ ] optional Kubernetes operator/CRD
- [ ] aggregate 1M RPS validation on appropriately sized hardware/networking

The 1M RPS item is a validation milestone, not a promise that the initial dual-Xeon single-node deployment can produce or absorb that rate.
