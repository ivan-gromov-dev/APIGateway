# Rate limit contract instructions

This package defines internal contracts between middleware, Token Bucket
policy, and Redis persistence. It is not a public SDK. Backend replacement is
expected through Redis protocol compatibility; Store substitution remains
useful for deterministic tests and gateway-local wiring.

Keep registration startup-only and reject duplicate or post-freeze mutations.
Maintain statement coverage strictly above 75%.
