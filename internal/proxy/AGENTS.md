# Proxy package instructions

Build one reverse proxy and independent balancer per route. Each attempt selects
one health-eligible target, joins URLs correctly, preserves context, method,
query, replayable body, forwarding/Request-ID/trace headers, and completes the
target callback before retry/return. Prefix stripping occurs outside the proxy.

Retry only configured statuses/transport failures under the safe replay policy.
Classify passive health independently: configured 5xx and transport errors fail,
client cancellation is neutral, other responses succeed. Return 502 for
transport failure and 503 for exhausted availability without leaking topology.
Keep metric labels bounded and attempt spans correctly closed. Test rewriting,
headers, balancing, failures/retries, callbacks, tracing, and concurrency with
`httptest`; cross-service behaviour belongs in integration tests.
