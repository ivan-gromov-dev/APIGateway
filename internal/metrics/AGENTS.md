# Metrics package instructions

## Responsibility

This package records process-local gateway request metrics and exposes them in
Prometheus text format through `http.Handler`.

## Current metrics

- `gateway_http_requests_total`: all observed public requests.
- `gateway_http_errors_total`: responses with status 500 or greater.
- `gateway_http_request_duration_seconds_sum`: cumulative request duration.

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

The current collector is intentionally minimal. Before adding histograms,
labels, registries, or OpenTelemetry, decide whether to adopt the official
Prometheus client rather than extending the handwritten exposition indefinitely.

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
