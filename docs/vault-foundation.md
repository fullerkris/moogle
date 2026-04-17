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

## Kubernetes Migration Path

- Keep app env names unchanged (`PIPELINE_REDIS_URL`, `MONGODB_URI`, etc.).
- Use External Secrets Operator and a Vault provider initially.
- Later migration to AWS Secrets Manager/Azure Key Vault only changes backend provider mapping; app contracts remain stable.

## Security Notes

- Local config disables TLS (`tls_disable = 1`) for developer bootstrap only.
- Production Vault must run with TLS, KMS auto-unseal, audit logging, and policy least privilege.
