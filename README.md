# Take My Load

Distributed load-generation and performance-testing platform.

> **Status:** bootstrap repository. The first implementation lands through pull requests; `main` is kept releasable.

The platform is being designed around a Go control plane and worker agent, a Rust high-throughput traffic engine, Kubernetes/k3s orchestration, and pluggable load engines for realistic user flows and raw throughput testing.

See the foundation pull request for the initial architecture, runnable services, tests, CI, and deployment documentation.
