#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="${MOOGLE_PROJECT_NAME:-moogle-stack}"
INFRA_COMPOSE="${ROOT_DIR}/scripts/docker/infra.compose.yml"

DURATION_SECONDS=60
INTERVAL_SECONDS=5

usage() {
  cat <<EOF
Usage: scripts/benchmark-crawler.sh [--duration <seconds>] [--interval <seconds>]

Samples crawler throughput counters from Redis while the stack is running.

Options:
  --duration <seconds>  Benchmark window length (default: 60)
  --interval <seconds>  Sampling interval (default: 5)
  -h, --help            Show this help text

Requires:
  - Stack already running (for example: scripts/fullstack.sh up)
  - Redis service available in scripts/docker/infra.compose.yml
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

compose_infra() {
  docker compose -p "${PROJECT_NAME}" -f "${INFRA_COMPOSE}" "$@"
}

redis_cli() {
  compose_infra exec -T redis redis-cli "$@" | tr -d '\r'
}

require_running_redis() {
  if ! compose_infra ps --status running --services | grep -q '^redis$'; then
    echo "redis is not running for project ${PROJECT_NAME}. Run scripts/fullstack.sh up first." >&2
    exit 1
  fi
}

is_int() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --duration)
        DURATION_SECONDS="${2:-}"
        shift 2
        ;;
      --interval)
        INTERVAL_SECONDS="${2:-}"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown option: $1" >&2
        usage
        exit 1
        ;;
    esac
  done

  if ! is_int "${DURATION_SECONDS}" || [[ "${DURATION_SECONDS}" -le 0 ]]; then
    echo "--duration must be a positive integer" >&2
    exit 1
  fi

  if ! is_int "${INTERVAL_SECONDS}" || [[ "${INTERVAL_SECONDS}" -le 0 ]]; then
    echo "--interval must be a positive integer" >&2
    exit 1
  fi

  if [[ "${INTERVAL_SECONDS}" -gt "${DURATION_SECONDS}" ]]; then
    echo "--interval must be <= --duration" >&2
    exit 1
  fi
}

main() {
  parse_args "$@"
  require_docker
  require_running_redis

  local start_ts
  local end_ts
  start_ts="$(date +%s)"
  end_ts=$((start_ts + DURATION_SECONDS))

  local start_seen
  local start_visited
  local start_frontier
  local start_indexer

  start_seen="$(redis_cli SCARD spider_seen_urls)"
  start_visited="$(redis_cli SCARD spider_visited_urls)"
  start_frontier="$(redis_cli ZCARD spider_queue)"
  start_indexer="$(redis_cli LLEN pages_queue)"

  echo "Crawler benchmark window: ${DURATION_SECONDS}s (interval: ${INTERVAL_SECONDS}s)"
  echo "Initial counters: seen=${start_seen} visited=${start_visited} frontier=${start_frontier} pages_queue=${start_indexer}"
  echo
  printf '%-8s %-10s %-10s %-10s %-12s\n' "time(s)" "visited" "seen" "frontier" "pages/min"

  while (( "$(date +%s)" < end_ts )); do
    sleep "${INTERVAL_SECONDS}"

    local now
    local elapsed
    local visited
    local seen
    local frontier
    local delta_visited
    local ppm

    now="$(date +%s)"
    elapsed=$((now - start_ts))
    visited="$(redis_cli SCARD spider_visited_urls)"
    seen="$(redis_cli SCARD spider_seen_urls)"
    frontier="$(redis_cli ZCARD spider_queue)"

    delta_visited=$((visited - start_visited))
    ppm="$(awk -v d="${delta_visited}" -v e="${elapsed}" 'BEGIN { if (e <= 0) printf "0.00"; else printf "%.2f", (d * 60.0) / e }')"

    printf '%-8s %-10s %-10s %-10s %-12s\n' "${elapsed}" "${visited}" "${seen}" "${frontier}" "${ppm}"
  done

  local end_seen
  local end_visited
  local end_frontier
  local end_indexer
  local elapsed_total
  local visited_growth
  local average_ppm

  end_seen="$(redis_cli SCARD spider_seen_urls)"
  end_visited="$(redis_cli SCARD spider_visited_urls)"
  end_frontier="$(redis_cli ZCARD spider_queue)"
  end_indexer="$(redis_cli LLEN pages_queue)"

  elapsed_total=$(("$(date +%s)" - start_ts))
  visited_growth=$((end_visited - start_visited))
  average_ppm="$(awk -v d="${visited_growth}" -v e="${elapsed_total}" 'BEGIN { if (e <= 0) printf "0.00"; else printf "%.2f", (d * 60.0) / e }')"

  echo
  echo "Benchmark complete"
  echo "Elapsed: ${elapsed_total}s"
  echo "Visited growth: ${visited_growth}"
  echo "Average crawl rate: ${average_ppm} pages/min"
  echo "Final counters: seen=${end_seen} visited=${end_visited} frontier=${end_frontier} pages_queue=${end_indexer}"
}

main "$@"
