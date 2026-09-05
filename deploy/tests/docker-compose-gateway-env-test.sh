#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

gateway_variables=$(mktemp "${TMPDIR:-/tmp}/sub2api-gateway-env.XXXXXX")
cleanup() {
  rm -f "$gateway_variables"
}
trap cleanup EXIT HUP INT TERM

awk '
  /^GATEWAY_[A-Z0-9_]+=/ {
    separator = index($0, "=")
    print substr($0, 1, separator - 1) "\t" substr($0, separator + 1)
  }
' deploy/.env.example > "$gateway_variables"

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml \
  deploy/docker-compose.dev.yml
do
  tab=$(printf '\t')
  while IFS="$tab" read -r key value; do
    # .env.example intentionally includes high-capacity tuning examples for
    # these values. An unconfigured Compose deployment must retain the
    # backend defaults instead of silently adopting the examples.
    case "$key" in
      GATEWAY_MAX_CONNS_PER_HOST) value=1024 ;;
      GATEWAY_MAX_IDLE_CONNS) value=2560 ;;
      GATEWAY_MAX_IDLE_CONNS_PER_HOST) value=120 ;;
    esac

    expected=$(printf '      - %s=${%s:-%s}' "$key" "$key" "$value")
    expected_count=$(grep -Fxc "$expected" "$compose_file" || true)
    key_count=$(grep -Ec "^[[:space:]]*-[[:space:]]*${key}([[:space:]]*=.*)?[[:space:]]*$" "$compose_file" || true)
    if [ "$expected_count" -ne 1 ] || [ "$key_count" -ne 1 ]; then
      printf '%s must pass %s with the expected fallback exactly once\n' "$compose_file" "$key" >&2
      exit 1
    fi
  done < "$gateway_variables"
done

printf 'docker compose Gateway environment test passed\n'
