# Architecture

## Goals

Take My Load separates the **control plane** from the **traffic-generation data plane**. The control plane should remain lightweight and deterministic while workers perform the expensive work.

The first physical deployment may use one Proxmox host, but logical boundaries must survive later scale-out to additional bare-metal, VM, or cloud workers.

## Planes

### Control plane

Implemented primarily in Go.

Responsibilities:

- API and CLI-facing contracts
- worker registration and health
- test definitions and lifecycle
- target authorization and policy
- capacity-aware scheduling
- synchronized test start/stop
- result aggregation
- audit events

### Data plane

Workers host one or more execution engines. The worker agent does not generate traffic itself; it supervises engines and reports capabilities/results.

Planned engines:

- `tml-blast` (Rust): optimized raw HTTP throughput
- k6: realistic scripted user journeys
- h2load: HTTP/2 and HTTP/3 oriented tests
- wrk2: constant-throughput HTTP tests

### Observability plane

Planned:

- OpenTelemetry for common instrumentation
- VictoriaMetrics for time-series metrics
- ClickHouse for high-volume historical analytics
- Grafana for dashboards
- C/eBPF probes for kernel/network visibility

Per-request telemetry will not be shipped through the control plane at high rates. Workers aggregate counters and histograms locally and emit bounded telemetry windows.

## One-node topology

```text
Proxmox host
|
+-- k3s/control-plane VM or LXC
|   +-- controller
|   +-- state/telemetry services
|
+-- logical worker pods/processes
|
+-- target VM/LXC
|
+-- vmbr-load (internal bridge, no physical port)
```

Multiple logical workers on one host validate distributed orchestration semantics but do **not** provide additional physical compute. True horizontal scaling begins when workers run on additional physical nodes.

## Scale-out topology

```text
                 controller
                     |
               scheduler/bus
          +----------+----------+
          |          |          |
      worker-a   worker-b   worker-c
      node A     node B     node C
          \          |          /
           +------ target ------+
```

Workers advertise capabilities and measured capacity. The scheduler shards requested load according to available capacity rather than dividing work blindly.

## Synchronization

Distributed tests use a prepare/start model:

1. Controller validates policy and capacity.
2. Workers receive an immutable execution plan.
3. Workers acknowledge readiness.
4. Controller issues a shared start deadline.
5. Workers execute against a monotonic local clock.
6. Aggregated windows stream back during execution.
7. Final summaries are persisted after completion.

## State ownership

Planned persistence boundaries:

- PostgreSQL: durable control-plane truth
- PostgreSQL read replicas: reporting/read scale, not same-node HA
- Redis: ephemeral shared state, leases, hot counters/cache
- NATS JetStream: commands/events and durable asynchronous delivery
- ClickHouse: analytical result sets
- VictoriaMetrics: time series
- MinIO: large artifacts and exported reports

NATS and Redis must not become competing sources of truth.
