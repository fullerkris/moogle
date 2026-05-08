#!/usr/bin/env bash

set -euo pipefail

ENVIRONMENT="${1:-${MOOGLE_ENVIRONMENT:-staging}}"
ROTATION_WARNING_DAYS="${MOOGLE_SECRET_ROTATION_WARNING_DAYS:-170}"

if [[ -z "${VAULT_ADDR:-}" ]]; then
  export VAULT_ADDR="http://127.0.0.1:8200"
fi

usage() {
  cat <<EOF
Usage: scripts/ops/validate-secret-readiness.sh [env]

Environment:
  VAULT_ADDR                         Vault address (default: http://127.0.0.1:8200)
  VAULT_TOKEN                        Vault token with read access to secret/moogle/<env>/*
  MOOGLE_SECRET_ROTATION_WARNING_DAYS Warning threshold before 180-day expiry (default: 170)
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "${1:-}" == "help" ]]; then
  usage
  exit 0
fi

if [[ -z "${VAULT_TOKEN:-}" ]]; then
  echo "VAULT_TOKEN is required" >&2
  exit 1
fi

for command_name in vault jq; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "${command_name} is required" >&2
    exit 1
  fi
done

failed=0

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s\n' "$1" >&2
  failed=1
}

warn() {
  printf 'WARN %s\n' "$1"
}

iso_to_epoch() {
  local value="$1"

  if date -u -d "${value}" +%s >/dev/null 2>&1; then
    date -u -d "${value}" +%s
    return 0
  fi

  if date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "${value}" +%s >/dev/null 2>&1; then
    date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "${value}" +%s
    return 0
  fi

  return 1
}

get_secret_json() {
  local path="$1"
  vault kv get -format=json "${path}" | jq '.data.data'
}

shared_path="secret/moogle/${ENVIRONMENT}/shared"
query_path="secret/moogle/${ENVIRONMENT}/query-engine"

shared_json="$(get_secret_json "${shared_path}")"
query_json="$(get_secret_json "${query_path}")"

required_shared_keys=(
  CADDY_SITE_ADDRESS
  PIPELINE_REDIS_PASSWORD
  PIPELINE_REDIS_URL
  QUERY_REDIS_URL
  CACHE_REDIS_URL
  MONGODB_URI
  MONGODB_DATABASE
  MONGODB_QUERY_USERNAME
  MONGODB_QUERY_PASSWORD
  MONGO_INITDB_ROOT_USERNAME
  MONGO_INITDB_ROOT_PASSWORD
  MONGO_HOST
  MONGO_PORT
  MONGO_DB
  MONGO_USERNAME
  MONGO_PASSWORD
  SECRET_ROTATION_DAYS
  SECRET_ROTATION_OWNER
  SECRET_ROTATED_AT
)

required_query_keys=(
  APP_ENV
  APP_DEBUG
  APP_KEY
  APP_URL
)

for key in "${required_shared_keys[@]}"; do
  if jq -e --arg key "${key}" 'has($key) and (.[$key] | tostring | length > 0)' <<<"${shared_json}" >/dev/null; then
    pass "${shared_path} has ${key}"
  else
    fail "${shared_path} missing ${key}"
  fi
done

for key in "${required_query_keys[@]}"; do
  if jq -e --arg key "${key}" 'has($key) and (.[$key] | tostring | length > 0)' <<<"${query_json}" >/dev/null; then
    pass "${query_path} has ${key}"
  else
    fail "${query_path} missing ${key}"
  fi
done

placeholder_matches="$(jq -r '[to_entries[] | select((.value | tostring) | test("replace-me|change-me|replace-with|base64:replace"; "i")) | .key] | join(",")' <<<"${shared_json}")"
if [[ -z "${placeholder_matches}" ]]; then
  pass "shared secrets do not use known placeholder values"
else
  fail "shared secrets contain placeholder values: ${placeholder_matches}"
fi

query_placeholder_matches="$(jq -r '[to_entries[] | select((.value | tostring) | test("replace-me|change-me|replace-with|base64:replace"; "i")) | .key] | join(",")' <<<"${query_json}")"
if [[ -z "${query_placeholder_matches}" ]]; then
  pass "query-engine secrets do not use known placeholder values"
else
  fail "query-engine secrets contain placeholder values: ${query_placeholder_matches}"
fi

query_user="$(jq -r '.MONGODB_QUERY_USERNAME' <<<"${shared_json}")"
pipeline_user="$(jq -r '.MONGO_USERNAME' <<<"${shared_json}")"
root_user="$(jq -r '.MONGO_INITDB_ROOT_USERNAME' <<<"${shared_json}")"
mongodb_uri="$(jq -r '.MONGODB_URI' <<<"${shared_json}")"
pipeline_redis_password="$(jq -r '.PIPELINE_REDIS_PASSWORD' <<<"${shared_json}")"
pipeline_redis_url="$(jq -r '.PIPELINE_REDIS_URL' <<<"${shared_json}")"

if [[ "${query_user}" != "${pipeline_user}" && "${query_user}" != "${root_user}" && "${pipeline_user}" != "${root_user}" ]]; then
  pass "Mongo query, pipeline, and root users are role-separated"
else
  fail "Mongo users are not role-separated"
fi

if [[ "${mongodb_uri}" == *"${query_user}"* && "${mongodb_uri}" == *"authSource="* && "${mongodb_uri}" != *"authSource=admin"* ]]; then
  pass "query-engine Mongo URI uses query user outside admin authSource"
else
  fail "query-engine Mongo URI does not appear to use query read-only user/authSource"
fi

if [[ "${pipeline_redis_url}" == *":${pipeline_redis_password}@"* ]]; then
  pass "pipeline Redis URL includes the Vault-managed password"
else
  fail "pipeline Redis URL does not include PIPELINE_REDIS_PASSWORD"
fi

rotation_days="$(jq -r '.SECRET_ROTATION_DAYS' <<<"${shared_json}")"
rotated_at="$(jq -r '.SECRET_ROTATED_AT' <<<"${shared_json}")"
rotation_owner="$(jq -r '.SECRET_ROTATION_OWNER' <<<"${shared_json}")"

if [[ "${rotation_days}" =~ ^[0-9]+$ && "${rotation_days}" -le 180 ]]; then
  pass "rotation interval is ${rotation_days} days"
else
  fail "rotation interval must be numeric and <= 180 days"
  rotation_days="180"
fi

if [[ -n "${rotation_owner}" && "${rotation_owner}" != "null" ]]; then
  pass "rotation owner is set"
else
  fail "rotation owner is missing"
fi

if rotated_epoch="$(iso_to_epoch "${rotated_at}")"; then
  now_epoch="$(date -u +%s)"
  age_days="$(((now_epoch - rotated_epoch) / 86400))"

  if [[ "${age_days}" -lt 0 ]]; then
    fail "SECRET_ROTATED_AT is in the future"
  elif [[ "${age_days}" -ge "${rotation_days}" ]]; then
    fail "secret age is ${age_days} days, exceeding ${rotation_days}-day rotation policy"
  elif [[ "${age_days}" -ge "${ROTATION_WARNING_DAYS}" ]]; then
    warn "secret age is ${age_days} days; rotation warning threshold is ${ROTATION_WARNING_DAYS} days"
  else
    pass "secret age is ${age_days} days"
  fi
else
  fail "SECRET_ROTATED_AT must be UTC ISO-8601, e.g. 2026-01-01T00:00:00Z"
fi

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi
