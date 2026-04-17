#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPLATE="${ROOT_DIR}/docs/weekly-readiness-scorecard-template.md"
OUTPUT_DIR="${ROOT_DIR}/docs/scorecards"

if [[ ! -f "${TEMPLATE}" ]]; then
  echo "Template not found: ${TEMPLATE}" >&2
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"

week_ending="$(date +%Y-%m-%d)"
output_file="${OUTPUT_DIR}/weekly-readiness-${week_ending}.md"

cp "${TEMPLATE}" "${output_file}"

echo "Created scorecard: ${output_file}"
