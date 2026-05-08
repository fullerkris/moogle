#!/usr/bin/env bash

set -euo pipefail

if ! command -v vault >/dev/null 2>&1; then
  echo "vault CLI is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required" >&2
  exit 1
fi

if [[ -z "${VAULT_ADDR:-}" ]]; then
  export VAULT_ADDR="http://127.0.0.1:8200"
fi

if [[ -z "${VAULT_TOKEN:-}" ]]; then
  echo "VAULT_TOKEN is required (use a root/admin token for bootstrap)" >&2
  exit 1
fi

random_secret() {
  openssl rand -base64 32 | tr -d '\n'
}

random_token() {
  openssl rand -hex 24
}

echo "Ensuring KV v2 secrets engine at secret/..."
if vault secrets list -format=json | grep -q '"secret/"'; then
  echo "secret/ engine already enabled"
else
  vault secrets enable -path=secret kv-v2
fi

for env in dev staging prod; do
  app_key_value="base64:$(random_secret)"
  pipeline_redis_password="$(random_token)"
  mongo_root_password="$(random_token)"
  mongo_query_password="$(random_token)"
  mongo_pipeline_password="$(random_token)"
  rotation_timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  caddy_site_address=":80"

  if [[ "${env}" == "prod" ]]; then
    caddy_site_address="moogle.example.com"
  fi

  vault kv put "secret/moogle/${env}/shared" \
    CADDY_SITE_ADDRESS="${caddy_site_address}" \
    PIPELINE_REDIS_PASSWORD="${pipeline_redis_password}" \
    PIPELINE_REDIS_URL="redis://:${pipeline_redis_password}@pipeline-redis.internal:6379/0" \
    QUERY_REDIS_URL="redis://query-redis.internal:6379/0" \
    CACHE_REDIS_URL="redis://query-redis.internal:6379/1" \
    MONGODB_URI="mongodb://moogle_query:${mongo_query_password}@mongo.internal:27017/moogle?authSource=moogle" \
    MONGODB_DATABASE="moogle" \
    MONGODB_QUERY_USERNAME="moogle_query" \
    MONGODB_QUERY_PASSWORD="${mongo_query_password}" \
    MONGO_INITDB_ROOT_USERNAME="moogle_admin" \
    MONGO_INITDB_ROOT_PASSWORD="${mongo_root_password}" \
    MONGO_HOST="mongo" \
    MONGO_PORT="27017" \
    MONGO_DB="moogle" \
    MONGO_USERNAME="moogle_pipeline" \
    MONGO_PASSWORD="${mongo_pipeline_password}" \
    SPIDER_STARTING_URL="https://en.wikipedia.org/wiki/Web_crawler" \
    SPIDER_HTTP_TIMEOUT_SECONDS="10" \
    SPIDER_HTTP_MAX_BODY_BYTES="2097152" \
    SPIDER_HTTP_USER_AGENT="MoogleSpider/1.0 (+https://github.com/IonelPopJara/search-engine)" \
    SPIDER_METRICS_ENABLED="true" \
    SPIDER_METRICS_ADDR=":2113" \
    TFIDF_OPERATIONS_THRESHOLD="1000" \
    TFIDF_NUM_THREADS="4" \
    SECRET_ROTATION_DAYS="180" \
    SECRET_ROTATION_OWNER="platform-infrastructure" \
    SECRET_ROTATED_AT="${rotation_timestamp}"

  vault kv put "secret/moogle/${env}/query-engine" \
    APP_ENV="${env}" \
    APP_DEBUG="false" \
    APP_KEY="${app_key_value}" \
    APP_URL="https://moogle.example.com"
done

echo "Writing read-only policy"
vault policy write moogle-read "infra/vault/policies/moogle-read.hcl"

echo "Vault bootstrap complete"
