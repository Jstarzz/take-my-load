# Proxmox deployment notes

## Internal load network

For high-rate testing on a single Proxmox host, use a dedicated Linux bridge with no physical bridge port. This keeps test traffic inside the host and avoids making the physical school/LAN switch part of the benchmark path.

Example `/etc/network/interfaces` fragment:

```text
auto vmbr-load
iface vmbr-load inet manual
    bridge-ports none
    bridge-stp off
    bridge-fd 0
```

Attach a second virtual NIC from the target and load-worker guests to `vmbr-load`, then assign addresses from a private test-only subnet.

Example:

```text
target:  10.250.0.10/24
worker:  10.250.0.20/24
```

No default gateway is required on the isolated test interface.

## Important measurement caveat

When the target and generators share one physical host, they also share CPU, memory bandwidth, NUMA resources, and parts of the host networking stack. Results are excellent for finding application bottlenecks and validating orchestration, but they are not equivalent to a clean external capacity benchmark.

For authoritative throughput numbers, place generators on separate physical hardware and document NIC/link capacity.

## Resource isolation

Use CPU and memory limits/reservations so the generator cannot accidentally consume every host resource before the target reaches its own bottleneck. Record those limits with each test result so runs remain comparable.
