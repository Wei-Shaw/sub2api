#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

expected='      - GATEWAY_OPENAI_WS_FORCE_HTTP=${GATEWAY_OPENAI_WS_FORCE_HTTP:-falseREDACTED'

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml \
  deploy/docker-compose.dev.yml
do
  count=$(grep -Fxc "$expected" "$compose_file" || true)
  if [ "$count" -ne 1 ]; then
    printf '%s must pass GATEWAY_OPENAI_WS_FORCE_HTTP exactly once to sub2api\n' "$compose_file" >&2
    exit 1
  fi
done

printf 'docker compose OpenAI force HTTP environment test passed\n'
