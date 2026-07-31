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
- OpenTelemetry traces with W3C context propagation and OTLP export
- Unit tests, real-network integration tests, and load-test scenarios
- Retries across upstreams for safe, replayable requests
- Passive upstream health tracking with per-instance circuit breakers
- Redis-backed global and per-route Token Bucket rate limiting
- Route-level JWT authentication using asymmetric signatures and JWKS
- Redis-backed response caching for explicitly enabled public routes
- Docker Compose demo with Users, Billing, Redis, and a demo identity service
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
curl http://localhost:8080/api/billing/invoices
curl http://localhost:8080/api/billing/invoices/inv-1002
curl -X POST http://localhost:8080/api/billing/payments
```

The Users route is protected. In PowerShell, obtain a short-lived demo token
and call it with:

```powershell
$token = (Invoke-RestMethod -Method Post `
  -Uri http://localhost:8084/token `
  -Authentication Basic `
  -Credential ([pscredential]::new('gateway-demo', (ConvertTo-SecureString 'gateway-demo-secret' -AsPlainText -Force))) `
  -Body @{ grant_type = 'client_credentials'; scope = 'users.read' }).access_token

Invoke-RestMethod http://localhost:8080/api/users `
  -Headers @{ Authorization = "Bearer $token" }
```

The identity service is deliberately demo-only: it has one client, keeps its
ephemeral signing key in memory, supports only `client_credentials`, and does
not implement users, refresh tokens, registration, or persistent key rotation.

Repeat a Users request to see `backend-1`, `backend-2`, and `backend-3`
rotate in the response.

## Configuration

The default local configuration is in
[`configs/gateway.yaml`](configs/gateway.yaml). Docker uses
[`configs/gateway.docker.yaml`](configs/gateway.docker.yaml).

```yaml
retry:
  max_attempts: 3
  per_attempt_timeout: 2s
  backoff: 25ms
  statuses: [502, 503, 504]

circuit_breaker:
  failure_threshold: 5
  open_timeout: 30s
  failure_statuses: [502, 503, 504]

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

`max_attempts` counts the initial request and is limited to 10. Retries apply
only to `GET`, `HEAD`, and `OPTIONS`, and only when a request body is absent or
can be replayed through `GetBody`. Unsafe methods such as `POST`, `PUT`,
`PATCH`, and `DELETE` are sent once. Each attempt selects the next upstream;
network errors and configured 5xx statuses are retryable within the request
context and per-attempt timeout.

Each upstream instance owns an independent circuit breaker. Configured failure
statuses and transport failures are recorded per proxy attempt; open upstreams
are skipped. If no upstream is available, the gateway returns `503 Service
Unavailable`. Client cancellation is neutral and does not penalize upstream
health.

Rate limiting uses an atomic Redis script and is enabled in the Docker demo.
Global rules protect gateway capacity; route rules protect individual services.
Built-in keys are `global` and `client_ip`, and filters are `all` and
`writes_only`. Exceeding a rule returns `429` with `Retry-After`. The gateway
talks through the Redis protocol, so the backend may be Redis itself or another
Redis-compatible server.

JWT authentication is opt-in per route. The gateway validates an explicit
`RS256` algorithm allowlist, JWKS key ID, signature, issuer, audience,
expiration, not-before, and required scopes. Tokens are issued by an external
identity provider; the gateway never issues or logs them. Verified subjects are
forwarded in `X-Authenticated-Subject` after any client-supplied value is
removed.

### Keycloak provider demo

An optional Keycloak setup demonstrates the same gateway contract against a
real OIDC provider. Start it from the repository root:

```powershell
docker compose -f docker-compose.yml -f deployments/keycloak/docker-compose.yml up --build
```

Request a service-account token and call the protected route:

```powershell
$token = (Invoke-RestMethod -Method Post `
  -Uri http://localhost:8085/realms/gateway-demo/protocol/openid-connect/token `
  -Body @{ client_id = 'gateway-demo'; client_secret = 'gateway-demo-secret'; grant_type = 'client_credentials' }).access_token

Invoke-RestMethod http://localhost:8080/api/users `
  -Headers @{ Authorization = "Bearer $token" }
