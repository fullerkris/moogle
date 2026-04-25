#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <kubeconfig-path> <kubectl args...>" >&2
  echo "Example: $0 k8s/kubeconfig.local.yaml apply -k k8s/monitoring-operator" >&2
  exit 1
fi

KUBECONFIG_PATH="$1"
shift

if [[ ! -f "${KUBECONFIG_PATH}" ]]; then
  echo "Kubeconfig file not found: ${KUBECONFIG_PATH}" >&2
  exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

KUBECONFIG="${KUBECONFIG_PATH}" kubectl "$@"
