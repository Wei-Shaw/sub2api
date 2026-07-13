#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

helm upgrade --install sub2api "${SCRIPT_DIR}/helm/sub2api" \
  --dependency-update \
  --namespace sub2api \
  --create-namespace \
  -f "${SCRIPT_DIR}/helm-values.simple.yaml"
