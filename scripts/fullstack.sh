#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="${MOOGLE_PROJECT_NAME:-moogle-stack}"
INFRA_COMPOSE="${ROOT_DIR}/scripts/docker/infra.compose.yml"

SERVICES=(
  spider
  indexer
  image-indexer
  backlinks-processor
  page-rank
  tfidf
)

usage() {
  cat <<EOF
Usage: scripts/fullstack.sh <up|down|logs|reset> [service]

Commands:
  up            Start shared infra (Redis, MongoDB) and pipeline services.
  down          Stop all pipeline services and shared infra.
  logs [name]   Show logs. Use a service name or 'infra'. Without a name, prints recent logs for all.
  reset         Stop stack and remove Redis/Mongo named volumes.

Notes:
  - Each service requires services/<name>/variables.env.
  - Recommended env hosts inside containers on this stack network:
      REDIS_HOST=redis
      MONGO_HOST=mongo (or equivalent mongo connection settings per service)
  - Project name can be overridden with MOOGLE_PROJECT_NAME.
EOF
}

require_docker() {
  command -v docker >/dev/null 2>&1 || {
    echo "docker is required but not found" >&2
    exit 1
  }

  docker compose version >/dev/null 2>&1 || {
    echo "docker compose plugin is required" >&2
    exit 1
  }
}

service_compose() {
  local service="$1"
  echo "${ROOT_DIR}/services/${service}/docker-compose.yml"
}

service_dir() {
  local service="$1"
  echo "${ROOT_DIR}/services/${service}"
}

ensure_env_files() {
  local service
  for service in "${SERVICES[@]}"; do
    local env_file
    env_file="$(service_dir "${service}")/variables.env"
    if [[ ! -f "${env_file}" ]]; then
      echo "Missing required env file: ${env_file}" >&2
      return 1
    fi
  done
}

compose_infra() {
  docker compose -p "${PROJECT_NAME}" -f "${INFRA_COMPOSE}" "$@"
}

compose_service() {
  local service="$1"
  shift
  docker compose -p "${PROJECT_NAME}" -f "$(service_compose "${service}")" --project-directory "$(service_dir "${service}")" "$@"
}

cmd_up() {
  ensure_env_files
  compose_infra up -d redis mongo

  local service
  for service in "${SERVICES[@]}"; do
    compose_service "${service}" up -d --build
  done
}

cmd_down() {
  local service
  for service in "${SERVICES[@]}"; do
    compose_service "${service}" down --remove-orphans || true
  done
  compose_infra down --remove-orphans || true
}

cmd_logs() {
  local target="${1:-all}"
  if [[ "${target}" == "infra" ]]; then
    compose_infra logs -f --tail=200
    return
  fi

  if [[ "${target}" == "all" ]]; then
    compose_infra logs --tail=80
    local service
    for service in "${SERVICES[@]}"; do
      compose_service "${service}" logs --tail=80
    done
    return
  fi

  compose_service "${target}" logs -f --tail=200
}

cmd_reset() {
  cmd_down
  docker volume rm "${PROJECT_NAME}_redis_data" "${PROJECT_NAME}_mongo_data" >/dev/null 2>&1 || true
}

main() {
  require_docker

  local command="${1:-}"
  case "${command}" in
    up)
      cmd_up
      ;;
    down)
      cmd_down
      ;;
    logs)
      cmd_logs "${2:-all}"
      ;;
    reset)
      cmd_reset
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
