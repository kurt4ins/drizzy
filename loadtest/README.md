# Load testing

Apache JMeter test plan for `profile-service` — the read-heavy service that
serves the bot's "browse" flow.

## Run

Headless run with 50 virtual users over 2 minutes against a locally running
stack (`docker compose up`):

```bash
mkdir -p loadtest/results
jmeter -n -t loadtest/profile-service.jmx \
  -Jhost=localhost -Jport=8080 \
  -Jthreads=50 -Jrampup=30 -Jduration=120 \
  -Juser_id=<existing-uuid> \
  -l loadtest/results/run.jtl \
  -e -o loadtest/results/html
```

Open `loadtest/results/html/index.html` for the full HTML report (per-endpoint
percentiles, throughput, error rate).

## Endpoints exercised

| Endpoint                         | Why                                                     |
| -------------------------------- | ------------------------------------------------------- |
| `GET /healthz`                   | Baseline — checks postgres/redis/minio pings end-to-end |
| `GET /api/v1/users/{user_id}`    | DB lookup by primary key                                |
| `GET /api/v1/profiles/{user_id}` | DB lookup + JSONB read                                  |

## Suggested ramps

| Profile  | Threads | Ramp-up | Duration |
| -------- | ------- | ------- | -------- |
| Smoke    | 10      | 10s     | 60s      |
| Baseline | 50      | 30s     | 120s     |
| Stress   | 200     | 60s     | 300s     |

Track p95/p99 of `GET /api/v1/profiles/...` while increasing concurrency until
`drizzy_http_request_duration_seconds` (Prometheus, scraped from `/metrics`)
crosses 250ms p95 — that's the saturation point.
