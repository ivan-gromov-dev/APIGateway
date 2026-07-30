# Circuit breaker package instructions

## Responsibility

This package owns an HTTP-independent, concurrency-safe circuit breaker state
machine. HTTP result classification belongs to passive upstream health tracking.

## Invariants

- A breaker moves through closed, open, and half-open states.
- Open circuits admit exactly one half-open probe after the timeout.
- Every admitted attempt receives an idempotent completion callback.
- Results from older state generations must not mutate the current state.
- Callers provide timestamps; the package does not read or wait on wall-clock time.
- All exported Breaker methods are safe for concurrent use.

## Dependencies and tests

Keep this package independent of `net/http`, proxy, balancing, configuration,
logging, and metrics. Tests must use explicit timestamps rather than sleeps and
must cover every transition, stale results, callback idempotency, and concurrent
half-open acquisition. Keep statement coverage strictly above 75%.
