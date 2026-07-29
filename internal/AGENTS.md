# Internal package instructions

These rules apply to all production code under `internal`. Read the
package-specific `AGENTS.md` as well; it contains the authoritative details for
that module.

## Why this file exists

The root instructions describe the whole repository. Package files describe one
module. This file captures policies shared by every production package:
dependency direction, ownership, concurrency, errors, API design, and validation.
Keep package-specific behaviour out of this file.

## Dependency direction

The current dependency graph is intentionally simple:

```text
config ───────────────┐
balancer ──► proxy ───┼──► server
config ─────► proxy ──┘
config ─────► logger
config ─────► middleware ─► metrics
metrics ───────────────────► server
```

- Keep dependencies acyclic.
- `server` is the composition boundary and may depend on lower-level packages.
- Lower-level packages must not import `server`.
- Algorithms such as balancing must not depend on HTTP lifecycle or demo
  services.
- Demo concepts such as Users and Billing must never enter `internal`.
- Introduce a new package only when it owns a distinct responsibility and
  reduces, rather than hides, coupling.

## API and ownership

- Prefer concrete types and constructors.
- Introduce a small interface when a consumer needs substitution or multiple
  implementations exist; avoid speculative interfaces.
- Make ownership explicit. Copy caller-owned slices or maps when retaining them.
- Do not expose mutable internal state.
- Keep exported APIs small; most implementation details should remain
  package-private.
- Public functions, types, and non-obvious invariants require GoDoc comments.

## Context and concurrency

- Any value accessed by concurrent requests must be immutable, synchronized, or
  atomic.
- Document whether exported types and methods are safe for concurrent use.
- Do not start background goroutines without an owner, cancellation path, and
  completion strategy.
- Preserve request contexts and cancellation across package boundaries.
- Never use sleeps as synchronization in production code or deterministic tests.
- Run concurrency-sensitive changes with the race detector when supported.

## Errors and logging

- Return errors with actionable context using `%w` when the caller may inspect
  the cause.
- Reject invalid state at construction or startup rather than failing later on
  a request path.
- Do not both log and return the same error at multiple layers. Log at an
  operational boundary; lower layers normally return context-rich errors.
- Client responses must not expose internal topology, stack traces, or secrets.
- Never log authorization tokens, credentials, or unbounded request bodies.

## Performance and dependencies

- Request hot paths should avoid blocking, unbounded allocation, and global
  contention.
- Prefer the standard library when it provides the required behaviour.
- Any new dependency needs a concrete benefit, maintenance assessment, and
  corresponding `go.mod`/`go.sum` update.
- Optimizations require a benchmark or load-test comparison when their value is
  not obvious.

## Change checklist

When changing an `internal` package:

1. read its scoped `AGENTS.md`;
2. preserve its documented invariants or update the contract explicitly;
3. add focused unit tests;
4. add an integration scenario when behaviour crosses packages or listeners;
5. update configuration examples and README when user-visible behaviour changes;
6. keep that package's statement coverage strictly above 75%;
7. run the package tests, relevant integration tests, and the verification
   harness.
