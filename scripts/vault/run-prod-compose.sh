#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EXPORT_SCRIPT="${ROOT_DIR}/scripts/vault/export-service-env.sh"
COMPOSE_FILE="${ROOT_DIR}/deploy/compose/docker-compose.prod.yml"

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <env> <compose args...>" >&2
  echo "Example: $0 staging up -d" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

ENVIRONMENT="$1"
shift

if [[ ! -x "${EXPORT_SCRIPT}" ]]; then
  echo "Missing executable export script: ${EXPORT_SCRIPT}" >&2
  exit 1
fi

eval "$("${EXPORT_SCRIPT}" "${ENVIRONMENT}" query-engine)"

: "${APP_KEY:?APP_KEY must be present in Vault for query-engine service}"
: "${APP_URL:?APP_URL must be present in Vault for query-engine service}"
: "${QUERY_REDIS_URL:?QUERY_REDIS_URL must be present in Vault shared secrets}"
: "${CACHE_REDIS_URL:?CACHE_REDIS_URL must be present in Vault shared secrets}"
: "${PIPELINE_REDIS_URL:?PIPELINE_REDIS_URL must be present in Vault shared secrets}"
: "${PIPELINE_REDIS_PASSWORD:?PIPELINE_REDIS_PASSWORD must be present in Vault shared secrets}"
: "${MONGODB_URI:?MONGODB_URI must be present in Vault shared secrets}"
: "${MONGODB_DATABASE:?MONGODB_DATABASE must be present in Vault shared secrets}"
: "${MONGODB_QUERY_USERNAME:?MONGODB_QUERY_USERNAME must be present in Vault shared secrets}"
: "${MONGODB_QUERY_PASSWORD:?MONGODB_QUERY_PASSWORD must be present in Vault shared secrets}"
: "${MONGO_INITDB_ROOT_USERNAME:?MONGO_INITDB_ROOT_USERNAME must be present in Vault shared secrets}"
: "${MONGO_INITDB_ROOT_PASSWORD:?MONGO_INITDB_ROOT_PASSWORD must be present in Vault shared secrets}"

: "${MONGO_HOST:=mongo}"
: "${MONGO_PORT:=27017}"
: "${MONGO_DB:=moogle}"
: "${MONGO_USERNAME:?MONGO_USERNAME must be present in Vault shared secrets}"
: "${MONGO_PASSWORD:?MONGO_PASSWORD must be present in Vault shared secrets}"

: "${SPIDER_STARTING_URL:=https://en.wikipedia.org/wiki/Web_crawler}"
: "${SPIDER_HTTP_TIMEOUT_SECONDS:=10}"
: "${SPIDER_HTTP_MAX_BODY_BYTES:=2097152}"
: "${SPIDER_HTTP_USER_AGENT:=MoogleSpider/1.0 (+https://github.com/IonelPopJara/search-engine)}"
: "${SPIDER_METRICS_ENABLED:=true}"
: "${SPIDER_METRICS_ADDR:=:2113}"

: "${TFIDF_OPERATIONS_THRESHOLD:=1000}"
: "${TFIDF_NUM_THREADS:=4}"

docker compose -f "${COMPOSE_FILE}" "$@"
