# Secret Rotation Runbook

## Policy

- Scheduled rotation interval: 180 days.
- Emergency rotation trigger examples:
  - suspected credential leak
  - privileged offboarding event
  - incident response directive

## Ownership

- Rotation owner: Platform/Infrastructure team
- Service validation owner: Service owner
- Incident communication owner: Incident commander or release manager

## Scheduled Rotation Procedure

1. Create replacement credentials/tokens.
2. Write new values into Vault at target path.
3. Restart or reload affected services in staging.
4. Run smoke checks.
5. Promote changes to production.
6. Revoke old credentials.
7. Record rotation timestamp and evidence.

## Emergency Rotation Procedure

1. Open incident channel and assign owner.
2. Freeze deploys unrelated to rotation.
3. Replace compromised secret(s) in Vault.
4. Restart impacted services.
5. Validate health/readiness and key business endpoints.
6. Revoke compromised credentials.
7. Publish incident update and follow-up actions.

## Validation Checklist

- [ ] Affected services healthy (`/api/health/live`).
- [ ] Readiness checks successful (`/api/health/ready`).
- [ ] Authenticated DB/Redis operations succeed.
- [ ] No critical alerts triggered after rollout.

## Evidence to Capture

- Vault path(s) updated
- Old credential revocation confirmation
- Service restart timestamps
- Smoke test output
- Incident/release links
