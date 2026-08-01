# Balancer package instructions

Own HTTP-independent upstream selection. A route gets an independent balancer;
`Next` is non-blocking/network-free, concurrency-safe, and returns an available
target plus a callback that callers must complete. Constructors return concrete
types, reject empty input, and copy retained slices. Target identity/health
persists across selections.

New algorithms use separate implementations and explicit input types. Document
fairness, state, overflow, complexity, and concurrency; add a compile-time
contract assertion. Test validation, deterministic order where applicable,
wraparound, input copying, unavailable targets, callbacks, and concurrency.
