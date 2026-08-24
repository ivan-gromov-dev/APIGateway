# Changelog

All notable changes to this project are documented here. Releases follow
[Semantic Versioning](https://semver.org/), and the entries below were rebuilt
from the annotated Git tags and their commit ranges.

## [1.1.0] - 2026-08-09

### Added

- continuous per-route DNS refresh using the existing `interval` and `grace`
  configuration contract;
- bounded discovery refresh, target-count, and staleness metrics;
- readiness and route-level `503` behaviour after a discovered snapshot expires.

### Changed

- DNS answers are deduplicated and sorted before immutable publication;
- unchanged upstream targets retain circuit-breaker and active-health state
  across membership updates;
- configuration reload and shutdown retire refresh workers and active probes.

## [1.0.0] - 2026-08-02

First stable release. The configuration and runtime contracts documented in
the README are now the supported 1.x baseline.

### Added

- weighted round-robin balancing and percentage, header, and stable-hash
  rollouts with bounded observability labels;
- DNS-backed upstream discovery during transactional snapshot construction;
- transparent opt-in gRPC proxying over HTTP/2, including metadata, deadlines,
  cancellation, streaming, and trailers;
- a Docker gRPC health backend and cross-feature smoke runners for the default
  Compose stack;
- scoped contributor instructions for every internal responsibility and the
  configuration, deployment, example, script, and smoke-test areas.

### Changed

- local and Docker configurations now demonstrate the gRPC route alongside the
  existing HTTP, resilience, security, storage, discovery, and observability
  features;
- release documentation and roadmap now match the implemented feature set.

## [0.6.0] - 2026-08-02

### Added

- active HTTP health checks with configurable intervals, timeouts, and healthy
  and unhealthy thresholds;
- transactional file-based configuration reload that preserves the last valid
  runtime snapshot.

### Changed

- expanded CI and package coverage for health-check and reload behaviour.

## [0.5.0] - 2026-07-31

### Added

- Prometheus RED and process metrics;
- the optional Grafana, Prometheus, Elasticsearch, Kibana, Filebeat, Elastic
  APM, and OpenTelemetry Collector demo stack;
- provisioned dashboards and operational documentation.

## [0.4.0] - 2026-07-31

### Added

- route-level JWT authentication backed by external JWKS providers;
- Redis-backed response caching with authorization and privacy safeguards;
- OpenTelemetry tracing and propagation;
- stricter repository instructions and coverage around security boundaries.

## [0.3.0] - 2026-07-30

### Added

- per-upstream circuit breakers and passive health tracking;
- global and per-route Redis-backed token-bucket rate limiting;
- explicit fail-open and fail-closed backend policies.

## [0.2.0] - 2026-07-30

### Added

- bounded retries for safe, replayable HTTP requests;
- per-attempt timeouts, backoff, and retryable-status configuration;
- load-test scripts and AI-assisted development instructions.

## [0.1.0] - 2026-07-29

### Added

- initial API gateway with strict YAML configuration;
- longest-prefix routing, prefix stripping, round-robin upstream balancing,
  forwarding headers, request IDs, middleware, and graceful shutdown;
- separate public and admin listeners, Docker Compose demos, and unit and
  real-network integration tests.

[1.0.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v0.6.0...v1.0.0
[1.1.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v1.0.0...v1.1.0
[0.6.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/ivan-gromov-dev/APIGateway/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/ivan-gromov-dev/APIGateway/releases/tag/v0.1.0
