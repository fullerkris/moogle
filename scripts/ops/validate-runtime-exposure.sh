#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/deploy/compose/docker-compose.prod.yml"

ENVIRONMENT="${MOOGLE_ENVIRONMENT:-prod}"
NAMESPACE="${MOOGLE_NAMESPACE:-moogle}"
PUBLIC_HOST="${MOOGLE_PUBLIC_HOST:-}"
BASE_URL="${MOOGLE_BASE_URL:-}"

if [[ -z "${BASE_URL}" && -n "${PUBLIC_HOST}" ]]; then
  BASE_URL="https://${PUBLIC_HOST}"
fi

case "${ENVIRONMENT}" in
  prod|production)
    OVERLAY="${MOOGLE_K8S_OVERLAY:-${ROOT_DIR}/k8s/overlays/prod}"
    K8S_SUFFIX="${MOOGLE_K8S_SUFFIX:--prod}"
    CLUSTER_ISSUER="${MOOGLE_CLUSTER_ISSUER:-letsencrypt-prod}"
    ;;
  staging)
    OVERLAY="${MOOGLE_K8S_OVERLAY:-${ROOT_DIR}/k8s/overlays/staging}"
    K8S_SUFFIX="${MOOGLE_K8S_SUFFIX:--staging}"
    CLUSTER_ISSUER="${MOOGLE_CLUSTER_ISSUER:-letsencrypt-staging}"
    ;;
  *)
    OVERLAY="${MOOGLE_K8S_OVERLAY:-${ROOT_DIR}/k8s/overlays/${ENVIRONMENT}}"
    K8S_SUFFIX="${MOOGLE_K8S_SUFFIX:-}"
    CLUSTER_ISSUER="${MOOGLE_CLUSTER_ISSUER:-}"
    ;;
esac

PUBLIC_FORBIDDEN_PORTS="${MOOGLE_PUBLIC_FORBIDDEN_PORTS:-27017 6379 16379 9000 2113 2114 2115 2116 9108}"

failed=0
skipped=0

usage() {
  cat <<EOF
Usage: scripts/ops/validate-runtime-exposure.sh [static|vm-public|k8s-runtime|all]

Modes:
  static       Validate checked-in Compose and Kustomize exposure config.
  vm-public    Validate public DNS/TLS and forbidden public ports for a VM target.
  k8s-runtime  Validate live Kubernetes ingress, ClusterIssuer, service types, and NetworkPolicy objects.
  all          Run every check that has the required local tools and environment variables.

Environment:
  MOOGLE_ENVIRONMENT              prod or staging (default: prod)
  MOOGLE_PUBLIC_HOST              Public DNS hostname for VM/Ingress checks
  MOOGLE_BASE_URL                 Public HTTPS URL, defaults to https://MOOGLE_PUBLIC_HOST
  MOOGLE_NAMESPACE                Kubernetes namespace (default: moogle)
  MOOGLE_K8S_OVERLAY              Kustomize overlay path override
  MOOGLE_K8S_SUFFIX               Rendered K8s suffix override (default: -prod or -staging)
  MOOGLE_CLUSTER_ISSUER           Expected cert-manager ClusterIssuer
  MOOGLE_PUBLIC_FORBIDDEN_PORTS   Space-separated ports that must not be publicly reachable
EOF
}

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s\n' "$1" >&2
  failed=1
}

skip() {
  printf 'SKIP %s\n' "$1"
  skipped=$((skipped + 1))
}

need_command() {
  local command_name="$1"

  if ! command -v "${command_name}" >/dev/null 2>&1; then
    skip "${command_name} is not installed"
    return 1
  fi
}

compose_env() {
  APP_KEY="${APP_KEY:-base64:test}" \
  APP_URL="${APP_URL:-https://moogle.example.com}" \
  CADDY_SITE_ADDRESS="${CADDY_SITE_ADDRESS:-moogle.example.com}" \
  QUERY_REDIS_URL="${QUERY_REDIS_URL:-redis://query-redis.internal:6379/0}" \
  CACHE_REDIS_URL="${CACHE_REDIS_URL:-redis://query-redis.internal:6379/1}" \
  MONGODB_URI="${MONGODB_URI:-mongodb://moogle_query:change-me@mongo.internal:27017/moogle?authSource=moogle}" \
  MONGODB_DATABASE="${MONGODB_DATABASE:-moogle}" \
  MONGODB_QUERY_USERNAME="${MONGODB_QUERY_USERNAME:-moogle_query}" \
  MONGODB_QUERY_PASSWORD="${MONGODB_QUERY_PASSWORD:-change-me}" \
  PIPELINE_REDIS_URL="${PIPELINE_REDIS_URL:-redis://:change-me@pipeline-redis.internal:6379/0}" \
  PIPELINE_REDIS_PASSWORD="${PIPELINE_REDIS_PASSWORD:-change-me}" \
  MONGO_USERNAME="${MONGO_USERNAME:-moogle}" \
  MONGO_PASSWORD="${MONGO_PASSWORD:-change-me}" \
  MONGO_INITDB_ROOT_USERNAME="${MONGO_INITDB_ROOT_USERNAME:-moogle}" \
  MONGO_INITDB_ROOT_PASSWORD="${MONGO_INITDB_ROOT_PASSWORD:-change-me}" \
  "$@"
}

