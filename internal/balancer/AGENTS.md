# Balancer package instructions

## Responsibility

This package defines upstream-selection contracts and load-balancing algorithms.
It must remain independent of HTTP handlers, configuration files, middleware,
metrics exporters, and demo services.

## Current API

- `Balancer.Next()` returns the upstream selected for the next request.
- `Factory` creates one independent `Balancer` for a route.
- `RoundRobin` stores an immutable upstream slice and an atomic sequence number.
- `NewRoundRobin` rejects an empty upstream list and returns a concrete type.

`RoundRobin.Next` is safe for concurrent use. Selection begins at index zero,
increments once per call, and wraps with modulo arithmetic.

## Invariants

- A constructed balancer always contains at least one upstream.
- Do not mutate the caller's slice or allow later caller mutations to change a
  balancer. Constructors must copy input slices.
- Upstream URLs are treated as immutable after configuration validation.
- `Next` must not block or perform network I/O.
- Algorithms used by the proxy must be safe under concurrent requests.
- Do not add HTTP response handling or retry loops to this package.

## Adding an algorithm

Add a separate implementation file and constructor, for example
`weighted_round_robin.go` or `random.go`. Return a concrete type and assert the
contract at compile time:

```go
var _ Balancer = (*WeightedRoundRobin)(nil)
```

Document algorithm-specific fairness, state, overflow behaviour, complexity,
and concurrency guarantees. If an algorithm needs weights or health state,
prefer an explicit constructor input type rather than parallel slices.

The current `Next() *url.URL` contract assumes that selection always succeeds.
Before implementing health-aware balancing, explicitly design the
no-available-upstream result; do not return `nil` and let the proxy panic.

## Tests

Test constructor validation, deterministic ordering where applicable, wraparound,
input copying, and concurrent selection. Run:

```bash
go test -count=10 ./internal/balancer
```

Keep package coverage strictly above 75%.
