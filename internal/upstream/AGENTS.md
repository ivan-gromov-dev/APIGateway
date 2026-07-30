# Upstream package instructions

## Responsibility

This package owns the runtime model of one configured upstream instance. Each
Target retains an immutable URL copy and an independent circuit breaker.

## Invariants

- Targets are safe for concurrent use.
- Caller-owned URLs are copied and mutable state is not exposed.
- Health state belongs to an instance, never to an entire route.
- This package does not classify HTTP responses or perform network I/O.
- Passive result classification belongs to proxy execution.
- Active health checks may update target health through explicit APIs later.

Use explicit timestamps in tests and keep statement coverage strictly above 75%.
