# API Gateway Roadmap

This roadmap describes the intended direction after the 1.0 stable baseline.
It is ordered by dependency and operational value, not by calendar date. A
milestone is complete only when its configuration, runtime behaviour, failure
semantics, observability, tests, Docker demonstration, and documentation land
together.

The roadmap is directional rather than a compatibility promise. Public 1.x
configuration remains backward compatible unless a security or correctness
defect requires a documented change.

## Status legend

- **Next**: the highest-priority implementation target;
- **Planned**: expected after the preceding foundations are complete;
- **Exploratory**: valuable, but requires a design decision before scheduling;
- **Out of scope**: intentionally delegated to a mature edge or platform.

## 1.1 — Continuous discovery and target lifecycle

**Status: Next**

Make discovery independent of configuration-file changes while retaining a
stable, race-free routing snapshot.

### Deliverables

- refresh DNS routes at their configured `interval`;
- retain the last valid target set for the configured `grace` period when
  resolution fails or returns no usable addresses;
- reconcile added, retained, and removed targets without discarding circuit and
  health state for unchanged instances;
- cancel refresh workers and active probes when a runtime snapshot is retired;
- expose bounded refresh outcome, target-count, and staleness metrics;
- keep readiness successful while a usable non-expired snapshot exists and
  define explicit behaviour after grace expires;
- demonstrate DNS membership changes in integration and Docker scenarios.

### Completion gates

- deterministic tests cover refresh, cancellation, stale retention, grace
  expiry, address changes, IPv4/IPv6, and concurrent requests;
- race tests show no mutation of published target slices;
- invalid discovery configuration still fails before publication;
- existing static routes remain allocation- and goroutine-free outside their
  current request path.

## 1.2 — Resilience budgets and overload protection

**Status: Planned**

Move from isolated retries and circuit thresholds to bounded, coordinated
resource use under partial failure and overload.

### Deliverables

- per-route retry budgets that cap amplification during upstream incidents;
- configurable concurrency limits with immediate rejection or a bounded queue;
- richer passive outlier detection with an explicit recovery policy;
- optional per-route request-body limits enforced before proxying;
- load-shed responses with consistent status, `Retry-After`, logs, metrics, and
  traces;
- policy composition that preserves safe-method and replayability rules.

### Completion gates

- unsafe and non-replayable requests still receive at most one attempt;
- cancellation removes queued work promptly and no queue grows without bound;
- load tests record latency, error rate, retry amplification, and recovery under
  a reproducible upstream-failure scenario;
- all new metric labels come from validated, bounded configuration values.

## 1.3 — Operational release maturity

**Status: Planned**

Make releases and production evaluation reproducible without turning the
repository into a hosted control plane.

### Deliverables

- `-version` and `-check-config` process commands with build metadata;
- tagged multi-platform binaries and container images;
- checksums, SBOMs, provenance, and a documented release procedure;
- image vulnerability and dependency scanning in CI;
- configurable protection for sensitive admin endpoints, while keeping liveness
  and readiness usable by orchestrators;
- documented Kubernetes probes, resource limits, rolling shutdown, and
  edge-termination examples;
- a repeatable benchmark profile with comparison guidance rather than unstable
  shared-runner performance gates.

### Completion gates

- a release candidate can be built and verified from a clean checkout using one
  documented workflow;
- artifacts report the tag, commit, build time, and supported Go version;
- admin protection cannot accidentally expose `pprof` on the public listener;
- Docker and deployment examples use immutable release references.

## 1.4 — Discovery and policy extensibility

**Status: Planned**

Add carefully bounded integrations only after the discovery lifecycle is
proven with DNS.

### Deliverables

- one platform-native discovery provider selected by an architecture decision,
  such as Kubernetes EndpointSlices or Consul;
- provider-specific watch or polling implemented behind the existing discovery
  boundary;
- optional least-connections balancing using explicit in-flight ownership;
- route policy inspection through a sanitized admin endpoint;
- configuration deprecation diagnostics for future 1.x evolution.

### Completion gates

- provider outages follow the same stale/grace contract as DNS;
- provider clients are lifecycle-owned by composition code and shut down
  cleanly;
- external metadata cannot create unbounded metric labels or logs;
- no provider SDK leaks into routing, proxy, middleware, or upstream algorithms.

## 2.0 candidates

**Status: Exploratory**

These capabilities materially broaden the gateway's trust boundary or
architecture. Each requires an accepted design and migration plan before it can
enter a release milestone:

- downstream TLS termination, certificate reload, and optional client mTLS;
- a declarative remote control plane with authenticated, versioned snapshots;
- multi-instance configuration coordination and audit history;
- WebSocket-specific policies and HTTP/3 evaluation;
- pluggable authorization beyond route-level JWT scope checks;
- multi-tier caching and explicit invalidation APIs.

## Intentionally out of scope

The project will not attempt to reproduce the complete feature surface of
Nginx, Envoy, Traefik, or a managed API platform. The following remain the
responsibility of a mature external edge or dedicated security product unless
the project scope is explicitly redefined:

- general-purpose WAF and bot-management rules;
- DDoS absorption and global anycast traffic management;
- automatic public certificate issuance and domain ownership workflows;
- arbitrary user-supplied plugins in the request process;
- a hosted developer portal, billing platform, or API product marketplace.

## Rules for changing this roadmap

Every proposed milestone should identify the owning package, configuration
contract, failure policy, lifecycle owner, security boundary, observability,
test level, and demo impact. New work should not begin by adding a dependency or
configuration field before those decisions are explicit.

Completed work moves to `CHANGELOG.md`; this file should describe only the
current baseline and future direction. Changes to ordering or scope should be
reviewed like architecture changes rather than treated as a task checklist.