validate_static_compose() {
  need_command docker || return 0
  need_command jq || return 0

  local config_json
  config_json="$(compose_env docker compose -f "${COMPOSE_FILE}" config --format json)"

  local exposed_ports
  exposed_ports="$(jq -r '.services | to_entries[] | select(.value.ports != null) | .key + ":" + (.value.ports | map((.published // "") + "->" + ((.target // "")|tostring)) | join(","))' <<<"${config_json}")"

  if [[ "${exposed_ports}" == "caddy:80->80,443->443" ]]; then
    pass "prod compose publishes only Caddy 80/443"
  else
    fail "prod compose exposes unexpected ports: ${exposed_ports}"
  fi

  local query_services
  query_services="$(jq -r '[.services | to_entries[] | select(.value.networks | has("query")) | .key] | sort | join(",")' <<<"${config_json}")"
  if [[ "${query_services}" == "caddy,laravel,query-redis" ]]; then
    pass "query network is limited to Caddy, Laravel, and query Redis"
  else
    fail "query network has unexpected members: ${query_services}"
  fi

  local pipeline_services
  pipeline_services="$(jq -r '[.services | to_entries[] | select(.value.networks | has("pipeline")) | .key] | sort | join(",")' <<<"${config_json}")"
  if [[ "${pipeline_services}" == "backlinks-processor,image-indexer,indexer,pipeline-redis,spider" ]]; then
    pass "pipeline network is isolated from query Redis"
  else
    fail "pipeline network has unexpected members: ${pipeline_services}"
  fi

  if command -v caddy >/dev/null 2>&1; then
    CADDY_SITE_ADDRESS=":80" caddy validate --config "${ROOT_DIR}/deploy/compose/Caddyfile" >/dev/null
    pass "Caddyfile validates for local HTTP mode"
  elif docker run --rm -e CADDY_SITE_ADDRESS=":80" -v "${ROOT_DIR}/deploy/compose/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2 caddy validate --config /etc/caddy/Caddyfile >/dev/null 2>&1; then
    pass "Caddyfile validates for local HTTP mode via Docker"
  else
    skip "Caddyfile validation skipped; install caddy or start Docker to validate with caddy:2"
  fi
}

validate_static_k8s() {
  need_command kubectl || return 0

  if [[ ! -d "${OVERLAY}" ]]; then
    fail "Kustomize overlay not found: ${OVERLAY}"
    return 0
  fi

  local rendered
  rendered="$(kubectl kustomize "${OVERLAY}")"

  if grep -q 'cert-manager.io/cluster-issuer' <<<"${rendered}" && grep -q 'nginx.ingress.kubernetes.io/force-ssl-redirect: "true"' <<<"${rendered}"; then
    pass "K8s ingress renders cert-manager and forced HTTPS annotations"
  else
    fail "K8s ingress is missing cert-manager or forced HTTPS annotations"
  fi

  if grep -q '^kind: NetworkPolicy$' <<<"${rendered}"; then
    pass "K8s overlay renders Redis NetworkPolicy objects"
  else
    fail "K8s overlay does not render Redis NetworkPolicy objects"
  fi

  if grep -q 'type: ClusterIP' <<<"${rendered}"; then
    pass "K8s services explicitly render as ClusterIP"
  else
    fail "K8s services do not explicitly render ClusterIP"
  fi
}

validate_public_http() {
  need_command curl || return 0

  if [[ -z "${PUBLIC_HOST}" || -z "${BASE_URL}" ]]; then
    skip "MOOGLE_PUBLIC_HOST or MOOGLE_BASE_URL not set; public DNS/TLS checks skipped"
    return 0
  fi

  if curl -fsS --max-time 10 "${BASE_URL}/api/health/live" >/dev/null; then
    pass "public liveness endpoint responds over HTTPS"
  else
    fail "public liveness endpoint failed: ${BASE_URL}/api/health/live"
  fi

  local redirect_status
  redirect_status="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 10 "http://${PUBLIC_HOST}/api/health/live" || true)"
  case "${redirect_status}" in
    301\ https://*|302\ https://*|307\ https://*|308\ https://*)
      pass "HTTP redirects to HTTPS"
      ;;
    *)
      fail "HTTP did not redirect to HTTPS: ${redirect_status}"
      ;;
  esac
}

