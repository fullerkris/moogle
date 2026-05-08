# Runtime Exposure Validation

Use this runbook to prove the remaining network exposure gates before staging or production internet exposure.

## Static Validation

Run this from the repository root before deployment:

```bash
scripts/ops/validate-runtime-exposure.sh static
```

Expected evidence:

- Production compose publishes only Caddy on `80/443`.
- K8s overlays render TLS, cert-manager annotations, HTTPS redirect, ClusterIP services, and Redis NetworkPolicy objects.

## VM Public Validation

Run this after DNS points to the target VM and Caddy is deployed:

```bash
MOOGLE_PUBLIC_HOST=moogle.example.com \
scripts/ops/validate-runtime-exposure.sh vm-public
```

Expected evidence:

- `https://<host>/api/health/live` succeeds.
- `http://<host>/api/health/live` redirects to HTTPS.
- Public ports `80` and `443` are reachable.
- Mongo, Redis, PHP-FPM, worker health, and metrics ports are not publicly reachable.

## Kubernetes Runtime Validation

Run this against the target cluster context after applying the overlay:

```bash
MOOGLE_ENVIRONMENT=prod \
MOOGLE_NAMESPACE=moogle \
scripts/ops/validate-runtime-exposure.sh k8s-runtime
```

Expected evidence:

- `moogle-ingress-prod` exists.
- `letsencrypt-prod` ClusterIssuer exists.
- App services are `ClusterIP` only.
- Redis NetworkPolicy objects exist.

For staging, use:

```bash
MOOGLE_ENVIRONMENT=staging \
MOOGLE_NAMESPACE=moogle \
scripts/ops/validate-runtime-exposure.sh k8s-runtime
```

## Evidence To Attach

- Script output with timestamp.
- Target environment and release SHA.
- DNS record target for `MOOGLE_PUBLIC_HOST`.
- Screenshot or output from cloud firewall/security-group rules showing only `80/443` public ingress.
- If Kubernetes is used, CNI/network-policy support confirmation from the cluster provider.

## Known Limits

- The script cannot prove cloud firewall policy unless it can query the provider account; attach provider evidence manually.
- Kubernetes NetworkPolicy objects only enforce traffic if the cluster CNI supports NetworkPolicy.
- TLS issuance requires real DNS and publicly reachable `80/443` for ACME HTTP-01 unless a DNS-01 issuer is used.
