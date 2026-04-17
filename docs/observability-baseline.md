# Observability Baseline

This baseline adds first-pass observability assets for production migration.

## Included Assets

- `observability/docker-compose.yml`: local Prometheus + Grafana stack
- `observability/prometheus/prometheus.yml`: scrape config baseline
- `observability/prometheus/alerts.yml`: initial warning/critical alert rules
- `docs/weekly-readiness-scorecard-template.md`: weekly RAG review template
- `scripts/ops/generate-weekly-scorecard.sh`: helper to create weekly scorecard files

## Startup

```bash
docker compose -f observability/docker-compose.yml up -d
```

## Alert Baseline Targets

- Query-engine 5xx ratio critical > 3% (5m)
- Query-engine p95 latency critical > 800ms (5m)
- Queue depth critical > 50,000 (15m)
- Oldest queue message critical > 15m
- Redis memory critical > 90%
- Backup freshness critical if no successful backup in 26h

## Notes

- Some targets require exporters/instrumentation to expose metrics with the referenced names.
- This PR provides the baseline contract and rule set; follow-up PRs should wire concrete exporters and dashboards.
