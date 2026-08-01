# Repository instructions

Production-minded Go API gateway and portfolio project. Prefer explicit,
testable infrastructure code over framework abstractions; keep Docker Compose
runnable and the design easy for backend engineers to inspect.

## Before edits

1. Read `README.md`, `git status --short --branch`, and existing tests/config.
2. Read every `AGENTS.md` from the root to each changed file; the closest file
   supplies package-specific contracts.
3. Preserve unrelated changes. Never edit `.cache` or `.vscode/launch.json`.

## Boundaries

- `cmd/gateway`: startup, signals, wiring.
- `internal/config`: strict YAML, defaults, env overrides, validation.
- `internal/server`: public/admin listeners, readiness, shutdown, composition.
- Other `internal/*` packages own the responsibility named by their directory;
  their scoped instructions are authoritative.
- `examples`: demo services only. `test/integration`: real-network scenarios.
- Public traffic uses the public listener; health, readiness, metrics, and
  `pprof` stay on admin.

Keep dependencies acyclic and lifecycle wiring out of lower-level packages.
Pass contexts through request/lifecycle boundaries. Make concurrency ownership
explicit and race-free. Prefer concrete constructors and small consumer-defined
interfaces only where substitution is real. Reject invalid configuration at
startup. Preserve cancellation, forwarding headers, Request IDs, and graceful
shutdown. Retry unsafe or non-replayable requests only under an explicit tested
policy. Prefer the standard library and justify new dependencies.

## Tests and completion

Test at the lowest useful level: unit tests for isolated behaviour, integration
tests for cross-package/listener behaviour via the shared harness, and load
tests only for manual performance work. Every `internal` package must retain
statement coverage strictly above 75%; never weaken CI gates.

For a completed change run `./scripts/verify.ps1` on Windows or
`sh scripts/verify.sh` elsewhere (race mode when supported). Update config
examples, README, and applicable scoped instructions when contracts change.
Do not leave generated binaries, coverage profiles, or load results.

Keep commits focused and conventional. Do not commit, push, tag, or rewrite
history unless explicitly requested. Final handoff names key files, validation,
and limitations.

Use `$implement-api-gateway-feature` for repository-wide feature work and
`$review-api-gateway` for a separate read-only pre-commit/PR review.
