# API Gateway

A production-minded HTTP API gateway written in Go. The project demonstrates
reverse proxying, prefix routing, concurrent load balancing, middleware,
operational endpoints, graceful shutdown, and real-network integration testing.

## Features

- YAML configuration with environment-variable overrides and strict validation
- HTTP reverse proxy with path-prefix routing and prefix stripping
- Multiple upstream instances with a concurrent Round Robin balancer
- Replaceable load-balancer interface and per-route balancer instances
- Separate public and administrative HTTP servers
- Graceful shutdown with readiness state transitions
- Structured logging with `log/slog`
- Request ID propagation, panic recovery, request logging, timeout, and CORS
- Prometheus-compatible metrics and Go `pprof`
- Unit tests, real-network integration tests, and load-test scenarios
- Docker Compose demo with Users and Billing services
- Parallel GitHub Actions jobs for build, unit tests, and integration tests

## Architecture

```text
                              ┌───────────────┐
                         ┌───►│ Users #1     │
                         │    └───────────────┘
                         │    ┌───────────────┐
Client ──► API Gateway ──┼───►│ Users #2     │
             │           │    └───────────────┘
             │           │    ┌───────────────┐
             │           └───►│ Users #3     │
             │                └───────────────┘
             │                ┌───────────────┐
             └───────────────►│ Billing      │
                              └───────────────┘
```

Requests under `/api/` are distributed between the three Users instances.
The more specific `/api/billing/` prefix is routed to Billing. Prefixes are
removed before forwarding:

```text
/api/users/42                    → Users  /users/42
/api/billing/invoices/inv-1002  → Billing /invoices/inv-1002
```

## Quick start

The complete demo can be started with one command:

```bash
docker compose up --build
```

Try the available endpoints:

```bash
curl http://localhost:8080/api/hello
curl http://localhost:8080/api/users
curl http://localhost:8080/api/users/42
curl http://localhost:8080/api/billing/invoices
curl http://localhost:8080/api/billing/invoices/inv-1002
curl -X POST http://localhost:8080/api/billing/payments
```

Repeat a Users request to see `backend-1`, `backend-2`, and `backend-3`
rotate in the response.

## Configuration

The default local configuration is in
[`configs/gateway.yaml`](configs/gateway.yaml). Docker uses
[`configs/gateway.docker.yaml`](configs/gateway.docker.yaml).

```yaml
routes:
  - path_prefix: /api/billing/
    upstreams:
      - http://localhost:8091
    strip_prefix: true

  - path_prefix: /api/
    upstreams:
      - http://localhost:8081
      - http://localhost:8082
      - http://localhost:8083
    strip_prefix: true
```

Unknown YAML fields, duplicate upstreams, unsupported URL schemes, invalid
durations, duplicate route prefixes, and multiple YAML documents are rejected
during startup.

Environment variables override selected configuration values:

```text
GATEWAY_SERVER_ADDRESS
GATEWAY_ADMIN_ADDRESS
GATEWAY_LOG_LEVEL
GATEWAY_LOG_FORMAT
GATEWAY_SERVER_READ_TIMEOUT
GATEWAY_SERVER_WRITE_TIMEOUT
GATEWAY_SERVER_IDLE_TIMEOUT
GATEWAY_SERVER_SHUTDOWN_TIMEOUT
GATEWAY_MIDDLEWARE_REQUEST_TIMEOUT
```

For example:

```bash
GATEWAY_LOG_LEVEL=debug go run ./cmd/gateway -config configs/gateway.yaml
```

## Operational endpoints

The administrative server listens on `:9090` by default:

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Process liveness |
| `GET /readyz` | Readiness; returns `503` before startup and during shutdown |
| `GET /metrics` | Prometheus-compatible gateway metrics |
| `GET /debug/pprof/` | Go runtime profiler |

The gateway reports ready only after both public and admin listeners have been
opened successfully. It becomes unready before graceful shutdown begins.

## Development

Requirements:

