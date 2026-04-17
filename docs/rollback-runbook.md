# Rollback Runbook

Use this runbook when a production deployment must be reverted.

## 1) Trigger Conditions

Trigger rollback when one or more conditions persist beyond agreed threshold:

- Search/API error rate spike beyond SLO threshold.
- API p95 latency regression beyond release threshold.
- Worker crash loops or sustained queue backlog growth.
- Smoke tests fail on critical user path.

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

## 4) Rollback Procedure (Generic)

1. Re-deploy previous stable image digest for affected service(s).
2. Verify pods/containers are healthy and ready.
3. Run production smoke tests.
4. Confirm metrics trend returns to baseline.
5. Announce rollback completion.

## 5) Data and Migration Safety

- If DB schema migrations were part of release:
  - Confirm rollback compatibility before service rollback.
  - If not backward compatible, execute DB rollback plan first.
- Never run destructive data rollback without explicit incident commander approval.

## 6) Validation Checklist

- [ ] Health checks green.
- [ ] Smoke tests pass.
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