validate_public_ports() {
  need_command nc || return 0

  if [[ -z "${PUBLIC_HOST}" ]]; then
    skip "MOOGLE_PUBLIC_HOST not set; public port checks skipped"
    return 0
  fi

  for port in 80 443; do
    if nc -z -w 5 "${PUBLIC_HOST}" "${port}" >/dev/null 2>&1; then
      pass "public port ${port} is reachable"
    else
      fail "public port ${port} is not reachable"
    fi
  done

  for port in ${PUBLIC_FORBIDDEN_PORTS}; do
    if nc -z -w 5 "${PUBLIC_HOST}" "${port}" >/dev/null 2>&1; then
      fail "forbidden public port ${port} is reachable"
    else
      pass "forbidden public port ${port} is closed"
    fi
  done
}

validate_tls_certificate() {
  need_command openssl || return 0

  if [[ -z "${PUBLIC_HOST}" ]]; then
    skip "MOOGLE_PUBLIC_HOST not set; TLS certificate check skipped"
    return 0
  fi

  local cert_subject
  cert_subject="$(printf '' | openssl s_client -connect "${PUBLIC_HOST}:443" -servername "${PUBLIC_HOST}" 2>/dev/null | openssl x509 -noout -subject -issuer 2>/dev/null || true)"

  if [[ -n "${cert_subject}" ]]; then
    pass "TLS certificate is presented for ${PUBLIC_HOST}"
    printf '%s\n' "${cert_subject}"
  else
    fail "TLS certificate could not be read for ${PUBLIC_HOST}"
  fi
}

validate_k8s_runtime() {
  need_command kubectl || return 0
  need_command jq || return 0

  if ! kubectl version --client >/dev/null 2>&1; then
    fail "kubectl is installed but not usable"
    return 0
  fi

  if ! kubectl get namespace "${NAMESPACE}" >/dev/null 2>&1; then
    skip "namespace ${NAMESPACE} is not reachable; live K8s runtime checks skipped"
    return 0
  fi

  local ingress_name="moogle-ingress${K8S_SUFFIX}"
  if kubectl -n "${NAMESPACE}" get ingress "${ingress_name}" >/dev/null 2>&1; then
    pass "K8s ingress ${ingress_name} exists"
  else
    fail "K8s ingress ${ingress_name} not found"
  fi

  if [[ -n "${CLUSTER_ISSUER}" ]]; then
    if kubectl get clusterissuer "${CLUSTER_ISSUER}" >/dev/null 2>&1; then
      pass "cert-manager ClusterIssuer ${CLUSTER_ISSUER} exists"
    else
      fail "cert-manager ClusterIssuer ${CLUSTER_ISSUER} not found"
    fi
  fi

  local bad_service_types
  bad_service_types="$(kubectl -n "${NAMESPACE}" get svc -o json | jq -r '[.items[] | select((.spec.type != "ClusterIP") and (.metadata.name | startswith("moogle-ingress") | not)) | .metadata.name + ":" + .spec.type] | join(",")')"
  if [[ -z "${bad_service_types}" ]]; then
    pass "K8s app services are ClusterIP-only"
  else
    fail "K8s app services expose unexpected types: ${bad_service_types}"
  fi

  if kubectl -n "${NAMESPACE}" get networkpolicy "query-redis-ingress${K8S_SUFFIX}" >/dev/null 2>&1 && kubectl -n "${NAMESPACE}" get networkpolicy "pipeline-redis-ingress${K8S_SUFFIX}" >/dev/null 2>&1; then
    pass "Redis NetworkPolicy objects exist"
  else
    fail "Redis NetworkPolicy objects are missing"
  fi
}

mode="${1:-all}"

case "${mode}" in
  -h|--help|help)
    usage
    exit 0
    ;;
  static)
    validate_static_compose
    validate_static_k8s
    ;;
  vm-public)
    validate_public_http
    validate_public_ports
    validate_tls_certificate
    ;;
  k8s-runtime)
    validate_k8s_runtime
    ;;
  all)
    validate_static_compose
    validate_static_k8s
    validate_public_http
    validate_public_ports
    validate_tls_certificate
    validate_k8s_runtime
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi

if [[ "${skipped}" -gt 0 ]]; then
  printf 'Completed with %s skipped check(s).\n' "${skipped}"
fi
