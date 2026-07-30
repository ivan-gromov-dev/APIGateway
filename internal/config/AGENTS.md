# Config package instructions

## Responsibility

This package is the single source of truth for runtime configuration. It loads
YAML, applies defaults, applies supported environment overrides, and validates
the final configuration before any listener is opened.

## Load pipeline

`Load` must preserve this order:

1. construct defaults;
2. decode exactly one YAML document;
3. reject unknown fields;
4. apply environment overrides;
5. validate the final value.

Do not silently ignore malformed YAML, invalid environment values, unknown
fields, or invalid routes.

## Current model

- `Config.Server` configures the public HTTP server.
- `Config.Admin` configures the operational HTTP server.
- `Config.Log` accepts `json` or `text` and a valid `slog` level.
- `Config.Middleware` contains request timeout and CORS policy.
- `Config.CircuitBreaker` contains the failure threshold, open timeout, and
  passive failure statuses used by the circuit breaker state machine.
- Each `Route` has a unique absolute `path_prefix`, one or more HTTP(S)
  `upstreams`, and optional prefix stripping.

Public and admin addresses must differ. Shutdown timeout must be positive.
Other configured timeouts cannot be negative. Routes and upstreams cannot be
duplicated.

## Adding a field

When adding a configuration field:

1. add the typed field and YAML tag;
2. add a conservative default where appropriate;
3. update strict nested-field allowlists if custom unmarshalling is involved;
4. add semantic validation;
5. add an environment override only when operationally useful;
6. update `configs/gateway.yaml` and `configs/gateway.docker.yaml`;
7. document the field in the root README;
8. test the valid value, malformed value, unknown field, and default behaviour.

Duration fields are represented as `time.Duration` internally and parsed from
Go duration strings such as `250ms` and `10s`.

## Error behaviour

Errors should identify the route/upstream index or environment variable involved.
Do not log inside this package; return contextual errors to the caller.

## Tests

Use temporary YAML files and `t.Setenv`. Tests must not depend on the developer's
real environment. Run:

```bash
go test -count=1 ./internal/config
```

Keep coverage strictly above 75%.
