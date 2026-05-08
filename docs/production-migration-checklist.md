# Moogle Production Migration Checklist

This checklist converts the Senior Developer + Backend Architect hardening guidance into concrete, repo-specific execution steps.

Status note: boxes marked `[x]` are implemented in this repository baseline; operational/run-time checks are intentionally left `[ ]` until executed in environment.
Tagging note: unchecked items with `(partial: ...)` have some implementation in-repo but still need completion/validation.

## Approved Migration Defaults

- Platform targets: support both VM deployments (Docker Compose) and Kubernetes deployments.
- Kubernetes packaging: Kustomize (`base` + environment overlays).
- Secret manager baseline: HashiCorp Vault (HA, KMS-backed auto-unseal).
- Secret rotation: every 180 days (plus emergency rotation when needed).
- Redis role contract: `PIPELINE_REDIS_URL` is required for broker/queue workloads; query cache/session must not share that keyspace.
- Reliability targets: keep current starter RPO/RTO/SLO values and tune after production telemetry.
- Release governance: PR-only to `main`, required checks (multi-language tests + build + vulnerability scan + smoke), and environment flow `dev -> staging -> production`.
- Threshold source of truth: `docs/slo-threshold-registry.md`.
- Smoke suite definition: `docs/smoke-suite.md`.
- DB migration safety contract: `docs/db-migration-safety-contract.md`.
- Runtime exposure validation: `docs/runtime-exposure-validation.md`.

## Current Baseline (from this branch)

- Polyglot services: Go (`spider`, `page-rank`), Python (`indexer`, `image-indexer`, `backlinks-processor`, `tfidf`), Laravel (`query-engine`), Vite (`client`), Rust (`monitoring`).
- Two Redis roles are active and should remain explicit:
  - Pipeline Redis (`pipeline-redis.internal`, internal by default; private host publishing requires `deploy/compose/docker-compose.private-redis.yml`)
  - Query-engine Redis (`query-redis.internal`, internal-only)
- **Redis durability tradeoff**: both Redis instances run with `--save ""` and `--appendonly no` (no disk persistence, pure in-memory). Pipeline queue items lost to a crash or OOM restart are unrecoverable without a checkpoint/replay mechanism. This is a deliberate throughput tradeoff; the zero-eviction SLO threshold in `docs/slo-threshold-registry.md` is the operational bound. If unbounded pipeline RPO becomes unacceptable, evaluate AOF persistence on pipeline-redis or implement Epic D3 (checkpoint/replay) first.
- Query engine is served through the ingress/reverse-proxy path; client remains containerized and configurable via `VITE_BACKEND_URL`.

---

## Phase 0 - Must Complete Before Internet Exposure

### 1) Production Runtime Mode Only

- [x] Disable dev workflows in production (`npm run dev`, Vite hot mode, `APP_DEBUG=true`).
- [x] Build frontend assets in image build stage and verify `public/build/manifest.json` exists in final image.
- [x] Set Laravel prod env: `APP_ENV=production`, `APP_DEBUG=false`.

### 2) Network and Access Hardening

- [ ] Expose only ingress/reverse proxy (80/443) to public network. (partial: VM prod compose now publishes only Caddy `80/443` by default; K8s services remain ClusterIP behind Ingress; runtime firewall/LB validation still required.)
- [ ] VM ingress standard: Caddy edge reverse proxy with hardened TLS config. (partial: Caddy exposes `80/443`, persists cert state, and supports automatic HTTPS via `CADDY_SITE_ADDRESS`; production DNS/TLS issuance still needs runtime validation.)
- [ ] Kubernetes ingress standard: NGINX Ingress Controller + cert-manager. (partial: `ingressClassName: nginx`, TLS sections, cert-manager cluster-issuer annotations, and forced HTTPS redirect are configured; cluster issuer/DNS validation still required.)
- [x] Keep MongoDB and Redis internal-only (no public port mappings in prod).
- [x] Add network segmentation so pipeline workers cannot access query-only Redis/cache unless required.

### 3) Secrets and Credentials

- [ ] Move production secrets out of checked-in `.env` files. (partial: production compose now uses env contracts instead of checked-in env files; remaining rollout needs environment-wide validation.)
- [ ] Use Vault as source-of-truth for Mongo/Redis credentials and keys. (partial: Vault bootstrap/export scripts + `run-prod-compose.sh` exist for query + pipeline runtime paths; `scripts/ops/validate-secret-readiness.sh` validates the live Vault contract; environment adoption evidence remains.)
- [x] Separate DB users by role (query read-only vs pipeline write users).
- [ ] Enforce 180-day secret rotation schedule with owner + runbook. (partial: policy, runbook, and `SECRET_ROTATED_AT`/owner validation are implemented; production evidence register still needs live entries.)

