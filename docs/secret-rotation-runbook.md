# Secret Rotation Runbook

Note: the validation checklist is run during each rotation event and should remain unchecked in git between rotations.

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

## Automation and Tracking Expectations

- Secret age is reviewed weekly in the readiness scorecard (`docs/weekly-readiness-scorecard-template.md`).
- Any secret older than 180 days must generate a tracked rotation task.
- Each rotation event must record evidence links in the rotation register.

## Scheduled Rotation Procedure

1. Create replacement credentials/tokens.
2. Write new values into Vault at target path.
3. Restart or reload affected services in staging.
4. Run smoke checks.
5. Promote changes to production.
6. Revoke old credentials.
7. Record rotation timestamp and evidence.
8. Update rotation register entry (path, owner, rotated at, next due, evidence).

## Emergency Rotation Procedure

1. Open incident channel and assign owner.
2. Freeze deploys unrelated to rotation.
3. Replace compromised secret(s) in Vault.
4. Restart impacted services.
5. Validate health/readiness and key business endpoints.
6. Revoke compromised credentials.
7. Publish incident update and follow-up actions.

## Validation Checklist

- [ ] Vault contract validation passes:

```bash
VAULT_ADDR=https://vault.example.com \
VAULT_TOKEN=<read-token> \
scripts/ops/validate-secret-readiness.sh prod
```

- [ ] Affected services healthy (`/api/health/live`).
- [ ] Readiness checks successful (`/api/health/ready`).
- [ ] Authenticated DB/Redis operations succeed.
- [ ] No critical alerts triggered after rollout.
- [ ] Rotation register updated with links to evidence.

## Evidence to Capture

- Vault path(s) updated
- `scripts/ops/validate-secret-readiness.sh <env>` output
- Old credential revocation confirmation
- Service restart timestamps
- Smoke test output
- Incident/release links

## Rotation Register Template

| Secret path | Owner | Rotated at (UTC) | Next due (UTC) | Evidence link |
|---|---|---|---|---|
| `secret/moogle/<env>/<service>` |  |  |  |  |
