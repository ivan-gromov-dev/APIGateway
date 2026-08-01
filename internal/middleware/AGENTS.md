# Middleware package instructions

Keep one composable HTTP concern per file and route business logic elsewhere.
Declaration order is outermost-first and observable; explicitly choose each new
middleware's short-circuiting, context/header mutations, response behaviour,
and position.

Preserve Request IDs end-to-end. Never trust client values for authorization.
Response wrappers record the first status, infer 200 on `Write`, and preserve
optional writer capabilities via `Unwrap`. Recovery hides panic details;
timeout/recovery never double-write committed responses; origin-dependent CORS
sets `Vary: Origin`. Avoid global mutable state. Test pass-through,
short-circuit, headers/status/body, panic/error paths, order, and concurrency.
