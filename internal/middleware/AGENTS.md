# Middleware package instructions

## Responsibility

This package contains composable HTTP cross-cutting behaviour. Keep one concern
per production source file and keep middleware independent of route-specific
business logic.

## Current components

- `Chain` applies middleware in declaration order.
- `RequestID` preserves `X-Request-ID` or generates a 128-bit random hex ID.
- `Recovery` logs panics with a stack and returns HTTP 500.
- `Logging` records status, bytes, duration, method, path, and Request ID, then
  updates the metrics collector.
- `Timeout` uses `http.TimeoutHandler` when duration is positive.
- `CORS` handles allowed origins and OPTIONS preflight responses.
- `Authentication` verifies Bearer tokens and installs a trusted principal.
- `ResponseCache` caches bounded, public GET/HEAD responses through a Store.

The order configured in `server.Run` is observable. The first declared
middleware is the outermost wrapper.

## HTTP invariants

- Request IDs must be present on both the forwarded request and response.
- Do not trust client-controlled values for security decisions.
- A response wrapper must record only the first `WriteHeader` call and treat a
  direct `Write` as HTTP 200.
- Preserve optional `http.ResponseWriter` capabilities through `Unwrap` so
  `http.ResponseController` and reverse-proxy streaming continue to work.
- Recovery must not expose panic details to clients.
- Timeout and recovery code must not write a second response after headers are
  committed.
- CORS must emit `Vary: Origin` when the response depends on Origin.

## Adding middleware

Create a focused file and constructor. State whether it short-circuits the
chain, mutates request context/headers, or writes a response. Add it to the
server chain only after deciding its position relative to recovery, logging,
timeout, and CORS.

Avoid package-global mutable state. Prefer request context for request-scoped
values.

## Tests

Test pass-through and short-circuit paths, headers, status, body, panic/error
paths, and chain ordering. Concurrency-sensitive middleware needs race-enabled
tests. Run:

```bash
go test -count=1 ./internal/middleware
```

Keep coverage strictly above 75%.
