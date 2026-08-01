# Metrics package instructions

Own the private Prometheus registry, gateway observations, and admin-only
`/metrics` handler; never use the global registry. Hot-path observation must be
non-blocking and concurrency-safe. Export durations in seconds and preserve
valid HELP/TYPE output.

Before adding a metric define its name, type, unit, bounded labels, and update
point. Counters are monotonic; route labels use configured patterns. Never label
with raw URLs, Request IDs, identities, tokens, or full upstream addresses.
Test exposition, values/types/units, content type, classification, and concurrent
observations.
