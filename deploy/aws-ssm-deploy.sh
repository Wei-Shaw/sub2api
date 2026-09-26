#!/usr/bin/env bash

set -Eeuo pipefail

readonly DEFAULT_REPO_DIR="/home/ec2-user/gptplusch-sub2api"
readonly DEFAULT_HEALTH_ATTEMPTS=60
readonly DEFAULT_HEALTH_INTERVAL=5

validate_target() {
  local target_sha="$1"
  local image="$2"

  [[ "${target_sha}" =~ ^[0-9a-f]{40}$ ]]
  [[ "${image}" == "ghcr.io/domenlee/sub2api:${target_sha}" ]]
}

render_override() {
  local source_file="$1"
  local output_file="$2"
  local image="$3"

  awk -v image="${image}" '
    /^  sub2api:[[:space:]]*$/ {
      in_sub2api = 1
      print
      next
    }
    /^  [A-Za-z0-9_-]+:[[:space:]]*$/ && in_sub2api == 1 {
      in_sub2api = 0
    }
    in_sub2api == 1 && /^[[:space:]]+image:[[:space:]]*/ && replaced == 0 {
      print "    image: \"" image "\""
      replaced = 1
      next
    }
    { print }
    END {
      if (replaced != 1) {
        exit 42
      }
    }
  ' "${source_file}" > "${output_file}"
}

wait_for_health() {
  local attempts="${1:-${DEFAULT_HEALTH_ATTEMPTS}}"
  local interval="${2:-${DEFAULT_HEALTH_INTERVAL}}"
  local state health attempt

  for attempt in $(seq 1 "${attempts}"); do
    state="$(docker inspect sub2api --format '{{.State.Status}}' 2>/dev/null || true)"
    health="$(docker inspect sub2api --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' 2>/dev/null || true)"
    echo "health attempt=${attempt} state=${state} health=${health}"

    if [[ "${health}" == "healthy" ]]; then
      return 0
    fi
    if [[ "${state}" == "exited" || "${state}" == "dead" ]]; then
      return 1
    fi
    sleep "${interval}"
  done

  return 1
}

main() {
  local target_sha="${1:?target commit SHA is required}"
  local image="${2:?target image is required}"
  local repo_dir="${REPO_DIR:-${DEFAULT_REPO_DIR}}"
  local deploy_dir="${repo_dir}/deploy"
  local compose_base="${deploy_dir}/docker-compose.local.yml"
  local compose_override="${deploy_dir}/docker-compose.prod.yml"
  local stamp backup_dir target_compose override_tmp
  local old_head old_image expected_version public_settings actual_image
  local rollback_required=0

  validate_target "${target_sha}" "${image}"
  test -d "${repo_dir}/.git"
  test -f "${compose_base}"
  test -f "${compose_override}"

  exec 9>/tmp/gptplusch-sub2api-production-deploy.lock
  flock -n 9 || {
    echo "another production deployment is already running" >&2
    return 1
  }

  git -C "${repo_dir}" diff --quiet
  git -C "${repo_dir}" diff --cached --quiet
  test "$(git -C "${repo_dir}" rev-parse origin/custom-ui)" = "${target_sha}"

  old_head="$(git -C "${repo_dir}" rev-parse HEAD)"
  old_image="$(docker inspect sub2api --format '{{.Config.Image}}')"
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_dir="${deploy_dir}/release-backups/${stamp}-${target_sha:0:12}"
  target_compose="${deploy_dir}/.docker-compose.target-${target_sha}.yml"
  mkdir -p "${backup_dir}"
  trap 'rm -f "${target_compose}"' EXIT

  cp "${compose_base}" "${backup_dir}/docker-compose.local.yml"
  cp "${compose_override}" "${backup_dir}/docker-compose.prod.yml"
  git -C "${repo_dir}" show "${target_sha}:deploy/docker-compose.local.yml" > "${target_compose}"
  test -s "${target_compose}"
  cp "${target_compose}" "${backup_dir}/docker-compose.target.yml"
  printf '%s\n' "${old_head}" > "${backup_dir}/git-head.txt"
  printf '%s\n' "${old_image}" > "${backup_dir}/image.txt"
  docker exec sub2api-postgres pg_dump -U sub2api -d sub2api -Fc > "${backup_dir}/sub2api.dump"
  test -s "${backup_dir}/sub2api.dump"
  sha256sum "${backup_dir}/sub2api.dump" "${backup_dir}/docker-compose.prod.yml" > "${backup_dir}/SHA256SUMS"
  chmod 600 "${backup_dir}"/*
  echo "backup=${backup_dir}"

  rollback() {
    local exit_code=$?
    trap - ERR

    if [[ "${rollback_required}" == 1 ]]; then
      echo "deployment failed; restoring ${old_image}" >&2
      cp "${backup_dir}/docker-compose.prod.yml" "${compose_override}"
      docker compose \
        --project-directory "${deploy_dir}" \
        -f "${backup_dir}/docker-compose.local.yml" \
        -f "${compose_override}" \
        up -d --no-deps --force-recreate sub2api || true
      wait_for_health 36 5 || true
      docker inspect sub2api --format 'rollback image={{.Config.Image}} status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{end}}' || true
    fi

    exit "${exit_code}"
  }
  trap rollback ERR

  override_tmp="$(mktemp "${deploy_dir}/docker-compose.prod.yml.XXXXXX")"
  render_override "${compose_override}" "${override_tmp}" "${image}"
  chmod --reference="${compose_override}" "${override_tmp}"
  mv "${override_tmp}" "${compose_override}"
  rollback_required=1

  docker compose \
    --project-directory "${deploy_dir}" \
    -f "${target_compose}" \
    -f "${compose_override}" \
    config >/dev/null
  docker pull "${image}"
  docker compose \
    --project-directory "${deploy_dir}" \
    -f "${target_compose}" \
    -f "${compose_override}" \
    up -d --no-deps --force-recreate sub2api

  wait_for_health
  actual_image="$(docker inspect sub2api --format '{{.Config.Image}}')"
  test "${actual_image}" = "${image}"

  expected_version="$(git -C "${repo_dir}" show "${target_sha}:backend/cmd/server/VERSION" | tr -d '[:space:]')"
  public_settings="$(docker exec sub2api wget -qO- http://127.0.0.1:8080/api/v1/settings/public)"
  grep -Fq "\"version\":\"${expected_version}\"" <<< "${public_settings}"
  docker exec sub2api-postgres pg_isready -U sub2api -d sub2api

  if docker logs --since 10m sub2api 2>&1 | grep -Eiq 'fatal|panic|migration.*(fail|error)|failed.*migration'; then
    echo "fatal startup or migration error detected" >&2
    return 1
  fi

  git -C "${repo_dir}" merge --ff-only --quiet "${target_sha}"
  test "$(git -C "${repo_dir}" rev-parse HEAD)" = "${target_sha}"

  rollback_required=0
  trap - ERR
  rm -f "${target_compose}"
  trap - EXIT
  docker inspect sub2api --format 'deployed image={{.Config.Image}} status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{end}} restarts={{.RestartCount}}'
  echo "deployed version=${expected_version} commit=${target_sha}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