- Go version declared in [`go.mod`](go.mod)
- Docker and Docker Compose for the complete demo
- `make` for the convenience commands below

```bash
make build
make run
make lint
make test
make test-integration
make tidy
```

Direct equivalents:

```bash
go build ./cmd/gateway
go vet ./...
go test ./...
go test -race -count=1 ./test/integration/...
```

## Testing

Unit tests cover every `internal` package. CI enforces coverage strictly greater
than 75% for each package individually, so high coverage in one package cannot
hide missing tests in another.

Integration tests live in [`test/integration`](test/integration) and run the
gateway in-process with real TCP listeners. They verify:

- Round Robin ordering
- routing between Users and Billing
- most-specific prefix selection
- prefix stripping
- generated and preserved Request IDs
- health and readiness endpoints
- unknown-route handling
- graceful shutdown

Shared test infrastructure is isolated in
[`test/integration/internal/testenv`](test/integration/internal/testenv).
It provides configurable upstream groups, dynamic ports, readiness polling,
an HTTP client, automatic cleanup, and scenario-focused response assertions.
See the integration [testing guide](test/integration/README.md) when adding a
new scenario.

## Load testing

Ready-to-run `hey` and `wrk` commands are documented in
[`test/load/README.md`](test/load/README.md). A mixed Users/Billing workload is
provided in [`test/load/mixed.lua`](test/load/mixed.lua).

Example:

```bash
wrk -t4 -c100 -d60s --latency -s test/load/mixed.lua \
  http://localhost:8080
```

Load tests are intentionally excluded from regular CI because shared runners
do not provide stable performance measurements.

## Continuous integration

GitHub Actions runs three independent jobs in parallel:

| Job | Checks |
|---|---|
| `build` | Builds Gateway, Users, and Billing binaries |
| `unit-tests` | Runs `go vet`, race-enabled unit tests, and per-package coverage gates |
| `integration-tests` | Runs the real-network integration suite with the race detector |

## AI agent harness

Repository-level instructions for coding agents are defined in
[`AGENTS.md`](AGENTS.md). More specific rules apply under
[`internal`](internal/AGENTS.md) and
[`test/integration`](test/integration/AGENTS.md).

Agents and contributors can reproduce the main local quality gate with:

```powershell
.\scripts\verify.ps1
```

or:

```bash
sh scripts/verify.sh
```

Race detection is optional locally because it requires a working C toolchain:

```powershell
.\scripts\verify.ps1 -Race
```

```bash
RACE=1 sh scripts/verify.sh
```

Run the project-specific, read-only review skill before opening a PR:

```text
Use $review-api-gateway to review my current changes.
```

The skill reads all applicable `AGENTS.md` files, reviews architecture and Go
semantics, runs relevant non-mutating checks, and reports prioritized findings.
It is stored in [`.codex/skills/review-api-gateway`](.codex/skills/review-api-gateway).

## Project layout

```text
.
├── cmd/gateway/                    gateway entry point
├── configs/                        local and Docker YAML configuration
├── deployments/                    backend container definitions
├── examples/
│   ├── backend/                    demo Users service
│   └── billing/                    demo Billing service
├── internal/
│   ├── balancer/                   balancer contract and Round Robin
│   ├── config/                     loading and strict validation
│   ├── logger/                     structured logger construction
│   ├── metrics/                    Prometheus-compatible metrics
│   ├── middleware/                 HTTP middleware chain
│   ├── proxy/                      route-aware reverse proxies
│   └── server/                     listeners, readiness, and lifecycle
├── test/
│   ├── integration/
│   │   └── internal/testenv/       shared integration harness
│   └── load/                       hey and wrk scenarios
├── scripts/                         cross-platform verification harness
├── AGENTS.md                        repository instructions for AI agents
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Roadmap

- Retries with safe-method and replayability rules
- Circuit breaker and passive upstream health tracking
- Rate limiting
- JWT authentication and response caching
- Configuration reload and OpenTelemetry
- gRPC proxying and service discovery
