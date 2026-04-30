# Smoke Suite Definition

Use this suite for every staging deployment, production deployment, and rollback validation.

## Ownership

- Suite owner: Application owner
- Runtime owner: Platform/Infrastructure
- Execution points: post-deploy, post-rollback, secret rotation validation

## Critical User Paths

1. API liveness/readiness
2. Search results render
3. Image search results render
4. Top-search telemetry path
5. Metrics endpoint reachable

## Standard Checks

Run against the target base URL (staging/prod ingress):

```bash
BASE_URL="http://localhost"

curl -fsS "$BASE_URL/api/health/live"
curl -fsS "$BASE_URL/api/health/ready"

curl -fsS "$BASE_URL/api/search?q=moogle" | grep -qi "results\|data\|moogle"
curl -fsS "$BASE_URL/api/images/search?q=moogle" | grep -qi "image\|results\|data"

curl -fsS "$BASE_URL/api/search/top" | grep -qi "top\|queries\|data"
curl -fsS "$BASE_URL/metrics" | grep -qi "# HELP\|# TYPE"
```

## Pass/Fail Criteria

- Pass when all checks return successful status and expected payload shape.
- Fail when any critical path fails twice in a row over a 5 minute window.
- Failures block rollout progression and require rollback criteria evaluation.

## Evidence Required

- Timestamp of run
- Environment and release identifier
- Executor name
- Command output or CI job link
- Follow-up ticket link for any failures
