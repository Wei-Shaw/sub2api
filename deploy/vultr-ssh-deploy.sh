#!/usr/bin/env bash

set -Eeuo pipefail

readonly DEFAULT_DEPLOY_DIR="/opt/sub2api"

validate_target() {
  [[ "$1" =~ ^[0-9a-f]{40}$ ]] &&
    [[ "$2" == "ghcr.io/domenlee/sub2api:$1" ]]
}

render_compose() {
  python3 - "$1" "$2" "$3" <<'PY'
import json
import sys

source, output, image = sys.argv[1:]
with open(source, encoding="utf-8") as handle:
    config = json.load(handle)
app = config["services"]["sub2api"]
if not app.get("image"):
    raise SystemExit("Production Compose is missing the sub2api image")
app["image"] = image
with open(output, "w", encoding="utf-8") as handle:
    json.dump(config, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
}

wait_for_health() {
  local attempt state health
  for attempt in $(seq 1 "${1:-60}"); do
    state="$(docker inspect sub2api --format '{{.State.Status}}' 2>/dev/null || true)"
    health="$(docker inspect sub2api --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' 2>/dev/null || true)"
    echo "health attempt=${attempt} state=${state} health=${health}"
    if [[ "${health}" == healthy ]]; then
      return 0
    fi
    if [[ "${state}" == exited || "${state}" == dead ]]; then
      return 1
    fi
    sleep 5
  done
  return 1
}

main() {
  local target_sha="${1:?target commit SHA is required}"
  local image="${2:?target image is required}"
  local deploy_dir="${DEFAULT_DEPLOY_DIR}"
  local compose_file="${deploy_dir}/compose.production.json"
  local old_image database_user database_name stamp backup_dir candidate started_at
  local rollback_required=0

  validate_target "${target_sha}" "${image}"
  [[ "${EUID}" == 0 ]]
  test -f "${compose_file}"
  umask 077

  exec 9>/run/lock/sub2api-vultr-deploy.lock
  flock -n 9 || {
    echo "Another Vultr production deployment is running; retry after it finishes." >&2
    return 1
  }

  old_image="$(docker inspect sub2api --format '{{.Config.Image}}')"
  IFS='|' read -r database_user database_name < <(
    docker inspect sub2api | python3 -c '
import json, sys
env = dict(item.split("=", 1) for item in json.load(sys.stdin)[0]["Config"]["Env"])
if env.get("DATABASE_HOST") != "postgres":
    raise SystemExit("Unexpected production database host")
print(env["DATABASE_USER"] + "|" + env["DATABASE_DBNAME"])
'
  )
  [[ "${database_user}" =~ ^[a-zA-Z0-9_]+$ ]]
  [[ "${database_name}" =~ ^[a-zA-Z0-9_]+$ ]]

  # Pull before taking backups or changing the running service.
  docker pull "${image}"
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_dir="${deploy_dir}/release-backups/${stamp}-${target_sha:0:12}"
  mkdir -p "${backup_dir}"
  cp "${compose_file}" "${backup_dir}/compose.production.json"
  docker inspect sub2api > "${backup_dir}/app-inspect.json"
  printf '%s\n' "${old_image}" > "${backup_dir}/image.txt"
  docker exec sub2api-postgres pg_dump -U "${database_user}" -d "${database_name}" -Fc -Z1 \
    > "${backup_dir}/database.dump.partial"
  test -s "${backup_dir}/database.dump.partial"
  mv "${backup_dir}/database.dump.partial" "${backup_dir}/database.dump"
  (
    cd "${backup_dir}"
    sha256sum database.dump compose.production.json app-inspect.json image.txt > SHA256SUMS
  )
  echo "backup=${backup_dir} database=${database_name}"

  candidate="$(mktemp "${deploy_dir}/.compose.production.XXXXXX")"
  trap 'rm -f "${candidate}"' EXIT
  render_compose "${compose_file}" "${candidate}" "${image}"
  chmod 600 "${candidate}"
  docker compose --project-directory "${deploy_dir}" -f "${candidate}" config --quiet

  rollback() {
    local exit_code="$1"
    # ERR can also run inside command substitutions; the parent owns rollback.
    if (( BASH_SUBSHELL > 0 )); then
      exit "${exit_code}"
    fi
    trap - ERR HUP INT TERM
    set +e
    if [[ "${rollback_required}" == 1 ]]; then
      echo "Vultr deployment ${target_sha} failed; restoring ${old_image}. Backup: ${backup_dir}" >&2
      cp "${backup_dir}/compose.production.json" "${compose_file}"
      docker compose --project-directory "${deploy_dir}" -f "${compose_file}" \
        up -d --no-deps --force-recreate sub2api
      if ! wait_for_health 36; then
        echo "Rollback is unhealthy; inspect sub2api and ${backup_dir}/startup.log on Vultr." >&2
      fi
    fi
    exit "${exit_code}"
  }
  trap 'rollback "$?"' ERR
  trap 'rollback 129' HUP
  trap 'rollback 130' INT
  trap 'rollback 143' TERM

  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  rollback_required=1
  mv "${candidate}" "${compose_file}"
  docker compose --project-directory "${deploy_dir}" -f "${compose_file}" \
    up -d --no-deps --force-recreate sub2api
  wait_for_health
  test "$(docker inspect sub2api --format '{{.Config.Image}}')" = "${image}"
  docker exec sub2api wget -q -T 10 -O /dev/null http://127.0.0.1:8080/health
  docker exec sub2api-postgres pg_isready -U "${database_user}" -d "${database_name}"
  docker logs --since "${started_at}" sub2api > "${backup_dir}/startup.log" 2>&1
  if grep -Eiq '"level":"fatal"|panic:|migration.*(fail|error)|failed.*migration' "${backup_dir}/startup.log"; then
    echo "Startup failed for ${target_sha}; inspect ${backup_dir}/startup.log." >&2
    false
  fi

  rollback_required=0
  trap - ERR HUP INT TERM EXIT
  printf '%s\n' "${target_sha}" > "${deploy_dir}/deployed-commit.txt"
  docker inspect sub2api --format 'deployed image={{.Config.Image}} health={{.State.Health.Status}} restarts={{.RestartCount}}'
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
