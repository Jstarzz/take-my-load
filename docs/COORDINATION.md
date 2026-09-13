# Worker coordination

The worker process has a coordination-only simulation mode used to validate distributed scheduling without generating network load.

Enable it with:

```text
TML_COORDINATION_SIMULATION=true
```

Simulation mode polls the worker's assignment queue and performs only control-plane transitions:

```text
pending -> ready
scheduled --(at start_at)--> running
running -> completed
```

It never invokes a traffic engine and never sends requests to the test target.

This deliberately separates two questions:

1. Can the controller shard work, synchronize workers, propagate cancellation, and converge job state correctly?
2. Can an execution engine produce the requested traffic accurately and efficiently?

The default is `false`. Real engine execution will be added behind a separate explicit execution mode and will still require controller-side target authorization.
