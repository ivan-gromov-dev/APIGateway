# Internal package instructions

Also read the closest package `AGENTS.md`; it is authoritative for local
invariants.

## Shared rules

- Keep dependencies acyclic. `server` is the composition boundary; lower-level
  packages never import it. Algorithms remain independent of HTTP lifecycle and
  demo services. Pass telemetry providers explicitly; avoid mutable globals.
- Prefer concrete types and constructors. Add a small interface only for real
  consumer substitution. Keep exports minimal and document non-obvious
  invariants. Copy retained caller-owned slices/maps and expose no mutable state.
- Concurrent state must be immutable, synchronized, or atomic. Document exported
  concurrency guarantees. Every goroutine needs ownership, cancellation, and a
  completion path. Preserve contexts; never synchronize with sleeps.
- Reject invalid state at construction/startup. Wrap inspectable errors with
  `%w`. Lower layers return contextual errors; operational boundaries log them.
  Never expose topology, stack traces, credentials, tokens, or unbounded bodies.
- Avoid blocking, unbounded allocation, and global contention on request paths.
  Prefer the standard library; justify dependencies. Non-obvious optimizations
  need benchmark or load-test evidence.

Add focused unit tests and integration coverage when behaviour crosses packages
or listeners. Update public configuration examples and README as needed. Keep
each package strictly above 75% statement coverage and run focused tests plus
the repository verification harness; use the race detector when supported.
