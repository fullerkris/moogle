#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/create-kubeconfig-local.sh \
    --server <https://api-server:6443> \
    --token <bearer-token> \
    [--ca-file <path/to/ca.crt> | --ca-data <base64-ca>] \
    [--context <name>] \
    [--cluster <name>] \
    [--user <name>] \
    [--namespace <name>] \
    [--output <path>]

Defaults:
  --context   moogle-context
  --cluster   moogle-cluster
  --user      moogle-deployer
  --namespace moogle
  --output    k8s/kubeconfig.local.yaml

Examples:
  scripts/create-kubeconfig-local.sh \
    --server https://1.2.3.4:6443 \
    --token "$(cat token.txt)" \
    --ca-file ./cluster-ca.crt

  scripts/create-kubeconfig-local.sh \
    --server https://1.2.3.4:6443 \
    --token "$K8S_BEARER_TOKEN" \
    --ca-data "$K8S_CA_B64"
EOF
}

require_value() {
  local flag="$1"
  local value="$2"
  if [[ -z "${value}" ]]; then
    echo "Missing required value for ${flag}" >&2
    exit 1
  fi
}

SERVER=""
TOKEN=""
CA_FILE=""
CA_DATA=""
CONTEXT_NAME="moogle-context"
CLUSTER_NAME="moogle-cluster"
USER_NAME="moogle-deployer"
NAMESPACE_NAME="moogle"
OUTPUT_PATH="k8s/kubeconfig.local.yaml"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server)
      SERVER="${2:-}"
      shift 2
      ;;
    --token)
      TOKEN="${2:-}"
      shift 2
      ;;
    --ca-file)
      CA_FILE="${2:-}"
      shift 2
      ;;
    --ca-data)
      CA_DATA="${2:-}"
      shift 2
      ;;
    --context)
      CONTEXT_NAME="${2:-}"
      shift 2
      ;;
    --cluster)
      CLUSTER_NAME="${2:-}"
      shift 2
      ;;
    --user)
      USER_NAME="${2:-}"
      shift 2
      ;;
    --namespace)
      NAMESPACE_NAME="${2:-}"
      shift 2
      ;;
    --output)
      OUTPUT_PATH="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

require_value "--server" "${SERVER}"
require_value "--token" "${TOKEN}"

if [[ -n "${CA_FILE}" && -n "${CA_DATA}" ]]; then
  echo "Use only one of --ca-file or --ca-data" >&2
  exit 1
fi

if [[ -n "${CA_FILE}" ]]; then
  if [[ ! -f "${CA_FILE}" ]]; then
    echo "CA file not found: ${CA_FILE}" >&2
    exit 1
  fi
  CA_DATA="$(base64 < "${CA_FILE}" | tr -d '\n')"
fi

if [[ -z "${CA_DATA}" ]]; then
  echo "Either --ca-file or --ca-data is required" >&2
  exit 1
fi

OUTPUT_DIR="$(dirname "${OUTPUT_PATH}")"
mkdir -p "${OUTPUT_DIR}"

umask 077
cat > "${OUTPUT_PATH}" <<EOF
apiVersion: v1
kind: Config
clusters:
  - name: ${CLUSTER_NAME}
    cluster:
      server: ${SERVER}
      certificate-authority-data: ${CA_DATA}

users:
  - name: ${USER_NAME}
    user:
      token: ${TOKEN}

contexts:
  - name: ${CONTEXT_NAME}
    context:
      cluster: ${CLUSTER_NAME}
      user: ${USER_NAME}
      namespace: ${NAMESPACE_NAME}

current-context: ${CONTEXT_NAME}
EOF

if ! kubectl config view --kubeconfig "${OUTPUT_PATH}" >/dev/null 2>&1; then
  echo "Kubeconfig written but failed kubectl parse check: ${OUTPUT_PATH}" >&2
  exit 1
fi

echo "Wrote kubeconfig: ${OUTPUT_PATH}"
echo "Validate connectivity: scripts/kubectl-with-config.sh ${OUTPUT_PATH} get ns"
