# Vault Foundation

This document defines the first production-grade Vault foundation for Moogle.

## Scope

- Vault is the source of truth for runtime secrets.
- Rotation policy baseline: every 180 days.
- Emergency rotation: immediate, outside schedule.
- App services consume secrets via environment variables (stable app contract).

## Local Bootstrap

1. Start local Vault:

```bash
docker compose -f infra/vault/docker-compose.yml up -d
```

2. Export Vault connection values:

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=<bootstrap-token>
```

3. Bootstrap KV paths and policy:

```bash
./scripts/vault/bootstrap-local.sh
```

## Secret Path Contract

- Shared environment secrets:
  - `secret/moogle/<env>/shared`
- Service-scoped secrets:
  - `secret/moogle/<env>/<service>`

Example:

- `secret/moogle/staging/shared`
- `secret/moogle/staging/query-engine`

## Export Pattern (for VM workflows)

To export env vars from Vault:

```bash
./scripts/vault/export-service-env.sh staging query-engine
```

Pipe to shell if needed:

```bash
eval "$(./scripts/vault/export-service-env.sh staging query-engine)"
```

Run production compose using Vault-injected secrets:

```bash
./scripts/vault/run-prod-compose.sh staging up -d
```

This wrapper loads shared + service secrets and runs `deploy/compose/docker-compose.prod.yml` (query-engine + pipeline services) without relying on checked-in `.env` files.

The local bootstrap seeds shared Redis/Mongo URLs using `*.internal` hostnames, and production compose exposes matching network aliases so VM rollout uses the same secret contract.

## Kubernetes Migration Path

- Keep app env names unchanged (`PIPELINE_REDIS_URL`, `MONGODB_URI`, etc.).
- Use External Secrets Operator and a Vault provider initially.
- Later migration to AWS Secrets Manager/Azure Key Vault only changes backend provider mapping; app contracts remain stable.

### Kubernetes wiring in this repo

- Secret store and runtime secret manifests:
  - `k8s/base/secretstore-vault.yaml`
  - `k8s/base/externalsecret-runtime-secrets.yaml`
- Query-engine and runtime-metrics deployments consume `moogle-runtime-secrets` via `envFrom.secretRef`.
- Overlay-specific Vault paths are defined in:
  - `k8s/overlays/dev/kustomization.yaml`
  - `k8s/overlays/staging/kustomization.yaml`
  - `k8s/overlays/prod/kustomization.yaml`

Before applying overlays, create a namespaced Vault token secret used by the SecretStore auth:

```bash
kubectl -n moogle create secret generic vault-token --from-literal=token='<vault-read-token>'
```

## Security Notes

- Local config disables TLS (`tls_disable = 1`) for developer bootstrap only.
- Production Vault must run with TLS, KMS auto-unseal, audit logging, and policy least privilege.
- Required secret contract for production compose includes:
  - `APP_KEY`, `APP_URL`
  - `PIPELINE_REDIS_URL`, `QUERY_REDIS_URL`, `CACHE_REDIS_URL`
  - `MONGODB_URI`, `MONGODB_DATABASE`
  - `MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD`
  - `MONGO_HOST`, `MONGO_PORT`, `MONGO_DB`, `MONGO_USERNAME`, `MONGO_PASSWORD`
  - `SPIDER_STARTING_URL`, `SPIDER_HTTP_TIMEOUT_SECONDS`, `SPIDER_HTTP_MAX_BODY_BYTES`, `SPIDER_HTTP_USER_AGENT`
  - `TFIDF_OPERATIONS_THRESHOLD`, `TFIDF_NUM_THREADS`
