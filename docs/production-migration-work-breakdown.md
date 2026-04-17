# Production Migration Work Breakdown

Use this document to convert the migration checklist into implementation tickets with clear sequencing and acceptance criteria.

## Locked Decisions (Approved)

These are now baseline constraints for implementation.

1. **Platform targets**
   - Must run on both:
     - VM deployments (Docker Compose)
     - Kubernetes deployments
   - Implementation rule: app behavior must be platform-agnostic; only deployment manifests differ.

2. **Ingress/DNS/TLS model**
   - Public ingress only on 80/443.
   - API and client should use stable ingress hostnames (not localhost in production).
   - TLS termination at ingress with HTTP->HTTPS redirect.
   - Recommended ingress stack:
     - VMs: Caddy as edge reverse proxy (keep current stack, harden config)
     - Kubernetes: NGINX Ingress Controller + cert-manager

3. **Secret rotation policy**
   - Rotate production secrets every 6 months (180 days).
   - Immediate out-of-band rotation required after any suspected exposure.
   - Secret manager baseline: HashiCorp Vault with KMS-backed auto-unseal.

4. **Redis formal role**
   - `PIPELINE_REDIS_URL` is mandatory for crawl/index queues and broker workloads.
   - Queue/broker keys must never be mixed with query cache/session keys.
   - If a second Redis is used for query cache/session, it must be explicitly configured and isolated.

5. **Initial reliability targets**
   - RPO/RTO/SLO starter values from migration plan are accepted for initial rollout and can be tuned later.

6. **Release governance model**
   - Main branch policy: PR-only to `main`.
   - Required checks: multi-language tests + build + vulnerability scan + smoke.
   - Environment flow: `dev -> staging -> production`.
   - Required governance docs: release checklist, rollback runbook, incident comms template.

7. **Kubernetes packaging**
   - Packaging standard: Kustomize (base + overlays).

## Security and Portability Strategy (Vault -> Cloud Native)

### Vault deployment standard (initial)

- Vault HA with integrated Raft storage.
- Auto-unseal via cloud KMS (AWS KMS/Azure Key Vault Managed HSM key).
- Enforce mTLS for clients and short-lived tokens.
- Prefer dynamic credentials where possible (DB creds), otherwise versioned static secrets with rotation metadata.

### Portability design for future cloud-native migration

- Keep app-level secret contract stable via environment variables.
- Introduce a thin secret sync layer per platform:
  - Kubernetes: External Secrets Operator with provider abstraction.
  - VMs: Vault Agent templates or sidecar/env injector.
- Store secret paths with consistent naming (e.g., `moogle/<env>/<service>/<secret>`).
- When migrating to AWS/Azure native stores, only the provider backend changes; app env names remain unchanged.

---

## Initial Numeric Alert Thresholds (Starter Values)

Tune after 2-4 weeks of production telemetry.

### API / Query Engine

- 5xx error rate:
  - Warning: > 1% for 5 minutes
  - Critical: > 3% for 5 minutes
- Latency:
  - Warning: p95 > 400 ms for 10 minutes
  - Critical: p95 > 800 ms for 5 minutes

### Pipeline / Queues

- `pages_queue` (or equivalent backlog queue) depth:
  - Warning: > 10,000 and still increasing for 15 minutes
  - Critical: > 50,000 and still increasing for 15 minutes
- Oldest queue message age:
  - Warning: > 5 minutes
  - Critical: > 15 minutes

### Workers

- Container restarts per service instance:
  - Warning: >= 3 restarts in 10 minutes
  - Critical: >= 6 restarts in 10 minutes

### Redis

- Memory utilization:
  - Warning: > 75%
  - Critical: > 90%
- Evictions:
  - Critical: any eviction on pipeline Redis for 5 minutes

### MongoDB

- Replication lag (if replica set enabled):
  - Warning: > 10 seconds
  - Critical: > 30 seconds
- Connection failure rate:
  - Warning: > 1% for 5 minutes

### Operational Controls

- Backup freshness:
  - Critical: no successful backup in 26 hours
- Secret age:
  - Warning: >= 170 days since last rotation
  - Critical: >= 180 days since last rotation

## Pre-Implementation Decisions (Should Be Locked First)

These are the highest-value decisions to finalize before coding heavily.

1. **Target runtime platform**
   - Decided: support both Docker Compose on VMs and Kubernetes.
   - Why it matters: changes networking, secrets strategy, autoscaling, and health probe shape.

2. **Ingress and DNS model**
   - Decide: Caddy/Nginx/API Gateway + TLS termination point.
   - Why it matters: determines client `VITE_BACKEND_URL`, CORS policy, and rate limiting.

3. **Secrets manager choice**
   - Decide platform implementation (Vault vs cloud-native secret manager), but keep 180-day rotation policy fixed.
   - Why it matters: unblock moving Mongo/Redis creds and API secrets out of `.env`.

