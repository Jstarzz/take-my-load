# ADR 0001: Polyglot language boundaries

- Status: Accepted
- Date: 2026-09-12

## Context

The project intentionally uses multiple languages, but language diversity must follow component boundaries rather than creating FFI chains on request hot paths.

## Decision

### Go

Owns the control plane, scheduler, worker agent, CLI, Kubernetes integration, and service orchestration.

### Rust

Owns the custom high-throughput traffic engine and other latency/throughput-sensitive user-space execution paths.

### C

Reserved for eBPF/XDP programs, kernel-facing probes, and narrowly scoped native helpers.

### C++

Reserved for protocol/telemetry modules where mature C++ libraries or specialized native performance work justify it.

### Python

Offline statistics, benchmark analysis, notebooks, regression analysis, and future ML-assisted anomaly classification. Python is not part of the request-generation hot path.

### TypeScript / SvelteKit

Future interactive control dashboard. Browser/UI performance is deliberately decoupled from traffic-generation performance.

## Consequences

- Components communicate using versioned network or process contracts.
- No Go -> Rust -> C++ -> Python call chain in the per-request path.
- A component must justify a new runtime/language before being added.
- Benchmarks determine whether a lower-level implementation is actually necessary.
