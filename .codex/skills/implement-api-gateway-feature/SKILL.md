---
name: implement-api-gateway-feature
description: Plan and implement new API Gateway capabilities across architecture, strict YAML configuration, runtime wiring, middleware or proxy behaviour, tests, Docker, CI, and documentation. Use when asked to design, add, build, or extend a gateway feature, or to turn a feature idea such as authentication, caching, health tracking, rate limiting, observability, service discovery, or protocol support into a repository-aligned implementation plan or completed change.
---

# Implement API Gateway Feature

Turn a feature request into an explicit design and, when requested, a verified
implementation that follows this repository's architecture and instructions.

## Respect the requested mode

- If the user asks for a concept, design, or plan, remain read-only and return
  an implementation-ready plan.
- If the user asks to add, build, implement, or make the feature, implement it
  through validation.
- If the user explicitly asks to do the work themselves, explain decisions and
  ordered tasks without editing files.
- Do not commit, push, tag, or open a PR unless separately requested.

## Establish the baseline

Before proposing or changing code:

1. read the root `README.md`;
2. inspect `git status --short --branch` and preserve unrelated changes;
3. read the root `AGENTS.md` and every nested `AGENTS.md` applying to affected
   paths;
4. inspect current configuration, constructors, wiring, tests, Docker, and CI
   related to the feature;
5. identify whether the requested capability is already partially implemented.

Use current repository evidence instead of assuming the architecture from a
previous conversation.

## Design the feature boundary

Define these contracts before editing:

- owning package and dependency direction;
- consumer-defined interfaces only where substitution is real;
- configuration shape, conservative defaults, strict validation, and startup
  failure behaviour;
- request lifecycle and middleware ordering;
- HTTP semantics, context cancellation, concurrency ownership, and resource
  cleanup;
- fail-open or fail-closed policy for dependency failures;
- observability without secrets or high-cardinality labels;
- unit, integration, race, and coverage expectations;
- Docker/demo and documentation impact.

For authentication, retries, caches, limits, forwarded headers, or other trust
boundaries, state abuse cases and data-isolation rules explicitly. Do not invent
unsafe fallback behaviour merely to keep requests succeeding.

## Present the plan

Before a broad implementation, give the user a concise plan containing:

1. package ownership and public contracts;
2. configuration and validation;
3. runtime wiring and ordering;
4. failure and security semantics;
5. test and validation coverage.

Ask for input only when a missing choice materially changes the public contract
or requires new authority. Otherwise state reasonable assumptions and proceed
when implementation was requested.

## Implement incrementally

When edits are authorized:

1. add or update the smallest stable contracts;
2. implement isolated behaviour with focused unit tests;
3. add strict configuration parsing and semantic validation;
4. wire dependencies at `cmd/gateway` or `internal/server`, keeping lower-level
   packages independent of lifecycle composition;
5. add integration scenarios through the shared test harness when behaviour
   crosses packages or listeners;
6. update Docker Compose, examples, CI, README, and applicable `AGENTS.md` files
   when their contracts change;
7. explain every new dependency added to `go.mod` and prefer the standard
   library when it is sufficient.

Preserve cancellation, graceful shutdown, Request IDs, forwarding behaviour,
safe-method/replayability rules, route precedence, and existing user changes.
Do not lower coverage or weaken validation to make checks pass.

## Validate proportionally

Run focused checks while iterating, then run the repository gate for a completed
cross-cutting feature:

```powershell
.\scripts\verify.ps1
```

or:

```bash
sh scripts/verify.sh
```

Also verify every affected `internal` package remains strictly above 75%
statement coverage. Use race detection when supported. Validate Compose or
external-service flows when the feature changes them; clearly report unavailable
local tooling rather than claiming those checks passed.

## Handoff

Lead with the achieved behaviour. Name important files, tests, full validation,
coverage or environment limitations, and any intentionally deferred work. Keep
the worktree uncommitted unless the user requested a commit.

Recommend `$review-api-gateway` as the separate read-only pre-commit or pre-PR
review step; do not treat successful implementation checks as an independent
code review.
