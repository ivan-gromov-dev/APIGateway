# API Gateway

A small, production-minded API gateway written in Go. The first milestone focuses
on a dependable HTTP reverse proxy with explicit configuration, middleware,
observability endpoints, and graceful shutdown.

## Quick start

```bash
go run ./cmd/gateway -config configs/gateway.yaml
```

The example configuration forwards `/api/` to a backend listening on
`http://localhost:8081`. Operational endpoints are exposed separately:

- `GET http://localhost:9090/healthz` — process liveness
- `GET http://localhost:9090/readyz` — gateway readiness
- `GET http://localhost:9090/metrics` — Prometheus metrics
- `GET http://localhost:9090/debug/pprof/` — Go profiler

Run the complete local demo with:

```bash
docker compose up --build
curl http://localhost:8080/api/hello
```

## Configuration

Environment variables override YAML values. Nested keys use `GATEWAY_` and
underscores, for example `GATEWAY_SERVER_ADDRESS=:8000`. See
[`configs/gateway.yaml`](configs/gateway.yaml) for all current options.

## Development

```bash
make test
make lint
make build
```

## Layout

```text
cmd/gateway/          application entry point
internal/config/      YAML loading and validation
internal/middleware/  request ID, recovery, logging, timeout, CORS
internal/proxy/       reverse proxy construction
internal/server/      HTTP servers and lifecycle
configs/              runtime configuration
deployments/          container assets
examples/             demo backend
```

## Roadmap

- Load balancing, retries, circuit breaking, and rate limiting
- JWT authentication and response caching
- Configuration reload and OpenTelemetry
- gRPC proxying, service discovery, and progressive delivery
