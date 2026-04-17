# Moogle Production Migration Checklist

This checklist converts the Senior Developer + Backend Architect hardening guidance into concrete, repo-specific execution steps.

## Approved Migration Defaults

- Platform targets: support both VM deployments (Docker Compose) and Kubernetes deployments.
- Kubernetes packaging: Kustomize (`base` + environment overlays).
- Secret manager baseline: HashiCorp Vault (HA, KMS-backed auto-unseal).
- Secret rotation: every 180 days (plus emergency rotation when needed).
- Redis role contract: `PIPELINE_REDIS_URL` is required for broker/queue workloads; query cache/session must not share that keyspace.
- Reliability targets: keep current starter RPO/RTO/SLO values and tune after production telemetry.
- Release governance: PR-only to `main`, required checks (multi-language tests + build + vulnerability scan + smoke), and environment flow `dev -> staging -> production`.

## Current Baseline (from this branch)

- Polyglot services: Go (`spider`, `page-rank`), Python (`indexer`, `image-indexer`, `backlinks-processor`, `tfidf`), Laravel (`query-engine`), Vite (`client`), Rust (`monitoring`).
- Two Redis roles are active and should remain explicit:
  - Pipeline Redis (`moogle-redis`, currently exposed as host `:6380`)
  - Query-engine Redis (`query-engine-redis-1`, currently exposed as host `:6379`)
- Query engine serves at `http://localhost`; client now containerized and configurable via `VITE_BACKEND_URL`.

---

## Phase 0 - Must Complete Before Internet Exposure

### 1) Production Runtime Mode Only

- [ ] Disable dev workflows in production (`npm run dev`, Vite hot mode, `APP_DEBUG=true`).
- [ ] Build frontend assets in image build stage and verify `public/build/manifest.json` exists in final image.
- [ ] Set Laravel prod env: `APP_ENV=production`, `APP_DEBUG=false`.

### 2) Network and Access Hardening

- [ ] Expose only ingress/reverse proxy (80/443) to public network.
- [ ] VM ingress standard: Caddy edge reverse proxy with hardened TLS config.
- [ ] Kubernetes ingress standard: NGINX Ingress Controller + cert-manager.
- [ ] Keep MongoDB and Redis internal-only (no public port mappings in prod).
- [ ] Add network segmentation so pipeline workers cannot access query-only Redis/cache unless required.

### 3) Secrets and Credentials

- [ ] Move production secrets out of checked-in `.env` files.
- [ ] Use Vault as source-of-truth for Mongo/Redis credentials and keys.
- [ ] Separate DB users by role (query read-only vs pipeline write users).
- [ ] Enforce 180-day secret rotation schedule with owner + runbook.

### 4) Safety Controls

- [ ] Add health/readiness checks to all containers/services.
- [ ] Add per-service CPU/memory limits and restart policies.
- [ ] Confirm strict HTTP/database timeouts and bounded retries in each language runtime.

### 5) Data Protection

- [ ] Implement daily Mongo backup job.
- [ ] Run and document weekly restore test in non-prod.

---

## Phase 1 - First 2 Weeks

### 1) CI/CD Quality Gates (Required on Every PR)

- [ ] Go: `go test ./...`
- [ ] Python: `pytest` (or service test command)
- [ ] Laravel: `php artisan test`
- [ ] Client: `npm run build`
- [ ] Rust: `cargo test`
- [ ] Image vulnerability scan (fail build for critical vulnerabilities).

### 2) Queue Integrity and Replayability

- [ ] Add idempotency strategy for Redis->Mongo writes (upserts + deterministic keys/content hash).
- [ ] Add retry policy with backoff and max attempts.
- [ ] Add DLQ/quarantine path for poison messages.
- [ ] Store checkpoints/watermarks for replay and recovery.

### 3) Observability Baseline

- [ ] Centralize logs (JSON format with `service`, `trace_id`, `error`, `duration_ms`).
- [ ] Add metrics for queue depth, oldest message age, worker throughput, API latency, error rate.
- [ ] Create first alert set:
  - queue depth rising continuously for 15m
  - API p95 latency above threshold
  - service crash loops/restart spikes
  - Redis memory pressure/evictions

---

## Phase 2 - Next 1 to 2 Months

### 1) Availability and Scaling

- [ ] Introduce autoscaling signals by queue lag and message age.
- [ ] Isolate read path from write path contention (query-engine should remain responsive during indexing spikes).
- [ ] Add periodic consistency verification jobs across Mongo collections.

### 2) Supply Chain Security

