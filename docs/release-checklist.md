# Release Checklist

Use this checklist for every production release.

Note: this is an execution-time checklist. Keep items unchecked in git, and check them during each release window.

References:

- SLO thresholds: `docs/slo-threshold-registry.md`
- Smoke suite: `docs/smoke-suite.md`
- DB migration contract: `docs/db-migration-safety-contract.md`

## Release Metadata

- Release owner:
- Date/time (UTC):
- Version/tag:
- Commit SHA:
- Change window:
- Rollback owner:

## 1) Pre-Release (Must Pass)

- [ ] PR merged through PR-only flow to `main`.
- [ ] Required checks passed:
  - [ ] multi-language tests
  - [ ] build
  - [ ] vulnerability scan
  - [ ] smoke (per `docs/smoke-suite.md`)
- [ ] Images built and tagged with immutable digest.
- [ ] Release notes drafted (scope, risks, rollback target).
- [ ] Staging deploy completed from same artifact(s).
- [ ] Staging smoke tests passed.
- [ ] No open Sev1/Sev2 incident affecting release scope.
- [ ] Backup/restore status confirmed (latest backup successful).

## 2) Pre-Deploy Production Verification

- [ ] Confirm environment is `dev -> staging -> production` promoted artifact, not rebuilt artifact.
- [ ] Confirm secrets are current (<= 180 days old) and not expiring during release window.
- [ ] Secret readiness validation passed: `scripts/ops/validate-secret-readiness.sh prod`.
- [ ] Confirm migration scripts (if any) follow `docs/db-migration-safety-contract.md`.
- [ ] Confirm each migration step has a rollback path or approved mitigation.
- [ ] Confirm on-call engineer is available.
- [ ] Confirm communication channel active (incident/release room).
- [ ] Runtime exposure validation passed per `docs/runtime-exposure-validation.md`.

## 3) Deploy Steps

- [ ] Deploy using approved rollout method (rolling/canary).
- [ ] Monitor first 5 minutes for:
  - [ ] error rate
  - [ ] p95 latency
  - [ ] queue depth/lag
  - [ ] restart loops
- [ ] Advance rollout only if metrics remain within thresholds in `docs/slo-threshold-registry.md`.

## 4) Post-Deploy Validation

- [ ] Production smoke tests passed.
- [ ] Search API returns expected responses.
- [ ] Client health verified from public ingress.
- [ ] No sustained alert firing after 15 minutes.
- [ ] Release marked successful in changelog/ops channel.

## 5) Rollback Criteria (Pre-agreed)

Rollback immediately if any of the following persist beyond 10 minutes:

- [ ] Query API 5xx ratio > 3% (5m window).
- [ ] Query API p95 latency > 800ms (5m window).
- [ ] Queue depth > 50,000 and rising (15m) or oldest message age > 15m.
- [ ] Critical worker restart count >= 6 in 10m.
- [ ] Critical user path fails smoke tests (`docs/smoke-suite.md`).

## 6) Sign-Off

- Platform owner sign-off:
- Application owner sign-off:
- Incident commander/on-call acknowledgment:
