# Config package instructions

This package is the runtime configuration source of truth. `Load` must: build
defaults, decode exactly one YAML document with unknown fields rejected, apply
supported environment overrides, then semantically validate before listeners
open. Reject malformed values, duplicate routes/upstreams, invalid URLs,
negative timeouts, and equal public/admin addresses with indexed/contextual
errors; never log here.

For each new field add its typed YAML representation, conservative default when
appropriate, strict nested handling, validation, useful env override, both
checked-in configs, README documentation, and tests for valid, malformed,
unknown, and default cases. Use Go duration strings and isolate tests with
temporary files plus `t.Setenv`.
