#!/usr/bin/env bash
set -euo pipefail

deploy_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$deploy_dir"

compose_files=(
  docker-compose.yml
  docker-compose.local.yml
  docker-compose.dev.yml
)

assert_redis_command() {
  local file=$1
  local expected=$2
  local config

  config="$(docker compose -f "$file" config --format json)"
  if ! jq -e --argjson expected "$expected" \
    '.services.redis.command == $expected' <<<"$config" >/dev/null; then
    printf 'unexpected Redis command in %s: ' "$file" >&2
    jq -c '.services.redis.command' <<<"$config" >&2
    return 1
  fi
}

export POSTGRES_PASSWORD=fixture-postgres
export REDIS_PASSWORD=
unset REDIS_SAVE_SECONDS REDIS_SAVE_CHANGES REDIS_LATENCY_MONITOR_THRESHOLD_MS

default_command='["redis-server","--save","60","1","--appendonly","yes","--appendfsync","everysec","--latency-monitor-threshold","0","--requirepass",""]'
for file in "${compose_files[@]}"; do
  assert_redis_command "$file" "$default_command"
done

export REDIS_PASSWORD=fixture-redis
export REDIS_SAVE_SECONDS=900
export REDIS_SAVE_CHANGES=2
export REDIS_LATENCY_MONITOR_THRESHOLD_MS=100

custom_command='["redis-server","--save","900","2","--appendonly","yes","--appendfsync","everysec","--latency-monitor-threshold","100","--requirepass","fixture-redis"]'
for file in "${compose_files[@]}"; do
  assert_redis_command "$file" "$custom_command"
done

printf 'Compose Redis command fixtures passed for %s files.\n' "${#compose_files[@]}"
