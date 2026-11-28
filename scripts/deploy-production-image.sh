#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

exec "$ROOT/scripts/deploy-by-digest.sh" \
  --project sub2api \
  --compose "${SUB2API_COMPOSE_FILE:-$ROOT/deploy/docker-compose.yml}" \
  --service "${SUB2API_DEPLOY_SERVICE:-sub2api}" \
  --container "${SUB2API_DEPLOY_CONTAINER:-sub2api}" \
  --lock "${SUB2API_IMAGE_LOCK:-$ROOT/deploy/IMAGE_LOCK.txt}" \
  "$@"
