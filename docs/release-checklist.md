# Release Checklist

Use this checklist for every production release.

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
  - [ ] smoke
- [ ] Images built and tagged with immutable digest.
- [ ] Release notes drafted (scope, risks, rollback target).
- [ ] Staging deploy completed from same artifact(s).
- [ ] Staging smoke tests passed.
- [ ] No open Sev1/Sev2 incident affecting release scope.
- [ ] Backup/restore status confirmed (latest backup successful).

## 2) Pre-Deploy Production Verification

- [ ] Confirm environment is `dev -> staging -> production` promoted artifact, not rebuilt artifact.
- [ ] Confirm secrets are current (<= 180 days old) and not expiring during release window.
- [ ] Confirm migration scripts (if any) are backward-compatible.
- [ ] Confirm on-call engineer is available.
- [ ] Confirm communication channel active (incident/release room).

## 3) Deploy Steps

- [ ] Deploy using approved rollout method (rolling/canary).
- [ ] Monitor first 5 minutes for:
  - [ ] error rate
  - [ ] p95 latency
  - [ ] queue depth/lag
  - [ ] restart loops
- [ ] Advance rollout only if metrics remain within SLO thresholds.

## 4) Post-Deploy Validation

- [ ] Production smoke tests passed.
- [ ] Search API returns expected responses.
- [ ] Client health verified from public ingress.
- [ ] No sustained alert firing after 15 minutes.
- [ ] Release marked successful in changelog/ops channel.

## 5) Rollback Criteria (Pre-agreed)

Rollback immediately if any of the following persist beyond 10 minutes:

- [ ] Error rate exceeds SLO/error budget burn threshold.
- [ ] API p95 latency exceeds release threshold.
- [ ] Queue backlog grows continuously with consumer degradation.
- [ ] Critical user path fails smoke tests.

## 6) Sign-Off

- Platform owner sign-off:
- Application owner sign-off:
- Incident commander/on-call acknowledgment:
