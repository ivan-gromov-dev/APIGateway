# Telemetry package instructions

This package owns OpenTelemetry tracing setup and HTTP context propagation.

- Keep tracing optional and use explicit providers instead of mutable globals.
- Export through OTLP to a Collector; do not couple the gateway to a tracing backend.
- Keep attribute cardinality bounded and never attach tokens or request bodies.
- Provider shutdown must flush spans and participate in server lifecycle errors.
- Unit tests must cover disabled mode, propagation, and injected providers.
