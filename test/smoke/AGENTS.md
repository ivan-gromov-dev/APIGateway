# Docker smoke-test instructions

Smoke checks exercise the already-running default Compose stack through its
published public and admin interfaces. Keep them deterministic, bounded by
deadlines, independent of invocation order, and limited to local demo data.

Cover one representative path for every enabled cross-cutting capability.
PowerShell and POSIX runners must remain behaviourally equivalent and must not
tear down or mutate unrelated Docker resources.
