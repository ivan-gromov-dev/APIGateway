# Proxy package instructions

## Responsibility

This package converts validated routes into an HTTP handler tree backed by
`httputil.ReverseProxy`. It owns upstream URL rewriting, per-route balancer
creation, forwarding headers, prefix stripping, and proxy error responses.

## Current construction

- `Handler` wires the default Round Robin implementation.
- `HandlerWithBalancer` accepts a factory for substitution and tests.
- Every route receives a separate balancer instance.
- Route authentication, rate limiting, and response caching wrap the route
  proxy in that order; cache hits cannot bypass authentication or limiting.
- `http.ServeMux` chooses the most specific registered prefix.
- The retry transport selects a health-eligible target for every attempt and
  joins its base URL with the incoming request path.
- When tracing is supplied, every proxy attempt owns an internal span and the
  instrumented base transport creates the child HTTP client span.
- `SetXForwarded` writes standardized forwarding information.
- Proxy transport errors are logged and returned as HTTP 502. Exhaustion of
  available upstreams returns HTTP 503.
- Optional metrics record configured routes/upstreams, attempts, outcomes,
  latency, and retry reasons without using incoming paths as labels.

Configuration validation normally guarantees valid URLs and non-empty upstream
lists. Constructors still return contextual errors for direct callers.

## Invariants

- Never share a stateful balancer between routes.
- Balancer selection occurs once per proxy attempt.
- Preserve request context, method, query string, body semantics, and Request ID.
- Preserve W3C trace context across retries and end attempt spans when their
  response bodies complete or close.
- Prefix stripping happens outside the reverse proxy and must not produce an
  unexpected empty path.
- Do not leak internal errors or upstream topology in client responses.
- A nil factory or nil balancer is a construction error.

## Passive health tracking

Classify every attempt independently from retry policy. Configured failure
statuses and upstream transport errors are failures, client cancellation is
neutral, and other received responses are successful. Complete the selected
target callback before retrying or returning.

## Tests

Use `httptest.Server` for upstreams. Verify path/query rewriting, prefix
stripping, deterministic balancing, custom factory injection, upstream failure,
headers, and concurrency. Cross-service behaviour belongs in integration tests.

```bash
go test -count=1 ./internal/proxy
go test -count=10 ./test/integration/...
```

Keep package coverage strictly above 75%.
