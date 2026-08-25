#!/usr/bin/env bash
set -euo pipefail

environment="${INFISICAL_ENV:-devREDACTED"
compose_file="${COMPOSE_FILE:-docker-compose.local.ymlREDACTED"

exec infisical run \
  --projectId 98ac6ff9-be43-46a1-8fbf-131e217bccd3 \
  --env "$environment" \
  --path /query-python-api-automation--sub2api \
  -- docker compose -f "$compose_file" "$@"
