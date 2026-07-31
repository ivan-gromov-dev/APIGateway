# Metrics package instructions

## Responsibility

This package records process-local gateway request metrics and exposes them in
Prometheus text format through `http.Handler`.

## Current metrics

- HTTP request count, latency histogram, response-size histogram, and in-flight gauge;
- route/status request counts;
- proxy attempts, attempt latency, retries, upstream outcomes, and exhaustion;
- bounded auth, cache, and rate-limit decisions;
- standard Go runtime and process collectors.

`Collector.Observe` is called by logging middleware. `Collector.ServeHTTP` is
mounted only on the admin server at `/metrics`.

## Concurrency and correctness

- Observation is on the request hot path and must remain non-blocking.
- All mutable values must be safe under concurrent requests.
- Durations are stored as nanoseconds and exported as seconds.
- Preserve valid Prometheus `HELP` and `TYPE` declarations.
- Do not expose metrics on the public proxy server.
- Avoid high-cardinality labels such as raw URL, Request ID, user ID, or full
  upstream address.

The collector owns a private official Prometheus registry. Do not use the
process-global default registry or register application metrics outside this package.

## Adding metrics

Define metric name, type, unit, cardinality, and update point before coding.
Counters must be monotonic. If per-route metrics are added, use the configured
route pattern rather than the incoming raw path.

## Tests

Verify exposition names, values, content type, error classification, duration
units, and concurrent observations. Run:

```bash
go test -count=10 ./internal/metrics
```

Keep coverage strictly above 75%.
