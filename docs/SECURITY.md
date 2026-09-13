# Security and target authorization

Take My Load is intended for systems the operator owns or is explicitly authorized to test.

The traffic-execution milestone must ship with guardrails before high-rate engines are remotely controllable.

## Required execution controls

- Default-deny target policy.
- Explicit allowlist by hostname, IP/CIDR, or signed project configuration.
- DNS resolution validation before execution and during long-running tests.
- Separate ceilings for RPS, concurrency, duration, and bandwidth.
- Mutating methods (`POST`, `PUT`, `PATCH`, `DELETE`) disabled unless explicitly enabled for the test.
- Audit record containing actor, target, requested load, resolved addresses, workers, start/end time, and outcome.
- Authentication between controller and workers; mTLS is the intended end state.
- Emergency stop that is independent of the normal scheduler path.

## Source addresses

Workers may bind to multiple source addresses that belong to the authorized test environment. The project will not implement proxy rotation or controls intended to evade third-party rate limits, WAFs, or abuse protections.

## Secrets

Secrets must not be stored in test definitions or committed YAML. The Kubernetes deployment will use Secrets/external secret injection for credentials and tokens.
