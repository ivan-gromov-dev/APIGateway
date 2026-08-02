# Rate store instructions

This directory owns persistence adapters for `internal/ratelimit.Store`. A
backend must make refill and admission atomic, use a consistent time source,
preserve contexts, and namespace external identities without logging raw keys
or credentials.

Keep rule selection, fail-open/fail-closed policy, and HTTP response behaviour
outside store packages. Implement each backend in its own subpackage and test
concurrency and backend-specific error mapping. Every Go package must retain
statement coverage above 75%.
