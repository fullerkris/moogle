#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VAULT_COMPOSE_FILE="${ROOT_DIR}/infra/vault/docker-compose.yml"
PROD_COMPOSE_FILE="${ROOT_DIR}/deploy/compose/docker-compose.prod.yml"
VAULT_INIT_FILE="${MOOGLE_VAULT_INIT_FILE:-/tmp/moogle-vault-init.json}"

ENVIRONMENT="${MOOGLE_ENVIRONMENT:-staging}"
VAULT_ADDR_VALUE="${MOOGLE_VAULT_ADDR:-http://127.0.0.1:8200}"

CLIENT_PORT="${MOOGLE_CLIENT_PORT:-5173}"
CLIENT_LOG_FILE="${MOOGLE_CLIENT_LOG:-/tmp/moogle-client.log}"
CLIENT_PID_FILE="${MOOGLE_CLIENT_PID_FILE:-/tmp/moogle-client.pid}"

AUTO_BUILD_IMAGES="${MOOGLE_AUTO_BUILD_IMAGES:-1}"

usage() {
  cat <<EOF
Usage: scripts/spin-up-local.sh [up|down|status]

Commands:
  up       Start Vault, seed/unseal as needed, start full backend stack, and start client dev server.
  down     Stop backend stack, client dev server, and local Vault.
  status   Show Vault, backend compose, and client status.

Optional environment variables:
  MOOGLE_ENVIRONMENT       Vault/compose environment (default: staging)
  MOOGLE_VAULT_ADDR        Vault address (default: http://127.0.0.1:8200)
  MOOGLE_VAULT_INIT_FILE   Vault init json path (default: /tmp/moogle-vault-init.json)
  MOOGLE_CLIENT_PORT       Client dev server port (default: 5173)
  MOOGLE_AUTO_BUILD_IMAGES Build missing local images before compose up (1 or 0, default: 1)
EOF
}

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "Missing required command: ${command_name}" >&2
    exit 1
  fi
}

require_prereqs() {
  require_command docker
  require_command jq
  require_command vault
  require_command curl
  require_command lsof
  require_command npm
  require_command openssl

  if [[ ! -f "${VAULT_INIT_FILE}" ]]; then
    echo "Vault init file not found: ${VAULT_INIT_FILE}" >&2
    echo "Expected a JSON file with root token and unseal key." >&2
    exit 1
  fi
}

vault_status_json() {
  VAULT_ADDR="${VAULT_ADDR_VALUE}" vault status -format=json
}

wait_for_vault_api() {
  local attempts=30
  local i

  for ((i = 1; i <= attempts; i++)); do
    if vault_status_json >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Vault API did not become ready at ${VAULT_ADDR_VALUE}" >&2
  exit 1
}

ensure_vault_running() {
  docker compose -f "${VAULT_COMPOSE_FILE}" up -d
  wait_for_vault_api
}

ensure_vault_unsealed() {
  local sealed
  sealed="$(vault_status_json | jq -r '.sealed')"

  if [[ "${sealed}" == "false" ]]; then
    return 0
  fi

  local unseal_key
  unseal_key="$(jq -r '.unseal_keys_b64[0]' "${VAULT_INIT_FILE}")"

  if [[ -z "${unseal_key}" || "${unseal_key}" == "null" ]]; then
    echo "Unseal key missing in ${VAULT_INIT_FILE}" >&2
    exit 1
  fi

  VAULT_ADDR="${VAULT_ADDR_VALUE}" vault operator unseal "${unseal_key}" >/dev/null
}

get_root_token() {
  jq -r '.root_token' "${VAULT_INIT_FILE}"
}

ensure_vault_seeded() {
  local root_token
  root_token="$(get_root_token)"

  if [[ -z "${root_token}" || "${root_token}" == "null" ]]; then
    echo "Root token missing in ${VAULT_INIT_FILE}" >&2
    exit 1
  fi

  if ! VAULT_ADDR="${VAULT_ADDR_VALUE}" VAULT_TOKEN="${root_token}" vault kv get -format=json "secret/moogle/${ENVIRONMENT}/shared" >/dev/null 2>&1; then
    echo "Seeding local Vault secrets..."
    VAULT_ADDR="${VAULT_ADDR_VALUE}" VAULT_TOKEN="${root_token}" "${ROOT_DIR}/scripts/vault/bootstrap-local.sh"
  fi

  local app_key
  app_key="$(VAULT_ADDR="${VAULT_ADDR_VALUE}" VAULT_TOKEN="${root_token}" vault kv get -field=APP_KEY "secret/moogle/${ENVIRONMENT}/query-engine" 2>/dev/null || true)"
  if [[ -z "${app_key}" || "${app_key}" == "base64:replace-me" ]]; then
    local generated_key
    generated_key="base64:$(openssl rand -base64 32 | tr -d '\n')"
    VAULT_ADDR="${VAULT_ADDR_VALUE}" VAULT_TOKEN="${root_token}" vault kv patch "secret/moogle/${ENVIRONMENT}/query-engine" APP_KEY="${generated_key}" >/dev/null
  fi
}

