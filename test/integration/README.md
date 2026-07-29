# Integration tests

These tests run the gateway in-process while using real public and admin TCP
listeners. Test upstreams use `httptest.Server`, so the suite needs neither
Docker nor fixed local ports.

Create only the services needed by a scenario:

```go
env := testenv.New(
    t,
    testenv.WithUsers(3),
    testenv.WithBilling(),
)
```

Use the scenario-focused client and assertions:

```go
env.GET("/api/users").
    RequireStatus(http.StatusOK).
    RequireService("users-1").
    RequireUpstreamPath("/users")
```

Add behaviour tests to the relevant top-level scenario file. Extend
`internal/testenv` only when infrastructure is shared by multiple scenarios.
The harness registers cleanup automatically; call `env.Stop()` directly only
when shutdown itself is part of the scenario.

Run the suite with:

```bash
go test -race -count=1 ./test/integration/...
```
