# Server package instructions

## Responsibility

This package assembles handlers and owns the complete lifecycle of the public
and administrative HTTP servers.

## Startup sequence

`Server.Run` currently:

1. creates metrics and the route-aware proxy;
2. builds the middleware chain;
3. constructs public and admin `http.Server` values;
4. opens the public listener;
5. opens the admin listener;
6. marks readiness true;
7. serves both listeners concurrently;
8. waits for context cancellation or a server failure;
9. marks readiness false;
10. gracefully shuts both servers down in parallel;
11. joins runtime and shutdown errors.

Do not mark ready before every required listener is open. If admin listener
creation fails, close the already-open public listener.

## Public and admin surfaces

The public server contains only proxied routes plus middleware. The admin server
owns:

- `/healthz`: process liveness;
- `/readyz`: current readiness;
- `/metrics`: Prometheus exposition;
- `/debug/pprof/*`: runtime profiling.

Keep operational endpoints off the public listener unless an explicit product
requirement changes this boundary.

## Lifecycle invariants

- Context cancellation is a normal shutdown trigger, not an application error.
- Unexpected failure of either server must initiate shutdown of the other.
- Readiness becomes false before listeners begin shutting down.
- Shutdown uses a fresh bounded context, not the already-cancelled run context.
- Public and admin shutdown occur concurrently.
- Active requests receive the configured grace period.
- Preserve all relevant errors with contextual wrapping and `errors.Join`.
- `Run` is designed for one lifecycle; do not call it concurrently on one
  `Server`.

## Middleware chain

Changing middleware order changes observable behaviour. Update middleware and
integration tests when modifying the chain. Ensure request logging and metrics
observe intended timeout, recovery, and CORS responses.

## Tests

Cover listener failures independently, readiness states, admin endpoints,
unexpected serve errors where injectable, graceful completion of active
requests, and shutdown deadlines.

```bash
go test -count=10 ./internal/server
go test -count=10 ./test/integration/...
```

Keep package coverage strictly above 75%.