build_missing_images() {
  if [[ "${AUTO_BUILD_IMAGES}" != "1" ]]; then
    return 0
  fi

  local image_specs=(
    "query-engine-image:latest|${ROOT_DIR}/services/query-engine"
    "ghcr.io/ionelpopjara/moogle/spider:latest|${ROOT_DIR}/services/spider"
    "ghcr.io/ionelpopjara/moogle/indexer:latest|${ROOT_DIR}/services/indexer"
    "ghcr.io/ionelpopjara/moogle/image-indexer:latest|${ROOT_DIR}/services/image-indexer"
    "ghcr.io/ionelpopjara/moogle/backlinks-processor:latest|${ROOT_DIR}/services/backlinks-processor"
    "ghcr.io/ionelpopjara/moogle/tfidf:latest|${ROOT_DIR}/services/tfidf"
    "ghcr.io/ionelpopjara/moogle/page-rank:latest|${ROOT_DIR}/services/page-rank"
  )

  local spec image context
  for spec in "${image_specs[@]}"; do
    image="${spec%%|*}"
    context="${spec#*|}"

    if docker image inspect "${image}" >/dev/null 2>&1; then
      continue
    fi

    echo "Building missing image ${image}..."
    docker build -t "${image}" "${context}"
  done
}

start_backend_stack() {
  run_prod_compose up -d
}

run_prod_compose() {
  local root_token
  root_token="$(get_root_token)"

  VAULT_ADDR="${VAULT_ADDR_VALUE}" \
  VAULT_TOKEN="${root_token}" \
  "${ROOT_DIR}/scripts/vault/run-prod-compose.sh" "${ENVIRONMENT}" "$@"
}

wait_for_backend_ready() {
  local attempts=90
  local i

  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "http://localhost/api/health/ready" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "Backend did not report ready on http://localhost/api/health/ready" >&2
  return 1
}

client_running() {
  lsof -nP -iTCP:"${CLIENT_PORT}" -sTCP:LISTEN >/dev/null 2>&1
}

start_client() {
  if client_running; then
    echo "Client already listening on port ${CLIENT_PORT}."
    return 0
  fi

  local client_dir
  client_dir="${ROOT_DIR}/services/client"

  if [[ ! -d "${client_dir}/node_modules" ]]; then
    echo "Installing client dependencies..."
    npm install --prefix "${client_dir}"
  fi

  echo "Starting client dev server on port ${CLIENT_PORT}..."
  (
    cd "${client_dir}"
    nohup npm run dev -- --host 0.0.0.0 --port "${CLIENT_PORT}" >"${CLIENT_LOG_FILE}" 2>&1 &
    echo $! > "${CLIENT_PID_FILE}"
  )

  local attempts=30
  local i
  for ((i = 1; i <= attempts; i++)); do
    if client_running; then
      return 0
    fi
    sleep 1
  done

  echo "Client did not start. Check logs: ${CLIENT_LOG_FILE}" >&2
  return 1
}

stop_client() {
  if [[ -f "${CLIENT_PID_FILE}" ]]; then
    local pid
    pid="$(<"${CLIENT_PID_FILE}")"
    if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
      kill "${pid}" >/dev/null 2>&1 || true
    fi
    rm -f "${CLIENT_PID_FILE}"
  fi

  if client_running; then
    local pids
    pids="$(lsof -tiTCP:"${CLIENT_PORT}" -sTCP:LISTEN || true)"
    if [[ -n "${pids}" ]]; then
      kill ${pids} >/dev/null 2>&1 || true
    fi
  fi
}

show_status() {
  local vault_ok="false"

  echo "Vault status:"
  if vault_status_json >/dev/null 2>&1; then
    vault_ok="true"
    vault_status_json | jq '{initialized, sealed, version, storage_type}'
  else
    echo "Vault API unreachable at ${VAULT_ADDR_VALUE}"
  fi

  echo
  echo "Backend stack status:"
  if [[ "${vault_ok}" == "true" ]] && [[ "$(vault_status_json | jq -r '.sealed')" == "false" ]]; then
    run_prod_compose ps || true
  else
    echo "Unavailable (Vault is unreachable or sealed)."
  fi

  echo
  echo "Client status:"
  if client_running; then
    lsof -nP -iTCP:"${CLIENT_PORT}" -sTCP:LISTEN
  else
    echo "No process listening on ${CLIENT_PORT}"
  fi
}

cmd_up() {
  require_prereqs
  ensure_vault_running
  ensure_vault_unsealed
  ensure_vault_seeded
  build_missing_images
  start_backend_stack
  wait_for_backend_ready
  start_client

  echo
  echo "Moogle local stack is up."
  echo "- Backend: http://localhost"
  echo "- API ready: http://localhost/api/health/ready"
  echo "- Metrics: http://localhost/metrics"
  echo "- Client: http://localhost:${CLIENT_PORT}"
  echo "- Client log: ${CLIENT_LOG_FILE}"
}

cmd_down() {
  require_prereqs
  ensure_vault_running
  ensure_vault_unsealed
  ensure_vault_seeded
  stop_client
  run_prod_compose down --remove-orphans
  docker compose -f "${VAULT_COMPOSE_FILE}" down --remove-orphans
  echo "Moogle local stack is down."
}

main() {
  local command="${1:-up}"
  case "${command}" in
    up)
      cmd_up
      ;;
    down)
      cmd_down
      ;;
    status)
      require_prereqs
      show_status
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