### 4) Safety Controls

- [x] Add health/readiness checks to all containers/services. (Compose: all services covered; tfidf upgraded to pymongo ping, page-rank upgraded to bash /dev/tcp Mongo connectivity check. K8s: spider/indexer/image-indexer/backlinks-processor use httpGet probes; tfidf uses exec pymongo ping; page-rank has no probe pending a health endpoint — see A5.)
- [x] Add per-service CPU/memory limits and restart policies. (Compose: all services have CPU/memory/PID caps; tfidf and page-rank restart policy corrected from `on-failure` to `unless-stopped`. K8s: all Deployment manifests include resource requests and limits.)
- [ ] Confirm strict HTTP/database timeouts and bounded retries in each language runtime. (partial: spider HTTP timeout controls exist; cross-service retry/backoff standards are not fully implemented — see Phase 1 Epic D2.)

### 5) Data Protection

- [x] Implement daily Mongo backup job. (Compose: `mongo-backup` service in `docker-compose.prod.yml` using `deploy/compose/scripts/mongo-backup.sh`; K8s: `cronjob-mongo-backup.yaml` + `pvc-mongo-backup.yaml` in `k8s/base/`.)
- [ ] Run and document weekly restore test in non-prod. (runtime: requires live environment with populated backup volume.)

---

## Phase 1 - First 2 Weeks

### 1) CI/CD Quality Gates (Required on Every PR)

- [x] Go: `go test ./...`
- [x] Python: `pytest` (or service test command)
- [x] Laravel: `php artisan test`
- [x] Client: `npm run build`
- [x] Rust: `cargo test`
- [ ] Image vulnerability scan (fail build for critical vulnerabilities). (partial: required workflow enforces Trivy CRITICAL scanning on repository filesystem; image-level scanning is still pending.)

### 2) Queue Integrity and Replayability

- [x] Add idempotency strategy for Redis->Mongo writes (upserts + deterministic keys/content hash).
- [ ] Add retry policy with backoff and max attempts.
- [ ] Add DLQ/quarantine path for poison messages.
- [ ] Store checkpoints/watermarks for replay and recovery.

### 3) Observability Baseline

- [ ] Centralize logs (JSON format with `service`, `trace_id`, `error`, `duration_ms`).
- [ ] Add metrics for queue depth, oldest message age, worker throughput, API latency, error rate. (partial: alert rules are defined; exporter/instrumentation wiring is still needed.)
- [ ] Create first alert set: (partial: API 5xx/latency, queue backlog/message-age, Redis memory, and backup freshness alerts exist; restart-spike and eviction-specific alerts are still pending.)
  - queue depth rising continuously for 15m
  - API p95 latency above threshold
  - service crash loops/restart spikes
  - Redis memory pressure/evictions

---

## Phase 2 - Next 1 to 2 Months

### 1) Availability and Scaling

- [ ] Introduce autoscaling signals by queue lag and message age.
- [ ] Isolate read path from write path contention (query-engine should remain responsive during indexing spikes). (partial: query and pipeline Redis roles are isolated; broader contention protections still need validation.)
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

- [ ] Create environment-specific compose files: (partial: `docker-compose.prod.yml` exists; `docker-compose.dev.yml` is still missing.)
  - `docker-compose.dev.yml`
  - `docker-compose.prod.yml`
- [x] Add K8s Deployment manifests for all pipeline workers. (Added: `deployment-spider.yaml`, `deployment-indexer.yaml`, `deployment-image-indexer.yaml`, `deployment-backlinks-processor.yaml`, `deployment-tfidf.yaml`, `deployment-page-rank.yaml` — all wired into `k8s/base/kustomization.yaml`. Full-stack K8s deployment is now possible. page-rank probes pending a health endpoint — see A5.)
- [x] In prod compose:
  - remove bind mounts for application code
  - disable dev server commands
  - attach healthchecks, health-gated dependencies, and VM-enforced resource constraints

### Environment Variable Clarity

- [x] Replace ambiguous Redis vars with explicit names across pipeline services and runtime contracts.
  - `PIPELINE_REDIS_URL` (required)
  - `QUERY_REDIS_URL` (optional/isolated for query cache-session workloads)
