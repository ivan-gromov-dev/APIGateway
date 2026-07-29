# Load scenarios

Start the complete demo before running these scenarios:

```bash
docker compose up --build
```

Load tests are intentionally not part of CI because their results depend on
the runner's CPU, networking, and neighbouring workloads.

## hey

`hey` is convenient for quick, single-endpoint checks. It uses a fixed number
of workers and prints throughput, latency distribution, and HTTP status counts.

Smoke test:

```bash
hey -n 100 -c 5 http://localhost:8080/api/users
```

30-second baseline:

```bash
hey -z 30s -c 50 http://localhost:8080/api/users
```

Billing write workload:

```bash
hey -z 30s -c 25 -m POST -H "Content-Type: application/json" -d "{}" \
  http://localhost:8080/api/billing/payments
```

Short stress run:

```bash
hey -z 2m -c 200 http://localhost:8080/api/users/42
```

## wrk

`wrk` is better for sustained high throughput. It uses a small number of event
loop threads and supports Lua scripts for mixed workloads.

Single endpoint:

```bash
wrk -t4 -c100 -d30s --latency http://localhost:8080/api/users
```

Mixed users and billing traffic:

```bash
wrk -t4 -c100 -d60s --latency -s test/load/mixed.lua \
  http://localhost:8080
```

## Reading the result

Track requests per second, p50/p95/p99 latency, socket errors, timeouts, and
non-2xx responses. Warm the gateway before recording a baseline and change one
variable at a time. Compare gateway results with direct upstream requests to
estimate proxy overhead, and watch `/metrics`, CPU, memory, and goroutine counts
while the scenario runs.
