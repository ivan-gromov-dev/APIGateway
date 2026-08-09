# Server package instructions

This package composes dependencies and owns both HTTP server lifecycles. Open
public then admin listeners before readiness; if either startup/serve path
fails, close or shut down the other. Clear readiness before concurrent bounded
shutdown, preserve active requests, join relevant errors, then flush owned
tracing with a fresh context. Cancellation is a normal trigger; `Run` is
single-use. Injected tracing runtimes remain caller-owned.

Public serves proxied traffic only. Admin owns `/healthz`, `/readyz`, `/metrics`,
and `/debug/pprof/*`. Middleware order is observable; keep request ID/logging
outside recovery so panics remain correlated and counted. Test listener failures,
readiness, admin isolation, serve failure, active-request drain, and deadlines.

Continuous discovery workers are runtime-snapshot resources. Publish immutable
handlers, retain target state only within the same route and URL, cancel retired
refresh/probe contexts, and withdraw readiness when a route exceeds its stale
grace period.
