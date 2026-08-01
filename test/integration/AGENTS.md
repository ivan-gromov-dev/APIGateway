# Integration test instructions

Exercise the gateway in-process over real TCP using `internal/testenv`; enable
only required demo services. Prefer its request and assertion helpers, rely on
`t.Cleanup`, and call `Stop` directly only for shutdown scenarios. Extend the
harness only for infrastructure reused across scenarios; keep tested behaviour
visible.

Organize scenarios by behaviour (routing, balancing, middleware, lifecycle, or
a focused feature file). Avoid fixed ports, sleeps, external processes/network,
and cross-test ordering. Run new scenarios repeatedly to expose flakes.
