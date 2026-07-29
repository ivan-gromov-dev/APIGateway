---
name: review-api-gateway
description: Review API Gateway code, configuration, tests, CI, documentation, and infrastructure changes against the repository's AGENTS.md hierarchy, Go conventions, architecture, concurrency guarantees, HTTP semantics, lifecycle rules, and coverage policy. Use when asked to review a diff, staged changes, commit, branch, pull request, or implementation before commit/merge, or to perform a read-only self-review of current work.
---

# Review API Gateway

Perform an evidence-based, read-only code review. Find defects and meaningful
regressions; do not implement fixes unless the user separately asks for them.

## Establish scope

Use the scope explicitly supplied by the user. Otherwise:

1. inspect `git status --short --branch`;
2. review staged changes when the index is non-empty;
3. also review unstaged changes when present and clearly label the scope;
4. if the worktree is clean and no commit/range is specified, ask which commit,
   branch, or range to review.

Use read-only Git commands such as:

```bash
git diff --check
git diff --stat
git diff
git diff --cached
git show --stat --oneline <commit>
git diff <base>...<head>
```

Do not fetch, checkout, reset, stage, commit, or modify files during review.

## Load project rules

Read the root `AGENTS.md`. For every changed file, read each nested `AGENTS.md`
from the repository root down to that file. Treat the closest file as the
module-specific contract and report conflicts with it.

Read enough surrounding source and tests to understand behaviour beyond the
changed lines. Check current configuration examples and README when a public
contract changes.

## Review workflow

1. Classify changed files by package and responsibility.
2. Identify intended behaviour from the diff, tests, and user request.
3. Trace each changed execution path, including errors and shutdown paths.
4. Check cross-package effects and configuration compatibility.
5. Run the narrowest relevant non-mutating validation.
6. Report only actionable findings supported by code or test evidence.

Prefer focused commands:

```bash
go test -count=1 ./internal/<package>
go test -count=10 ./test/integration/...
go vet ./...
```

Use `.\scripts\verify.ps1` or `sh scripts/verify.sh` for a full local gate when
the change is broad. Enable race detection only when the platform supports it.
Never lower coverage, skip failing checks, or rewrite files to make review pass.

## Review lenses

### Correctness and HTTP

- Verify route precedence, prefix stripping, URL/query joining, forwarding
  headers, Request IDs, body replayability, and response status/body handling.
- Check empty, duplicate, malformed, timeout, cancellation, partial-write, and
  unavailable-upstream cases.
- Ensure errors returned to clients do not expose internals.

### Concurrency and lifecycle

- Look for data races, unsafe shared slices/maps, atomic sequencing mistakes,
  goroutine leaks, blocked channels, and missing cancellation.
- Verify readiness is set only after required listeners open and cleared before
  shutdown.
- Verify failures of either HTTP server stop the other and graceful shutdown is
  bounded while allowing active requests to finish.

### Architecture and configuration

- Enforce package dependency direction and scoped `AGENTS.md` invariants.
- Reject speculative abstractions and demo-service concepts inside `internal`.
- Ensure constructors validate retained state and copy caller-owned mutable data.
- Require strict YAML handling, semantic validation, defaults, both example
  configs, documentation, and tests for new fields.

### Observability, security, and performance

- Check logging for secrets, duplicate error logs, missing context, and
  unbounded request data.
- Check metric names, types, units, thread safety, and label cardinality.
- Flag blocking work, unbounded allocation, or global contention on hot paths.
- Treat authorization, CORS, forwarded headers, and panic output as trust
  boundaries.

### Tests, CI, and documentation

- Require unit tests for isolated logic and integration tests for behaviour
  spanning packages/listeners.
- Ensure every `internal` package remains strictly above 75% coverage.
- Look for flaky fixed ports, sleeps, external network dependencies, shared
  mutable fixtures, and tests that cannot fail for the intended regression.
- Keep build, unit-test, and integration-test CI jobs consistent with local
  harness commands.
- Verify examples and README commands match actual routes and configuration.

## Finding quality

Report a finding only when the change introduces or preserves a concrete defect,
regression, unsafe behaviour, missing required validation, or test gap that can
hide one. Do not report formatting preferences, speculative future work, or
general praise as findings.

Assign one priority:

- `P0`: immediate catastrophic impact or broad data/security loss;
- `P1`: serious correctness, security, availability, or CI breakage;
- `P2`: normal defect affecting a supported scenario;
- `P3`: localized low-impact issue worth fixing.

For each finding provide:

- concise title with priority;
- exact file and tight line range;
- triggering scenario;
- observed or likely consequence;
- why existing tests/checks do not prevent it.

Order findings by priority. When the host supports inline code comments, attach
findings directly to the affected lines.

## Final response

Lead with findings. If none exist, state that explicitly. Then include:

- validation commands run and their outcomes;
- residual risks or checks not run;
- a one-sentence scope summary.

Keep the review concise. Do not provide a change summary before findings and do
not claim approval when important validation could not be performed.