```

Keycloak runs in development mode and imports
`deployments/keycloak/gateway-demo-realm.json`; its credentials are strictly
for local demonstration. The lightweight identity service remains available on
port `8084`, but the gateway uses Keycloak while the override is active.

Response caching is also opt-in per route and uses Redis with bounded operation
timeouts and response sizes. Only public `GET` and `HEAD` requests are eligible.
Requests containing `Authorization`, and responses containing `Set-Cookie`,
`Cache-Control: private`, or `Cache-Control: no-store`, bypass storage. Cache
failures allow upstream traffic by default. The Docker Billing route
demonstrates a 30-second TTL and returns `X-Cache: MISS` or `X-Cache: HIT`.

### Distributed tracing

Tracing is optional and disabled by default. When enabled, the gateway creates
an HTTP server span, a span for every proxy attempt, and an HTTP client span for
upstream and JWKS requests. The outgoing `traceparent` header lets an
instrumented backend continue the same trace. Structured request logs include
`trace_id` and `span_id`; Prometheus metrics and `pprof` remain independent.

The gateway exports OTLP/HTTP to an OpenTelemetry Collector rather than
directly to a vendor backend. Start the demo Collector and Tempo backend with:

```powershell
docker compose -f docker-compose.yml -f deployments/observability/docker-compose.yml up --build
```

Tempo's API is exposed at `http://localhost:3200`. A visualization layer such
as Grafana can be connected later without changing gateway instrumentation.

```yaml
telemetry:
  tracing:
    enabled: false
    service_name: api-gateway
    endpoint: localhost:4318
    insecure: true
    sample_ratio: 1
    shutdown_timeout: 5s
```

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
GATEWAY_TELEMETRY_TRACING_ENABLED
GATEWAY_TELEMETRY_TRACING_SERVICE_NAME
GATEWAY_TELEMETRY_TRACING_ENDPOINT
GATEWAY_TELEMETRY_TRACING_INSECURE
GATEWAY_TELEMETRY_TRACING_SAMPLE_RATIO
GATEWAY_TELEMETRY_TRACING_SHUTDOWN_TIMEOUT
```

For example:

```bash
GATEWAY_LOG_LEVEL=debug go run ./cmd/gateway -config configs/gateway.yaml
```

## Operational endpoints

The administrative server listens on `:9090` by default:

| Endpoint            | Purpose                                                     |
| ------------------- | ----------------------------------------------------------- |
| `GET /healthz`      | Process liveness                                            |
| `GET /readyz`       | Readiness; returns `503` before startup and during shutdown |
| `GET /metrics`      | Prometheus-compatible gateway metrics                       |
| `GET /debug/pprof/` | Go runtime profiler                                         |

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

| Job                 | Checks                                                                 |
| ------------------- | ---------------------------------------------------------------------- |
| `build`             | Builds Gateway, Users, Billing, and demo Identity binaries             |
| `unit-tests`        | Runs `go vet`, race-enabled unit tests, and per-package coverage gates |
| `integration-tests` | Runs the real-network integration suite with the race detector         |

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

For new features, start with the implementation workflow:

```text
Use $implement-api-gateway-feature to plan and implement <feature>.
```

It supports plan-only and implementation requests, establishes package and
configuration boundaries, adds the appropriate tests and documentation, and
runs the repository validation gate. Follow it with `$review-api-gateway` before
commit or PR. The skill is stored in
[`.codex/skills/implement-api-gateway-feature`](.codex/skills/implement-api-gateway-feature).

## Project layout

```text
.
├── cmd/gateway/                    gateway entry point
├── configs/                        local and Docker YAML configuration
├── deployments/                    backend container definitions
├── examples/
│   ├── backend/                    demo Users service
│   ├── billing/                    demo Billing service
│   └── identity/                   demo token issuer and JWKS endpoint
├── internal/
│   ├── balancer/                   balancer contract and Round Robin
│   ├── auth/                       JWT and JWKS verification
│   ├── cache/                      response-cache contract
│   ├── cachestore/redis/           Redis response-cache persistence
│   ├── circuitbreaker/              circuit breaker state machine
│   ├── config/                     loading and strict validation
│   ├── logger/                     structured logger construction
│   ├── metrics/                    Prometheus-compatible metrics
│   ├── middleware/                 HTTP middleware chain
│   ├── proxy/                      route-aware reverse proxies
│   ├── server/                     listeners, readiness, and lifecycle
│   ├── telemetry/                  OpenTelemetry tracing and propagation
│   └── upstream/                   upstream instance and health state
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

- gRPC proxying and service discovery
