# Local observability stack

This optional Docker Compose override runs the complete local demo:

- Prometheus scrapes Gateway metrics;
- Grafana provisions the Prometheus datasource and API Gateway dashboard;
- Filebeat decodes Docker JSON logs and writes them to Elasticsearch;
- Kibana provides log discovery and Elastic APM views;
- OpenTelemetry Collector receives OTLP/HTTP traces and forwards them to APM Server.

Run from the repository root:

```powershell
docker compose -f docker-compose.yml -f deployments/observability/docker-compose.yml up --build
```

Open:

| Service | URL | Credentials |
| --- | --- | --- |
| Grafana | http://localhost:3000 | `admin` / `admin` |
| Prometheus | http://localhost:9091 | none |
| Kibana | http://localhost:5601 | none |
| Elasticsearch | http://localhost:9200 | none |
| APM Server | http://localhost:8200 | none |

In Kibana, use Discover with the Filebeat data view for logs and Observability
APM for traces. Filebeat may take a short time to install its data view and
dashboards after Kibana first becomes available.

Grafana provisions `API Gateway Overview` from the repository on every start
and uses it as the home dashboard. It is also available under Dashboards in the
`API Gateway` folder. The provisioned dashboard is read-only; edit its JSON file
in the repository to keep changes reproducible.

This is a local-only setup. Authentication and TLS are intentionally disabled,
the Grafana password is public, and Filebeat can read the Docker socket. Do not
deploy these settings to a shared or public environment. Expect roughly 4 GB of
available Docker memory for the complete stack.
