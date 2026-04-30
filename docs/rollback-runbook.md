# Rollback Runbook

Use this runbook when a production deployment must be reverted.

Note: validation and follow-up checklists are incident-time steps and are expected to remain unchecked in git.

References:

- Thresholds: `docs/slo-threshold-registry.md`
- Smoke suite: `docs/smoke-suite.md`
- DB migration safety: `docs/db-migration-safety-contract.md`

## 1) Trigger Conditions

Trigger rollback when one or more conditions persist beyond threshold:

- Query API 5xx ratio > 3% over 5m.
- Query API p95 latency > 800ms over 5m.
- Queue depth > 50,000 and rising for 15m, or oldest queue message age > 15m.
- Worker restart count >= 6 in 10m for a critical service.
- Smoke tests fail on critical user path (`docs/smoke-suite.md`).

## 2) Roles

- Incident commander:
- Rollback executor:
- Communications owner:
- Observer/scribe:

## 3) Immediate Actions

1. Pause rollout / stop further deployment progression.
2. Declare rollback in incident channel.
3. Confirm last known good version/tag and image digest.
4. Snapshot key telemetry (for postmortem):
   - error rate
   - latency
   - queue depth
   - recent logs

## 4) Rollback Procedure

### 4A) VM (Docker Compose)

1. Confirm last known good release digest/tag from release metadata.
2. Ensure Vault connectivity and production secret export path are healthy.
3. Re-deploy affected services using the previous approved digest set.
4. Verify service/container health and readiness.
5. Run smoke suite (`docs/smoke-suite.md`) and compare metrics to baseline.

### 4B) Kubernetes

1. Confirm last known good release revision/digest.
2. Roll back affected deployment(s) to previous revision, for example:

```bash
scripts/kubectl-with-config.sh <prod-kubeconfig> -n moogle rollout undo deployment/<deployment-name>
```

3. If multiple services are affected, repeat per deployment and monitor rollout status.
4. Verify readiness, run smoke suite (`docs/smoke-suite.md`), and confirm metric recovery.
5. Announce rollback completion.

## 5) Data and Migration Safety

- If DB schema migrations were part of release:
  - Confirm migration phase (`expand` or `contract`) per `docs/db-migration-safety-contract.md`.
  - Confirm rollback compatibility before service rollback.
  - If not backward compatible, execute DB rollback plan first.
- Never run destructive data rollback without explicit incident commander approval.

## 6) Validation Checklist

- [ ] Health checks green.
- [ ] Smoke suite passes (`docs/smoke-suite.md`).
- [ ] Error rate back to baseline.
- [ ] Latency back to baseline.
- [ ] Queue lag stabilizing/decreasing.

## 7) Communications

- [ ] Update status page/internal channel with rollback status.
- [ ] Post final outcome and user impact statement.
- [ ] Open post-incident review ticket with timeline.

## 8) Post-Rollback Follow-Up

- [ ] Freeze re-deploy of failed version until RCA complete.
- [ ] Create corrective action items with owners/dates.
- [ ] Add regression coverage (tests/alerts/runbooks).
