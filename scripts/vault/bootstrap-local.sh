#!/usr/bin/env bash

set -euo pipefail

if ! command -v vault >/dev/null 2>&1; then
  echo "vault CLI is required" >&2
  exit 1
fi

if [[ -z "${VAULT_ADDR:-}" ]]; then
  export VAULT_ADDR="http://127.0.0.1:8200"
fi

if [[ -z "${VAULT_TOKEN:-}" ]]; then
  echo "VAULT_TOKEN is required (use a root/admin token for bootstrap)" >&2
  exit 1
fi

echo "Ensuring KV v2 secrets engine at secret/..."
if vault secrets list -format=json | grep -q '"secret/"'; then
  echo "secret/ engine already enabled"
else
  vault secrets enable -path=secret kv-v2
fi

for env in dev staging prod; do
  vault kv put "secret/moogle/${env}/shared" \
    PIPELINE_REDIS_URL="redis://pipeline-redis.internal:6379/0" \
    QUERY_REDIS_URL="redis://query-redis.internal:6379/0" \
    CACHE_REDIS_URL="redis://query-redis.internal:6379/1" \
    MONGODB_URI="mongodb://moogle:change-me@mongo.internal:27017/moogle?authSource=admin" \
    SECRET_ROTATION_DAYS="180"

  vault kv put "secret/moogle/${env}/query-engine" \
    APP_ENV="${env}" \
    APP_DEBUG="false"
done

echo "Writing read-only policy"
vault policy write moogle-read "infra/vault/policies/moogle-read.hcl"

echo "Vault bootstrap complete"
