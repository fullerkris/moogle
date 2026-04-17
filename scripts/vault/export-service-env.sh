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
  echo "VAULT_TOKEN is required" >&2
  exit 1
fi

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 <env> <service>" >&2
  echo "Example: $0 staging query-engine" >&2
  exit 1
fi

ENVIRONMENT="$1"
SERVICE="$2"

shared_path="secret/moogle/${ENVIRONMENT}/shared"
service_path="secret/moogle/${ENVIRONMENT}/${SERVICE}"

echo "# Shared secrets from ${shared_path}"
vault kv get -format=json "${shared_path}" \
  | jq -r '.data.data | to_entries[] | "export \(.key)=\(.value|@sh)"'

echo
echo "# Service secrets from ${service_path}"
vault kv get -format=json "${service_path}" \
  | jq -r '.data.data | to_entries[] | "export \(.key)=\(.value|@sh)"'
