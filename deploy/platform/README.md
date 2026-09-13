# One-node platform services

This compose layer provides the durable/observability dependencies for the single-Proxmox-node deployment. Run it together with the root application compose file:

```bash
cp deploy/platform/.env.example deploy/platform/.env
# edit every password first
docker compose \
  --env-file deploy/platform/.env \
  -f compose.yaml \
  -f deploy/platform/compose.yaml \
  up -d
```

## Services and ownership

| Service | Owns | Must not own |
| --- | --- | --- |
| PostgreSQL primary | durable control-plane truth, jobs, assignments, policies, audit events | high-volume per-request metrics |
| PostgreSQL replica | report/read traffic and replica-path testing | failover claims on a single physical node |
| PgBouncer | PostgreSQL connection pooling | durable state |
| NATS JetStream | commands/events, durable async delivery | canonical job state |
| Redis | leases, hot ephemeral coordination, short-lived counters/cache | canonical job state or benchmark history |
| ClickHouse | historical test windows and run analytics | scheduler truth |
| VictoriaMetrics | time-series telemetry | relational state |
| OpenTelemetry Collector | telemetry ingestion/routing | storage |
| Grafana | visualization | storage |

## Read replica caveat

The replica uses PostgreSQL streaming replication, but both instances live on the same physical deployment by default. That gives us real read/write routing semantics and lets us rehearse failover procedures; it does **not** provide host-level high availability. Put the replica on another physical node before calling it HA.

## Artifact storage

The architecture keeps an object-store boundary for raw reports, traces, OpenAPI files, and large artifacts. MinIO Community Edition is source-only as of 2026, so this baseline deliberately does not hide an unmaintained legacy binary image behind an `S3-compatible` label. We can either self-build MinIO from source or plug another S3-compatible store into that boundary later.

## Pinned versions

The baseline intentionally pins service versions instead of using `latest`:

- PostgreSQL 18.6
- PgBouncer 1.25.2
- NATS Server 2.14.6
- Redis Open Source 8.4.6
- ClickHouse 26.8.2.7
- VictoriaMetrics 1.148.3
- OpenTelemetry Collector Contrib 0.160.0
- Grafana 13.2.1

Upgrade these as one reviewed change; do not silently drift infrastructure underneath benchmark comparisons.
