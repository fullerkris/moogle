# Kubernetes Secret Bootstrap (ESO + Vault)

This runbook describes how to bootstrap Vault-backed runtime secrets in Kubernetes using External Secrets Operator (ESO), and how to validate rollout health.

## Scope

- Applies to Kustomize overlays:
  - `k8s/overlays/dev`
  - `k8s/overlays/staging`
  - `k8s/overlays/prod`
- Covers:
  - ESO readiness checks
  - `vault-token` secret creation
  - secret sync validation (`SecretStore`, `ExternalSecret`, target `Secret`)
  - deployment rollout checks

## 1) Prerequisites

1. `kubectl` access to target cluster.
2. `moogle` namespace exists.
3. Vault contains expected keys:
   - `secret/moogle/<env>/shared`
   - `secret/moogle/<env>/query-engine`
4. A Vault token with read access to those paths.

## 2) Verify ESO is installed and healthy

Check that ESO CRDs are available:

```bash
kubectl get crd externalsecrets.external-secrets.io secretstores.external-secrets.io
```

Check operator pods:

```bash
kubectl -n external-secrets get pods
```

Expected: ESO controller/webhook/cert-controller pods are `Running`.

## 3) Create or rotate `vault-token` in `moogle`

Create/update token secret used by `k8s/base/secretstore-vault.yaml`:

```bash
kubectl -n moogle create secret generic vault-token \
  --from-literal=token='<vault-read-token>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

Verify:

```bash
kubectl -n moogle get secret vault-token
```

## 4) Apply target overlay

Choose one environment:

```bash
kubectl apply -k k8s/overlays/dev
# or
kubectl apply -k k8s/overlays/staging
# or
kubectl apply -k k8s/overlays/prod
```

## 5) Validate secret sync

Check `SecretStore` condition:

```bash
kubectl -n moogle get secretstore
kubectl -n moogle describe secretstore vault-backend-<env-suffix>
```

Check `ExternalSecret` condition:

```bash
kubectl -n moogle get externalsecret
kubectl -n moogle describe externalsecret moogle-runtime-secrets-<env-suffix>
```

Check materialized target secret:

```bash
kubectl -n moogle get secret moogle-runtime-secrets
kubectl -n moogle get secret moogle-runtime-secrets -o jsonpath='{.data.PIPELINE_REDIS_URL}'
kubectl -n moogle get secret moogle-runtime-secrets -o jsonpath='{.data.PIPELINE_REDIS_PASSWORD}'
```

Expected: `SecretStore` and `ExternalSecret` are `Ready=True`, and `moogle-runtime-secrets` exists.

## 6) Rollout checks

Deployments consuming runtime secrets:

- `query-engine-<env-suffix>`
- `pipeline-redis-<env-suffix>`
- `runtime-metrics-exporter-<env-suffix>`

Check rollout:

```bash
kubectl -n moogle rollout status deploy/query-engine-<env-suffix>
kubectl -n moogle rollout status deploy/pipeline-redis-<env-suffix>
kubectl -n moogle rollout status deploy/runtime-metrics-exporter-<env-suffix>
```

Check pods and recent events:

```bash
kubectl -n moogle get pods
kubectl -n moogle get events --sort-by=.metadata.creationTimestamp
```

Health verification:

```bash
kubectl -n moogle port-forward svc/query-engine-http-<env-suffix> 8080:80
curl -fsS http://127.0.0.1:8080/api/health/live
curl -fsS http://127.0.0.1:8080/api/health/ready
```

## 7) Troubleshooting

- `ExternalSecret Ready=False`:
  - confirm `vault-token` exists in `moogle` namespace
  - confirm Vault token policy grants access to `secret/moogle/<env>/*`
  - confirm Vault server URL in `SecretStore` is reachable from cluster
- `SecretStore Ready=False`:
  - inspect ESO controller logs:
    - `kubectl -n external-secrets logs deploy/external-secrets`
- rollout failures after secret sync:
  - inspect deployment events and container logs
  - verify required keys exist in Vault paths and map to expected env names

## 8) Evidence capture (release/change record)

Record:

- output of `kubectl -n moogle get secretstore,externalsecret,secret`
- rollout status command output
- `/api/health/live` and `/api/health/ready` responses
- timestamp and operator identity
