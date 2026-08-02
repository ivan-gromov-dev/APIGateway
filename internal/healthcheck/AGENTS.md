# Active health-check instructions

This package owns periodic HTTP probes of configured upstream targets. Copy
caller-owned target slices, use request contexts and per-probe deadlines, close
response bodies, stop tickers on cancellation, and keep consecutive success and
failure streaks race-free per target.

Only configured 2xx and 3xx probe responses are healthy. Threshold crossings
may update explicit upstream health APIs; routing, passive failure
classification, logging, and lifecycle composition remain outside this package.
Tests must avoid sleeps and fixed ports and retain coverage above 75%.
