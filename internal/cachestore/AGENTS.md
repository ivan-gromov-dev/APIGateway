# Cache store instructions

This directory owns persistence adapters for `internal/cache.Store`. Adapters
must preserve contexts, bound operation time through their callers, and return
inspectable errors without exposing keys, credentials, or cached payloads.

Keep cache policy, HTTP eligibility, and response-size enforcement in
`internal/middleware`; stores only persist and retrieve already-approved
entries. Implement each backend in its own subpackage and test backend-specific
failure mapping. Every Go package must retain statement coverage above 75%.