- [x] Add startup validation that fails fast if required vars are missing for pipeline Redis and production compose env contracts.

### Query-Engine Asset Reliability

- [x] Keep Vite build in Docker image build pipeline.
- [x] Add pre-start check to fail fast when `public/build/manifest.json` is missing.

### Client Stability

- [x] Keep `VITE_BACKEND_URL` environment-driven for prod/staging/dev.
- [ ] Ensure production client points to stable API ingress URL, not localhost. (partial: client supports `VITE_BACKEND_URL`; production deployment wiring still needs explicit enforcement.)

---

## Service-by-Service Hardening Notes

### Spider (Go)

- [ ] Enforce crawl budgets and domain rate limits. (partial: domain policy + robots + per-domain politeness controls are implemented; crawl budget quotas/limits still pending.)
- [x] Set bounded queue insertion to prevent broker overload.
- [ ] Add metrics for fetched pages/sec, timeout rate, and enqueue failures.

### Indexer/Image Indexer/Backlinks/TF-IDF (Python)

- [x] Ensure all write paths are idempotent (upsert or unique constraints).
- [ ] Apply backoff with jitter on transient Mongo/Redis errors.
- [ ] Add dead-letter handling for unparseable payloads.
- [x] Replace fragile process-name health probes with functional checks. (tfidf now uses a pymongo ping to verify Mongo connectivity; page-rank now uses bash /dev/tcp TCP check to Mongo port. K8s tfidf uses exec pymongo ping. page-rank K8s probe pending a health endpoint — see A5.)
- [x] Change restart policy from `on-failure` to `unless-stopped` for pipeline workers. (Fixed for tfidf and page-rank in `docker-compose.prod.yml`.)

### Page-Rank (Go)

- [ ] Tag each run with `run_id` and write results atomically.
- [ ] Keep previous rank snapshot until new run is marked complete.

### Query-Engine (Laravel)

- [x] Add `healthz` and dependency-aware `readyz` endpoints (implemented as `/api/health/live` and `/api/health/ready`).
- [ ] Add request rate limiting at edge/API gateway.
- [ ] Use cache key versioning for rank/index updates.

### Monitoring (Rust)

- [ ] Monitoring service is currently non-functional — it is a placeholder not updated for the current architecture. Without it there is no automated service-respawning safety net beyond Docker/K8s restart policies. Decide before production: rewrite to integrate with current service topology, replace with an external health management approach, or formally remove and document the gap.

### Client (Vite)

- [ ] Use production build artifacts for deployment.
- [x] Do not rely on dev server in production.

---

## Go-Live Gates (Measurable)

- [ ] Security gate: no critical runtime vulnerabilities.
- [ ] Reliability gate: end-to-end pipeline success >= 99.5% over 7 days.
- [ ] Performance gate: query API p95 latency within agreed SLO at expected load.
- [ ] Data gate: backup + restore drill completed within RPO/RTO targets.
- [ ] Operations gate: on-call runbook validated by engineer not authoring the change.
- [ ] Governance gate: release checklist, rollback runbook, and incident comms template are present and reviewed. (partial: all three docs are present in `docs/`; formal review/sign-off evidence is pending.)
- [x] Threshold registry documented and linked in governance docs.
- [x] Smoke suite definition + ownership documented.
- [x] DB migration safety contract documented.

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

- [x] API 5xx warning >1%/5m, critical >3%/5m. (documented in `docs/slo-threshold-registry.md`; alert rule parity still tracked in observability work.)
- [x] API latency warning p95 >400ms/10m, critical p95 >800ms/5m. (documented in `docs/slo-threshold-registry.md`; alert rule parity still tracked in observability work.)
- [x] Queue depth warning >10k + rising/15m, critical >50k + rising/15m. (documented in `docs/slo-threshold-registry.md`; alert rule parity still tracked in observability work.)
- [x] Oldest queue message warning >5m, critical >15m. (documented in `docs/slo-threshold-registry.md`; alert rule parity still tracked in observability work.)
- [x] Worker restarts warning >=3/10m, critical >=6/10m. (documented in `docs/slo-threshold-registry.md`; alert rule parity still tracked in observability work.)
- [x] Redis memory warning >75%, critical >90%; pipeline Redis evictions critical if >0 for 5m. (documented in `docs/slo-threshold-registry.md`; alert rule parity still tracked in observability work.)
- [x] Backup freshness critical if no successful backup in 26h. (documented in `docs/slo-threshold-registry.md`; backup job/exporter signal integration still tracked separately.)
