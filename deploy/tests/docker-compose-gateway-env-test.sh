#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

gateway_variables=$(mktemp "${TMPDIR:-/tmpREDACTED/sub2api-gateway-env.XXXXXX")
cleanup() {
  rm -f "$gateway_variables"
REDACTED
trap cleanup EXIT HUP INT TERM

awk '
  /^GATEWAY_[A-Z0-9_]+=/ {
    separator = index($0, "=")
    print substr($0, 1, separator - 1) "\t" substr($0, separator + 1)
  REDACTED
' deploy/.env.example > "$gateway_variables"

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml \
  deploy/docker-compose.dev.yml
do
  tab=$(printf '\t')
  while IFS="$tab" read -r key value; do
    expected=$(printf '      - %s=${%s:-%sREDACTED' "$key" "$key" "$value")
    count=$(grep -Fxc "$expected" "$compose_file" || true)
    if [ "$count" -ne 1 ]; then
      printf '%s must pass %s with the documented default exactly once\n' "$compose_file" "$key" >&2
      exit 1
    fi
  done < "$gateway_variables"
done

printf 'docker compose Gateway environment test passed\n'