4. **Redis role contract**
   - Decided: `PIPELINE_REDIS_URL` is the required broker role.
   - If query cache/session Redis is enabled, it must be isolated and explicitly named.
   - Why it matters: avoids accidental keyspace overlap and config mistakes.

5. **Backup/RTO/RPO targets**
   - Decide hard targets for Mongo restore time and acceptable data loss.
   - Why it matters: backup frequency and infra cost depend on this.

6. **SLOs for query and indexing freshness**
   - Decide p95 latency target and crawl->search availability target.
   - Why it matters: drives scaling policies and alert thresholds.

7. **Release strategy**
   - Decide staging gate + production promotion + rollback policy.
   - Why it matters: determines CI/CD design and branch/release process.

8. **Security baseline policy**
   - Decide minimum image scanning/SBOM/signing standards.
   - Why it matters: needed before first production release candidate.

---

## Suggested GitHub Epics and Tickets

Each ticket includes clear Definition of Done (DoD) so implementation can start immediately.

## Epic A - Runtime and Deployment Hardening

### A1. Create production compose profile
- **DoD**
  - `docker-compose.prod.yml` exists.
  - No code bind mounts in prod profile.
  - Internal services are not host-exposed.

### A2. Add service health/readiness checks
- **DoD**
  - Every service has a healthcheck.
  - Query-engine has `healthz` and `readyz` endpoint behavior documented.

### A3. Set resource and restart policies
- **DoD**
  - CPU/memory limits defined for each service.
  - Restart behavior defined and tested.

## Epic B - Secrets and Configuration Safety

### B1. Externalize secrets
- **DoD**
  - Production credentials removed from repo-managed env files.
  - Secret injection path documented and validated in staging.

### B2. Standardize Redis env naming
- **DoD**
  - `PIPELINE_REDIS_URL` adopted across pipeline services.
  - Query cache/session Redis (if enabled) is explicitly configured and not mixed with pipeline keyspace.
  - Startup validation fails fast if missing/misconfigured.

### B3. Enforce production env defaults
- **DoD**
  - `APP_ENV=production`, `APP_DEBUG=false` enforced in prod.
  - No dev server command in production startup path.

## Epic C - Build and Supply Chain

### C1. Ensure Laravel asset build is immutable
- **DoD**
  - `public/build/manifest.json` is produced during image build.
  - Runtime no longer depends on ad-hoc `npm run build` fixes.

### C2. Add vulnerability scanning in CI
- **DoD**
  - CI fails on critical vulnerabilities.
  - Scan reports attached to build artifacts.

### C3. Pin base image digests
- **DoD**
  - Service Dockerfiles pin digest for production images.
  - Update process documented.

## Epic D - Pipeline Integrity and Recovery

### D1. Idempotency for Redis->Mongo writes
- **DoD**
  - Upsert/unique-key strategy implemented for core write paths.
  - Duplicate reprocessing does not corrupt data.

### D2. Retry + DLQ strategy
- **DoD**
  - Max retry with exponential backoff configured.
  - Poison messages routed to DLQ with diagnostic payload.

### D3. Checkpoint and replay utility
- **DoD**
  - Watermark/checkpoint persisted per pipeline stage.
  - Replay script can reprocess selected window/domain.

## Epic E - Observability and Operations

### E1. Structured logging baseline
- **DoD**
  - Core services emit structured logs with service name and error context.

### E2. Metrics and alerts baseline
- **DoD**
  - Dashboards track queue depth, lag, API latency, error rates.
  - Alerts configured for sustained backlog growth and API degradation.

### E3. Backup and restore drill
- **DoD**
  - Daily backups automated.
  - Documented restore drill succeeds in staging.

## Epic F - Release Governance

### F1. CI quality gate parity across languages
- **DoD**
  - Go/Python/PHP/JS/Rust test jobs run in CI.
  - Build, vulnerability scan, and smoke checks are mandatory.
  - Required checks block merges when failing.

### F2. Staging smoke suite + rollback runbook
- **DoD**
  - Smoke tests run on every staging deploy.
  - Rollback steps tested and documented.

### F3. Governance documentation pack
- **DoD**
  - `docs/release-checklist.md` exists and is usable by release manager.
  - `docs/rollback-runbook.md` exists with validated rollback steps.
  - `docs/incident-comms-template.md` exists for incident updates.

---

## Dependency Order (Recommended)

1. **A + B** first (safe runtime + safe config)
2. **C** second (repeatable secure builds)
3. **D** third (data correctness and replay)
4. **E** in parallel with D (visibility while hardening)
5. **F** final gate to control releases

---

## Implementation Readiness Check

Start implementation once the team has named owners for:

- Platform/infra ownership
- Data pipeline ownership
- Query-engine ownership
- CI/CD ownership
- Security/compliance ownership

Without clear owners, migration work tends to stall between infra and app teams.
