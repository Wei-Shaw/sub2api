#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CODEX_VERSION="${CODEX_E2E_VERSION:-0.166.0}"
IMAGE="${CODEX_E2E_IMAGE:-sub2api-codexapp-e2e:${CODEX_VERSION}}"

docker build \
  --build-arg "CODEX_VERSION=${CODEX_VERSION}" \
  --file "${BACKEND_DIR}/internal/codexapp/testdata/real-appserver/Dockerfile" \
  --tag "${IMAGE}" \
  "${BACKEND_DIR}"

docker run --rm \
  --network none \
  --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=256m \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --pids-limit 256 \
  --memory 1g \
  --cpus 2 \
  "${IMAGE}"
