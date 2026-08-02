# gRPC proxy package instructions

Own transparent gRPC routing and HTTP/2 upstream transport. Preserve streaming,
trailers, metadata, deadlines, and cancellation. Never reuse the HTTP retry
transport: a call receives exactly one upstream attempt unless a future policy
can prove the RPC is unary, replayable, and idempotent. Keep route labels
configured and bounded; never derive labels from arbitrary RPC method paths.
