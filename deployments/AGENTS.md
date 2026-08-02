# Deployment demo instructions

Deployment files must remain reproducible local demonstrations. Pin image
versions, keep the default Compose stack runnable from the repository root,
declare health checks for startup dependencies, and expose only interfaces
needed by the demo. Credentials and disabled security controls must be clearly
identified as local-only.

When a gateway capability is demonstrated, keep its service, strict YAML
configuration, smoke checks, and README instructions consistent. Validate with
`docker compose config` and a clean build/start/smoke/stop cycle.
