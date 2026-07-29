# Logger package instructions

## Responsibility

This package converts validated logging configuration into a standard
`*slog.Logger`. It does not define application events or own global logger
state.

## Current behaviour

- `New` writes to `os.Stdout`.
- `NewWithWriter` supports deterministic tests and embedding.
- `text` selects `slog.TextHandler`; every other value selects JSON.
- Invalid levels fall back to `INFO`.

Configuration validation normally prevents invalid format and level values from
reaching this package. Keep the constructor defensive because tests or embedded
callers may build `config.Log` directly.

## Rules

- Return `*slog.Logger`; do not create a custom logger interface.
- Keep output structured and machine-readable in JSON mode.
- Do not add request-specific fields here; attach them at the call site.
- Do not write logs during logger construction.
- Never include secrets, authorization tokens, or full request bodies.
- If source locations or attribute replacement are added, make them explicit
  configuration because they affect output stability and cost.

## Tests

Use `NewWithWriter` with a buffer. Verify format, level filtering, and fallback
behaviour without depending on timestamps. Run:

```bash
go test -count=1 ./internal/logger
```

Keep coverage strictly above 75%.
