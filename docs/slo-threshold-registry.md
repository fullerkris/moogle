# SLO Threshold Registry

This registry is the single source of truth for release gating and rollback thresholds.

Use these values in:

- `docs/release-checklist.md`
- `docs/rollback-runbook.md`
- alert rules and dashboards in `observability/`

## Ownership

- Primary owner: Platform/Infrastructure
- Review cadence: monthly
- Change approval: Platform owner + application owner

## Thresholds

| Signal | Warning | Critical (Release/Rollback Gate) | Window | Notes |
|---|---:|---:|---|---|
| Query API 5xx ratio | > 1% | > 3% | 5m | Critical sustained breach triggers rollback review. |
| Query API p95 latency | > 400ms | > 800ms | warning 10m / critical 5m | Use same query mix as smoke/load baseline. |
| Queue depth (pages queue) | > 10,000 and rising | > 50,000 and rising | 15m | Rising trend required to avoid false alarms. |
| Oldest queue message age | > 5m | > 15m | warning 5m / critical 5m | Indicates consumer lag or stalled workers. |
| Worker restart count (per service) | >= 3 | >= 6 | 10m | Count by deployment/service. |
| Redis memory usage | > 75% | > 90% | 5m | Track query and pipeline Redis separately. |
| Pipeline Redis evictions | n/a | any (> 0) | 5m | Any eviction on pipeline Redis is treated as critical for durability. |
| Backup freshness | n/a | last successful backup > 26h | instant | Hard stop for production releases. |

## Spider Starter Thresholds

Tune these after 2-4 weeks of production telemetry.

| Signal | Warning | Critical (Release/Rollback Gate) | Window | Notes |
|---|---:|---:|---|---|
| Spider timeout ratio | > 10% | > 15% | 10m | Based on `moogle_spider_fetch_total` timeout/sum ratio. |
| Spider enqueue reject ratio | > 15% | > 25% | 10m | Excludes expected duplicate rejection if needed by query filter. |
| Spider fetch success throughput | < 2 pages/s | < 1 page/s (with queue depth > 100) | 10m | Alert only when frontier backlog exists. |
| Spider run budget remaining | < 20% | < 10% | instant + 5m confirm | Scope `run_attempts` and `run_successes`. |
| Spider bypass usage share | > 3% | > 5% | 15m | Share of bypassed fetches over all fetches. |

## How To Update

1. Propose change with rationale and expected impact.
2. Update this file and matching alert rules.
3. Validate thresholds in staging.
4. Record approval in release notes/change log.

## Change Log

| Date (UTC) | Change | Author | Approvers |
|---|---|---|---|
| 2026-04-27 | Initial threshold registry baseline. |  |  |
| 2026-05-01 | Added explicit warning/critical windows and finalized starter alert threshold definitions (API, queue, restarts, Redis, backup freshness). |  |  |
