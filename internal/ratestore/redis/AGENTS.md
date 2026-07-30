# Redis rate store instructions

This package implements `internal/ratelimit.Store`. Refill, admission, deduction,
persistence, and TTL must remain one atomic Redis script operation. Use Redis
server time, namespace and hash external identities, propagate contexts, and
never expose credentials.

Test result mapping locally and script behavior against real Redis. Keep
statement coverage strictly above 75%.