- [ ] Pin image digests for base images.
- [ ] Generate SBOM for produced images.
- [ ] Sign artifacts/images before deployment.

### 3) Failure Drills and Runbooks

- [ ] Simulate Redis outage and verify graceful recovery.
- [ ] Simulate Mongo latency/failure and verify retry/backoff behavior.
- [ ] Validate rollback runbook end-to-end.

---

## Immediate Repo Changes to Track

### Compose and Deployment Structure

- [ ] Create environment-specific compose files:
  - `docker-compose.dev.yml`
  - `docker-compose.prod.yml`
- [ ] In prod compose:
  - remove bind mounts for application code
  - disable dev server commands
  - attach healthchecks and resource constraints

### Environment Variable Clarity

- [ ] Replace ambiguous Redis vars with explicit names:
  - `PIPELINE_REDIS_URL` (required)
  - `QUERY_REDIS_URL` (optional/isolated for query cache-session workloads)
- [ ] Add startup validation that fails fast if required vars are missing.

### Query-Engine Asset Reliability

- [ ] Keep Vite build in Docker image build pipeline.
- [ ] Add pre-start check to fail fast when `public/build/manifest.json` is missing.

### Client Stability

- [ ] Keep `VITE_BACKEND_URL` environment-driven for prod/staging/dev.
- [ ] Ensure production client points to stable API ingress URL, not localhost.

---

## Service-by-Service Hardening Notes

### Spider (Go)

- [ ] Enforce crawl budgets and domain rate limits.
- [ ] Set bounded queue insertion to prevent broker overload.
- [ ] Add metrics for fetched pages/sec, timeout rate, and enqueue failures.

### Indexer/Image Indexer/Backlinks/TF-IDF (Python)

- [ ] Ensure all write paths are idempotent (upsert or unique constraints).
- [ ] Apply backoff with jitter on transient Mongo/Redis errors.
- [ ] Add dead-letter handling for unparseable payloads.

### Page-Rank (Go)

- [ ] Tag each run with `run_id` and write results atomically.
- [ ] Keep previous rank snapshot until new run is marked complete.

### Query-Engine (Laravel)

- [ ] Add `healthz` and dependency-aware `readyz` endpoints.
- [ ] Add request rate limiting at edge/API gateway.
- [ ] Use cache key versioning for rank/index updates.

### Client (Vite)

- [ ] Use production build artifacts for deployment.
- [ ] Do not rely on dev server in production.

---

## Go-Live Gates (Measurable)

- [ ] Security gate: no critical runtime vulnerabilities.
- [ ] Reliability gate: end-to-end pipeline success >= 99.5% over 7 days.
- [ ] Performance gate: query API p95 latency within agreed SLO at expected load.
- [ ] Data gate: backup + restore drill completed within RPO/RTO targets.
- [ ] Operations gate: on-call runbook validated by engineer not authoring the change.
- [ ] Governance gate: release checklist, rollback runbook, and incident comms template are present and reviewed.

---

## Weekly Readiness Scorecard (R/Y/G)

Use this every week for staging and production.

| Domain | Check | Target | Status | Owner | Notes |
|---|---|---:|:---:|---|---|
| Security | Mongo/Redis private-only access | 100% |  |  |  |
| Security | Secrets managed outside repo/env files | 100% |  |  |  |
| Build | Immutable images + pinned digest | 100% |  |  |  |
| Build | Critical vulnerabilities | 0 |  |  |  |
| Reliability | Service healthcheck success | >99.9% |  |  |  |
| Reliability | Crash loop incidents | 0 |  |  |  |
| Performance | Query p95 latency | SLO met |  |  |  |
| Pipeline | Queue backlog growth alarms | 0 active |  |  |  |
| Data | Backup success | Daily |  |  |  |
| Data | Restore drill success | Weekly/Monthly |  |  |  |
| Observability | Logs + metrics coverage | 100% |  |  |  |
| Delivery | Required CI gates enforced | 100% |  |  |  |

## Starter Alert Thresholds

- [ ] API 5xx warning >1%/5m, critical >3%/5m.
- [ ] API latency warning p95 >400ms/10m, critical p95 >800ms/5m.
- [ ] Queue depth warning >10k + rising/15m, critical >50k + rising/15m.
- [ ] Oldest queue message warning >5m, critical >15m.
- [ ] Worker restarts warning >=3/10m, critical >=6/10m.
- [ ] Redis memory warning >75%, critical >90%; pipeline Redis evictions critical if >0 for 5m.
- [ ] Backup freshness critical if no successful backup in 26h.
