# Observability Baseline

This baseline adds first-pass observability assets for production migration.

Threshold source of truth: `docs/slo-threshold-registry.md`.

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
scripts/kubectl-with-config.sh k8s/kubeconfig.local.yaml apply -k k8s/monitoring-operator
```

If Prometheus CRDs were installed after the operator pod started, restart the operator once so all controllers initialize:

```bash
scripts/kubectl-with-config.sh k8s/kubeconfig.local.yaml rollout restart deployment/prometheus-operator -n default
```

This now applies:

- ServiceMonitor resources for `query-engine-metrics` and `runtime-metrics`.
- ServiceMonitor resource for `spider-metrics` (active when a `spider-metrics` service exists in namespace `moogle`).
- A minimal local `Prometheus` custom resource (`moogle-local`) that scrapes ServiceMonitors in namespace `moogle`.
- Namespaced RBAC (`ServiceAccount`/`Role`/`RoleBinding`) for service/endpoints/pod discovery.

Follow-up tuning:

- Revisit Prometheus retention/storage and resource limits for non-local environments.

## Alert Baseline Targets

- Query-engine 5xx ratio critical > 3% (5m)
- Query-engine p95 latency critical > 800ms (5m)
- Queue depth critical > 50,000 (15m)
- Oldest queue message critical > 15m
- Redis memory critical > 90%
- Backup freshness critical if no successful backup in 26h

Use warning and critical values from `docs/slo-threshold-registry.md` when adding/updating alert rules.

## Notes

- Query-engine now exposes `/metrics` from Laravel for request count + latency histogram metrics.
- Spider/indexer now track queue enqueue timestamps under `pages_queue_enqueued_at` so oldest message age can be exported.
- Runtime exporter emits queue depth, oldest message age, Redis memory usage/capacity, and optional backup freshness.
- Grafana now auto-loads the starter runtime dashboard from provisioning at container startup.
- Runtime exporter image publish flow is automated in `.github/workflows/build-docker-images.yml` (`build-runtime-metrics-exporter`).
- Spider now exports native Prometheus metrics at `/metrics` (default `:2113`) for fetch outcomes, enqueue results, policy decisions, bypass events, and budget remaining.
- Local Prometheus scrapes Spider metrics via `host.docker.internal:2113`; ensure the Spider runtime publishes `127.0.0.1:2113:2113`.

## Spider PromQL Quick Queries

- Fetch success rate (pages/sec): `sum(rate(moogle_spider_fetch_total{result="success"}[5m]))`
- Timeout ratio: `sum(rate(moogle_spider_fetch_total{result="timeout"}[5m])) / clamp_min(sum(rate(moogle_spider_fetch_total[5m])), 1)`
- Enqueue reject ratio (excluding duplicate): `sum(rate(moogle_spider_enqueue_total{result="rejected",reason!="duplicate"}[5m])) / clamp_min(sum(rate(moogle_spider_enqueue_total[5m])), 1)`
- Policy deny rate: `sum(rate(moogle_spider_policy_decision_total{decision="deny"}[5m]))`
- Bypass usage share: `sum(rate(moogle_spider_bypass_total{result="granted"}[5m])) / clamp_min(sum(rate(moogle_spider_fetch_total[5m])), 1)`
- Fetch p95 duration: `histogram_quantile(0.95, sum(rate(moogle_spider_fetch_duration_seconds_bucket[5m])) by (le))`
