# Discovery package instructions

This package owns concrete conversion from a validated discovery declaration
to immutable upstream URL strings. Providers perform bounded, context-aware
resolution and return contextual errors; they do not publish runtime snapshots,
start refresh loops, mutate health state, or log.

Provider names are stable configuration contracts. DNS results must preserve
the configured scheme and port, handle IPv4 and IPv6 correctly, and reject an
empty result. Keep registry selection explicit and test cancellation, resolver
errors, empty answers, and address construction. Maintain coverage above 75%.
