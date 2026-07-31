# Authentication package instructions

This package verifies externally issued JWTs. It never issues tokens. Require an
explicit algorithm allowlist and validate signature, issuer, audience, expiry,
and not-before. JWKS state must be concurrency-safe. Never log tokens or claims.
Keep HTTP authorization responses in middleware.
