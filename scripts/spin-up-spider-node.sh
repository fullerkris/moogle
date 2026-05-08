#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY_DIR="${ROOT_DIR}/deploy/spider-node"
BASE_COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.yml"
LOCAL_COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.local.yml"
ENV_FILE="${MOOGLE_SPIDER_NODE_ENV_FILE:-${DEPLOY_DIR}/.env}"

MODE="${MOOGLE_SPIDER_NODE_MODE:-local}"

usage() {
  cat <<EOF
Usage: scripts/spin-up-spider-node.sh [local|remote] [up|down|status|logs|config|restart] [compose args...]

Modes:
  local    Run spider-node with bundled local pipeline Redis (default).
  remote   Run only spider-node; PIPELINE_REDIS_URL must point at a private Redis endpoint.

Commands:
  up       Start spider-node in detached mode, building the image if needed.
  down     Stop and remove the spider-node compose stack.
  status   Show compose service status.
  logs     Follow spider-node logs.
  config   Render the effective compose config.
  restart  Restart spider-node.

Environment:
  MOOGLE_SPIDER_NODE_MODE      local or remote (default: local)
  MOOGLE_SPIDER_NODE_ENV_FILE  env file path (default: deploy/spider-node/.env)

Examples:
  scripts/spin-up-spider-node.sh up
  scripts/spin-up-spider-node.sh remote up
  PIPELINE_REDIS_URL=redis://:secret@100.99.200.105:16379/0 scripts/spin-up-spider-node.sh remote up
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "${1:-}" == "help" ]]; then
  usage
  exit 0
fi

if [[ "${1:-}" == "local" || "${1:-}" == "remote" ]]; then
  MODE="$1"
  shift
fi

COMMAND="${1:-up}"
if [[ $# -gt 0 ]]; then
  shift
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

compose_args=(--project-directory "${DEPLOY_DIR}")

if [[ -f "${ENV_FILE}" ]]; then
  compose_args+=(--env-file "${ENV_FILE}")
fi

compose_args+=(-f "${BASE_COMPOSE_FILE}")

case "${MODE}" in
  local)
    export PIPELINE_REDIS_URL="${PIPELINE_REDIS_URL:-redis://pipeline-redis:6379/0}"
    compose_args+=(-f "${LOCAL_COMPOSE_FILE}")
    ;;
  remote)
    ;;
  *)
    echo "Unknown spider-node mode: ${MODE}" >&2
    usage >&2
    exit 2
    ;;
esac

case "${COMMAND}" in
  up)
    docker compose "${compose_args[@]}" up --build -d "$@"
    ;;
  down)
    docker compose "${compose_args[@]}" down "$@"
    ;;
  status|ps)
    docker compose "${compose_args[@]}" ps "$@"
    ;;
  logs)
    docker compose "${compose_args[@]}" logs -f spider-node "$@"
    ;;
  config)
    docker compose "${compose_args[@]}" config "$@"
    ;;
  restart)
    docker compose "${compose_args[@]}" restart spider-node "$@"
    ;;
  *)
    echo "Unknown spider-node command: ${COMMAND}" >&2
    usage >&2
    exit 2
    ;;
esac
