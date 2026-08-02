# Demo service instructions

Examples are small deterministic services for local and Docker demonstrations;
they are not gateway runtime dependencies. Use the standard library or existing
project dependencies, configurable listen addresses, `/healthz` where the
protocol permits it, bounded shutdown where useful, and responses that make
routing or balancing behaviour visible.

Do not move gateway policy into demo services. Keep images non-root and minimal,
add focused tests for non-trivial behaviour, and document commands in README.
