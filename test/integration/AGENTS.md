# Integration test instructions

Integration tests exercise the gateway in-process through real TCP listeners.
They must remain fast, deterministic, parallel-CI-safe, and independent of
Docker.

## Using the harness

- Construct environments with `internal/testenv`.
- Enable only services required by the scenario:

```go
env := testenv.New(
    t,
    testenv.WithUsers(3),
    testenv.WithBilling(),
)
```

- Use `GET`, `POST`, `Do`, and `AdminGET` instead of creating ad hoc clients.
- Use response assertions such as `RequireStatus`, `RequireService`,
  `RequireUpstreamPath`, and `RequireRequestID`.
- Let `t.Cleanup` stop the environment. Call `env.Stop()` directly only when
  shutdown is the behaviour under test.

## Scenario organization

- `routing_test.go`: route matching and path transformations.
- `balancing_test.go`: distribution and concurrency.
- `middleware_test.go`: request/response cross-cutting behaviour.
- `lifecycle_test.go`: health, readiness, startup, and shutdown.
- Add focused files such as `retry_test.go` or `circuit_breaker_test.go` as those
  features arrive.

Extend `internal/testenv` only for infrastructure reused by multiple scenarios.
Do not hide the behaviour under test behind a large helper.

Avoid fixed ports, sleeps as synchronization, external processes, internet
access, and ordering dependencies between tests. Run new scenarios repeatedly:

```bash
go test -count=10 ./test/integration/...
```
