# Proxy package instructions

## Responsibility

This package converts validated routes into an HTTP handler tree backed by
`httputil.ReverseProxy`. It owns upstream URL rewriting, per-route balancer
creation, forwarding headers, prefix stripping, and proxy error responses.

## Current construction

- `Handler` wires the default Round Robin implementation.
- `HandlerWithBalancer` accepts a factory for substitution and tests.
- Every route receives a separate balancer instance.
- `http.ServeMux` chooses the most specific registered prefix.
- `ProxyRequest.SetURL` joins the selected upstream base URL with the incoming
  request path.
- `SetXForwarded` writes standardized forwarding information.
- Proxy transport errors are logged and returned as HTTP 502.

Configuration validation normally guarantees valid URLs and non-empty upstream
lists. Constructors still return contextual errors for direct callers.

## Invariants

- Never share a stateful balancer between routes.
- Balancer selection occurs once per proxy attempt.
- Preserve request context, method, query string, body semantics, and Request ID.
- Prefix stripping happens outside the reverse proxy and must not produce an
  unexpected empty path.
- Do not leak internal errors or upstream topology in client responses.
- A nil factory or nil balancer is a construction error.

## Future retry work

Retry belongs near proxy transport/execution, not inside a balancer. Define
safe methods, replayable bodies, attempt limits, per-attempt timeouts, backoff,
and upstream-result reporting before implementation. Do not retry after a
response has been partially written.

Health-aware selection may require evolving the balancer contract. Handle the
no-upstream case explicitly rather than allowing `Next` to return `nil`.

## Tests

Use `httptest.Server` for upstreams. Verify path/query rewriting, prefix
stripping, deterministic balancing, custom factory injection, upstream failure,
headers, and concurrency. Cross-service behaviour belongs in integration tests.

```bash
go test -count=1 ./internal/proxy
go test -count=10 ./test/integration/...
```

Keep package coverage strictly above 75%.
