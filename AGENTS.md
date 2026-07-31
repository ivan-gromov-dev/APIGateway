# Agent instructions

## Mission

This repository is a production-minded Go API gateway and a portfolio project.
Prefer explicit, testable infrastructure code over framework-like abstraction.
Keep the gateway runnable through Docker Compose and understandable to a backend
engineer reviewing the repository.

## Start here

Before changing code:

1. Read `README.md` for the current feature set and architecture.
2. Inspect `git status --short --branch`; preserve unrelated user changes.
3. Read this file and every nested `AGENTS.md` on the path to the files being
   changed. Nested instructions add package-specific constraints.
4. Inspect existing tests and configuration before introducing a new pattern.

## Instruction hierarchy

- This root file applies to the entire repository.
- `internal/AGENTS.md` adds shared rules for all production packages.
- Each `internal/<package>/AGENTS.md` documents that package's API, invariants,
  dependencies, extension points, and required tests.
- `test/integration/AGENTS.md` applies to integration scenarios and their shared
  harness.

Keep cross-repository policy here. Keep production-package policy in
`internal/AGENTS.md`, and keep implementation details in the closest package
file. Update instructions in the same change when an architectural contract
changes.

## Architecture

- `cmd/gateway` owns process startup, signals, and dependency wiring.
- `internal/config` owns strict YAML loading, defaults, environment overrides,
  and semantic validation.
- `internal/balancer` owns load-balancing contracts and algorithms.
- `internal/circuitbreaker` owns the HTTP-independent circuit state machine.
- `internal/upstream` owns per-instance URLs and health state.
- `internal/limiter` owns Token Bucket policy wiring.
- `internal/ratestore/redis` owns atomic Redis rate-limit persistence.
- `internal/ratelimit` contains store, key, filter, and registry contracts.
- `internal/auth` owns JWT and JWKS verification.
- `internal/cache` owns response-cache contracts.
- `internal/cachestore/redis` owns Redis response-cache persistence.
- `internal/proxy` owns route-specific reverse proxy construction.
- `internal/middleware` owns composable HTTP cross-cutting behaviour.
- `internal/metrics` and `internal/logger` own observability primitives.
- `internal/server` owns public/admin listeners, readiness, and shutdown.
- `internal/telemetry` owns optional OpenTelemetry tracing, OTLP export, and propagation.
- `examples` contains demo upstream and identity services, not gateway business logic.
- `test/integration` contains real-network behavioral scenarios.
- `test/load` contains manual performance scenarios and local result files.
- `scripts` contains the cross-platform verification harness.

The public server handles proxied traffic. Health, readiness, metrics, and
`pprof` belong on the admin server.

## Engineering rules

- Follow standard Go formatting and naming.
- Pass `context.Context` through request and lifecycle boundaries.
- Keep concurrency race-free and make ownership/lifecycle explicit.
- Prefer small consumer-defined interfaces when implementations genuinely need
  substitution. Do not create interfaces for every concrete type.
- Constructors should normally return concrete types.
- Reject invalid configuration during startup instead of silently correcting it.
- Do not add retry behaviour for unsafe or non-replayable requests without an
  explicit policy and tests.
- Preserve request cancellation, forwarding headers, Request IDs, and graceful
  shutdown semantics.
- Avoid new dependencies when the standard library is sufficient. Explain any
  dependency added to `go.mod`.
- Never reduce or bypass CI coverage thresholds to make a change pass.

## Tests

Add tests at the lowest useful level:

- Unit tests for isolated algorithms, parsing, and middleware.
- Integration tests for behaviour spanning routing, proxying, listeners, or
  lifecycle.
- Load scenarios only for performance experiments; do not put performance
  thresholds on shared CI runners.

Every `internal` package must retain statement coverage strictly above 75%.
Use the shared integration harness instead of duplicating server/upstream setup.

## Validation

Windows PowerShell:

```powershell
.\scripts\verify.ps1
```

Linux, macOS, WSL, or Git Bash:

```bash
sh scripts/verify.sh
```

The full validation checks formatting, `go vet`, unit/integration tests, and
builds Gateway, Users, Billing, and demo Identity. Use `-Race` or `RACE=1` when the local
platform has a working C toolchain.

Local verification does not replace CI's per-package coverage gate. CI runs
three independent jobs: builds, race-enabled unit tests with coverage strictly
above 75% for every `internal` package, and race-enabled integration tests.

For a read-only architectural self-review before commit or PR, invoke the
project skill:

```text
Use $review-api-gateway to review my current changes.
```

The review skill reports findings but does not modify, stage, or commit files.

For a repository-aligned plan or implementation of a new capability, invoke:

```text
Use $implement-api-gateway-feature to plan and implement <feature>.
```

The implementation skill respects plan-only requests, loads the complete
instruction hierarchy, and carries authorized changes through configuration,
runtime wiring, tests, documentation, and validation. Use the review skill as a
separate final check.

## Definition of done

A change is complete when:

- behaviour and failure semantics are explicit;
- relevant unit or integration tests exist;
- strict configuration and docs are updated when public configuration changes;
- `gofmt`, `go vet`, tests, and builds pass;
- no generated binaries, coverage profiles, or load-result files are included
  unintentionally;
- the final summary names important files, validation performed, and any local
  limitation.

## Git and scope

- Keep commits focused and use conventional prefixes such as `feat:`, `fix:`,
  `test:`, `ci:`, and `docs:`.
- Do not commit, push, tag, or rewrite history unless explicitly requested.
- Do not modify `.vscode/launch.json`; it is intentionally local and ignored.
- Do not edit files under `.cache`.
