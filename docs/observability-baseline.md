# Observability Baseline

This baseline adds first-pass observability assets for production migration.

## Included Assets

- `observability/docker-compose.yml`: local Prometheus + Grafana stack
- `observability/prometheus/prometheus.yml`: scrape config baseline
- `observability/prometheus/alerts.yml`: initial warning/critical alert rules
- `observability/exporters/runtime-metrics/*`: runtime exporter for queue + Redis health metrics
- `observability/grafana/provisioning/*`: Grafana datasource/dashboard provisioning
- `observability/grafana/dashboards/moogle-runtime-overview.json`: starter runtime dashboard
- `docs/weekly-readiness-scorecard-template.md`: weekly RAG review template
- `scripts/ops/generate-weekly-scorecard.sh`: helper to create weekly scorecard files

## Startup

```bash
docker compose -f observability/docker-compose.yml up -d
```

## Kubernetes (Prometheus Operator)

If your cluster uses Prometheus Operator, apply ServiceMonitor resources:

```bash
kubectl apply -k k8s/monitoring-operator
```

This switches scrape discovery from annotation-only to explicit ServiceMonitor resources.

## Alert Baseline Targets

- Query-engine 5xx ratio critical > 3% (5m)
- Query-engine p95 latency critical > 800ms (5m)
- Queue depth critical > 50,000 (15m)
- Oldest queue message critical > 15m
- Redis memory critical > 90%
- Backup freshness critical if no successful backup in 26h

## Notes

- Query-engine now exposes `/metrics` from Laravel for request count + latency histogram metrics.
- Spider/indexer now track queue enqueue timestamps under `pages_queue_enqueued_at` so oldest message age can be exported.
- Runtime exporter emits queue depth, oldest message age, Redis memory usage/capacity, and optional backup freshness.
- Grafana now auto-loads the starter runtime dashboard from provisioning at container startup.
- Runtime exporter image publish flow is automated in `.github/workflows/build-docker-images.yml` (`build-runtime-metrics-exporter`).
