# Limiter package instructions

This package owns Token Bucket policy wiring and remains independent of HTTP
and Redis. State changes go through the internal atomic `ratelimit.Store`
contract so alternative backends preserve correctness.

Propagate request contexts and keep statement coverage strictly above 75%.
