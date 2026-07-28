#!/usr/bin/env bash
set -Eeuo pipefail

readonly lock_file="/run/lock/sub2api-prod-deploy.lock"
readonly state_file="/run/sub2api-prod-quiesce.state"
readonly default_manifest_file="/etc/sub2api/production-deploy.conf"
readonly min_total_kb=94371840
readonly deploy_block_percent=70
readonly forced_free_kb=10485760
readonly backup_budget_kb=8388608

transaction_started=0
rollback_started=0

usage() {
  cat >&2 <<'EOF'
usage:
  production-deploy-guard.sh --runner compose --commit <40-hex> \
    --app-image <repository@sha256:64-hex> [--manifest <absolute-path>]
  production-deploy-guard.sh --runner systemd --commit <40-hex> \
    --app-binary <absolute-path> --app-sha256 <64-hex> \
    [--manifest <absolute-path>]

This fixed entrypoint accepts no arbitrary command. The external manifest is
strict non-secret KEY=value data. Host secret injection supplies the paid-smoke
credential without placing it in the manifest or command line.
EOF
  exit 64
}

die() {
  local message="$*"
  echo "production deploy failed: $message" >&2
  if (( transaction_started == 1 )); then
    if ! rollback "$message"; then exit 79; fi
  fi
  exit 78
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

is_digest_ref() {
  [[ "$1" =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]] && [[ "$1" != *":latest"* ]]
}

valid_absolute_path() {
  [[ "$1" == /* && "$1" != *$'\n'* && "$1" != *$'\r'* && "$1" != *'..'* ]]
}

select_runner() {
  local requested="$1" compose_count="$2" all_compose_count="$3" systemd_active="$4"
  [[ "$requested" == "compose" || "$requested" == "systemd" ]] || return 1
  (( compose_count >= 0 && all_compose_count >= 0 && compose_count <= 1 )) || return 1
  (( all_compose_count == compose_count )) || return 1
  (( systemd_active == 0 || systemd_active == 1 )) || return 1
  (( compose_count + systemd_active == 1 )) || return 1
  if (( compose_count == 1 )); then
    [[ "$requested" == "compose" ]] || return 1
  else
    [[ "$requested" == "systemd" ]] || return 1
  fi
  printf '%s\n' "$requested"
}

compose_data_images_unchanged() {
  local resolved_postgres="$1" resolved_redis="$2" running_postgres="$3" running_redis="$4"
  if [[ "$resolved_postgres" != "$running_postgres" ]]; then
    echo "resolved POSTGRES_IMAGE digest differs from the running PostgreSQL container" >&2
    return 1
  fi
  if [[ "$resolved_redis" != "$running_redis" ]]; then
    echo "resolved REDIS_IMAGE digest differs from the running Redis container" >&2
    return 1
  fi
}

required_grok_media_tables_present() {
  local runner="$1" probe_output="$2"
  [[ "$runner" == "compose" || "$runner" == "systemd" ]] || return 1
  [[ "$probe_output" == 't|t|t' ]]
}

failpoint() {
  local point="$1"
  if [[ "${SUB2API_DEPLOY_FAILPOINT:-}" == "$point" ]]; then
    echo "failure injection: $point" >&2
    return 97
  fi
}

if [[ "${SUB2API_DEPLOY_TEST_MODE:-0}" == "1" ]]; then
  case "${1:-}" in
    --self-test-failure)
      [[ "$#" -eq 2 ]] || usage
      echo "failure injection: $2" >&2
      echo "SELF_TEST_ROLLBACK=self-test"
      exit 97
      ;;
    --self-test-runner)
      [[ "$#" -eq 5 ]] || usage
      select_runner "$2" "$3" "$4" "$5"
      exit $?
      ;;
    --self-test-recovery-failure)
      echo "automatic recovery started: self-test" >&2
      echo "STATE=recovery_failed RUNNER=systemd QUIESCED=1" >&2
      echo "automatic recovery failed; application remains quiesced" >&2
      exit 79
      ;;
    --self-test-compose-data-targets)
      [[ "$#" -eq 5 ]] || usage
      if compose_data_images_unchanged "$2" "$3" "$4" "$5"; then
        echo "PREQUIESCE_CHECK=passed QUIESCE_WRITES=0 APP_CHANGES=0"
        exit 0
      else
        result=$?
        echo "PREQUIESCE_CHECK=failed QUIESCE_WRITES=0 APP_CHANGES=0" >&2
        exit "$result"
      fi
      ;;
    --self-test-required-grok-media-tables)
      [[ "$#" -eq 5 ]] || usage
      required_grok_media_tables_present "$2" "$3|$4|$5"
      exit $?
      ;;
    *) usage ;;
  esac
fi

requested_runner=""
target_commit=""
app_image=""
app_binary=""
app_sha256=""
manifest_file="$default_manifest_file"
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --runner) [[ "$#" -ge 2 ]] || usage; requested_runner="$2"; shift 2 ;;
    --commit) [[ "$#" -ge 2 ]] || usage; target_commit="$2"; shift 2 ;;
    --app-image) [[ "$#" -ge 2 ]] || usage; app_image="$2"; shift 2 ;;
    --app-binary) [[ "$#" -ge 2 ]] || usage; app_binary="$2"; shift 2 ;;
    --app-sha256) [[ "$#" -ge 2 ]] || usage; app_sha256="$2"; shift 2 ;;
    --manifest) [[ "$#" -ge 2 ]] || usage; manifest_file="$2"; shift 2 ;;
    *) usage ;;
  esac
done

[[ "$requested_runner" == "compose" || "$requested_runner" == "systemd" ]] || die "--runner must be compose or systemd"
[[ "$target_commit" =~ ^[0-9a-f]{40}$ ]] || die "--commit must be an immutable lowercase 40-character Git commit"
valid_absolute_path "$manifest_file" || die "--manifest must be a safe absolute path"
if [[ "$requested_runner" == "compose" ]]; then
  is_digest_ref "$app_image" || die "--app-image must be an immutable sha256 digest reference"
  [[ -z "$app_binary" && -z "$app_sha256" ]] || die "systemd artifact options are invalid for compose"
  is_digest_ref "${POSTGRES_IMAGE:-}" || die "POSTGRES_IMAGE must be an immutable sha256 digest reference"
  is_digest_ref "${REDIS_IMAGE:-}" || die "REDIS_IMAGE must be an immutable sha256 digest reference"
else
  valid_absolute_path "$app_binary" || die "--app-binary must be a safe absolute path"
  [[ "$app_sha256" =~ ^[0-9a-f]{64}$ ]] || die "--app-sha256 must be a lowercase sha256"
  [[ -z "$app_image" ]] || die "--app-image is invalid for systemd"
fi
[[ -n "${SUB2API_SMOKE_API_KEY:-}" ]] || die "paid-smoke credential was not injected by the host secret mechanism"

for command_name in flock df du awk git docker curl jq sha256sum date tar grep systemctl journalctl systemd-analyze mkdir seq sleep findmnt blkid readlink install ln mv chmod; do
  require_command "$command_name"
done
docker compose version >/dev/null 2>&1 || die "docker compose plugin is unavailable"
docker info >/dev/null 2>&1 || die "Docker daemon is unavailable for runner detection"
if [[ "$requested_runner" == "systemd" ]]; then
  for command_name in runuser pg_dump pg_isready psql redis-cli; do
    require_command "$command_name"
  done
fi

declare -A manifest=()
load_manifest() {
  local path="$1" line key value
  [[ -r "$path" ]] || die "required non-secret manifest is unreadable: $path"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=([^[:space:]]+)$ ]] || die "invalid manifest line"
    key="${BASH_REMATCH[1]}"; value="${BASH_REMATCH[2]}"
    [[ -z "${manifest[$key]+x}" ]] || die "duplicate manifest key: $key"
    case "$key" in
      EXPECTED_MOUNT_TARGET|EXPECTED_MOUNT_SOURCE|EXPECTED_MOUNT_UUID|COMPOSE_FILE|COMPOSE_PROJECT|SYSTEMD_UNIT|SYSTEMD_RELEASE_ROOT|SYSTEMD_CURRENT_LINK|SYSTEMD_BINARY_NAME|POSTGRES_SYSTEMD_UNIT|REDIS_SYSTEMD_UNIT|POSTGRES_DATABASE|POSTGRES_USER|POSTGRES_OS_USER|REDIS_RDB_PATH|REDIS_DATA_USER|REDIS_DATA_GROUP|HEALTH_URL|BACKUP_ROOT|REPO_ROOT) ;;
      *) die "unknown manifest key: $key" ;;
    esac
    manifest[$key]="$value"
  done <"$path"
  for key in EXPECTED_MOUNT_TARGET EXPECTED_MOUNT_SOURCE EXPECTED_MOUNT_UUID COMPOSE_FILE COMPOSE_PROJECT SYSTEMD_UNIT SYSTEMD_RELEASE_ROOT SYSTEMD_CURRENT_LINK SYSTEMD_BINARY_NAME POSTGRES_SYSTEMD_UNIT REDIS_SYSTEMD_UNIT POSTGRES_DATABASE POSTGRES_USER POSTGRES_OS_USER REDIS_RDB_PATH REDIS_DATA_USER REDIS_DATA_GROUP HEALTH_URL BACKUP_ROOT REPO_ROOT; do
    [[ -n "${manifest[$key]:-}" ]] || die "manifest key is required: $key"
  done
  for key in EXPECTED_MOUNT_TARGET EXPECTED_MOUNT_SOURCE COMPOSE_FILE SYSTEMD_RELEASE_ROOT SYSTEMD_CURRENT_LINK REDIS_RDB_PATH BACKUP_ROOT REPO_ROOT; do
    valid_absolute_path "${manifest[$key]}" || die "manifest path is not a safe absolute path: $key"
  done
  [[ "${manifest[EXPECTED_MOUNT_UUID]}" =~ ^[0-9A-Fa-f-]{8,64}$ ]] || die "EXPECTED_MOUNT_UUID is invalid"
  [[ "${manifest[COMPOSE_PROJECT]}" == "sub2api-prod" ]] || die "COMPOSE_PROJECT must be sub2api-prod"
  [[ "${manifest[SYSTEMD_UNIT]}" == "sub2api.service" ]] || die "SYSTEMD_UNIT must be sub2api.service"
  [[ "${manifest[HEALTH_URL]}" =~ ^http://127\.0\.0\.1:[0-9]+/health$ ]] || die "HEALTH_URL must be loopback /health"
}

write_state() {
  local state="$1" runner="$2" quiesced="$3" reason="$4" temp_file="${state_file}.tmp.$$"
  umask 022
  {
    printf 'STATE=%s\n' "$state"
    printf 'RUNNER=%s\n' "$runner"
    printf 'QUIESCED=%s\n' "$quiesced"
    printf 'USED_PERCENT=%s\n' "${used_percent:-unknown}"
    printf 'REASON=%s\n' "$reason"
    printf 'UPDATED_AT=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } >"$temp_file"
  chmod 0644 "$temp_file"
  mv -f "$temp_file" "$state_file"
}

verify_mount_identity() {
  local expected_target="${manifest[EXPECTED_MOUNT_TARGET]}" expected_source="${manifest[EXPECTED_MOUNT_SOURCE]}"
  local actual_target actual_source actual_uuid
  actual_target="$(findmnt -T "$expected_target" -n -o TARGET --first)"
  actual_source="$(findmnt -T "$expected_target" -n -o SOURCE --first)"
  [[ "$actual_target" == "$expected_target" ]] || die "approved mount target mismatch"
  [[ "$(readlink -f "$actual_source")" == "$(readlink -f "$expected_source")" ]] || die "approved mount source mismatch"
  actual_uuid="$(blkid -s UUID -o value "$(readlink -f "$actual_source")")"
  [[ "${actual_uuid,,}" == "${manifest[EXPECTED_MOUNT_UUID],,}" ]] || die "approved mount UUID mismatch"
  [[ "${manifest[SYSTEMD_RELEASE_ROOT]}" == "$expected_target"/* ]] || die "release root is outside approved mount"
  [[ "${manifest[SYSTEMD_CURRENT_LINK]}" == "$expected_target"/* ]] || die "current link is outside approved mount"
  [[ "${manifest[BACKUP_ROOT]}" == "$expected_target"/* ]] || die "backup root is outside approved mount"
}

exec 9>"$lock_file"
flock -n 9 || die "another production operation holds $lock_file"
load_manifest "$manifest_file"
verify_mount_identity

repo_root="${manifest[REPO_ROOT]}"
compose_project="${manifest[COMPOSE_PROJECT]}"
compose_file="${manifest[COMPOSE_FILE]}"
backup_root="${manifest[BACKUP_ROOT]}"
compose=(docker compose --project-name "$compose_project" --file "$compose_file")
[[ "$(git -C "$repo_root" rev-parse HEAD)" == "$target_commit" ]] || die "worktree HEAD does not match --commit"
git -C "$repo_root" diff --quiet || die "worktree has unstaged changes"
git -C "$repo_root" diff --cached --quiet || die "worktree has staged changes"

read -r total_kb used_kb available_kb used_percent < <(
  df -Pk "${manifest[EXPECTED_MOUNT_TARGET]}" | awk 'NR==2 {gsub(/%/, "", $5); print $2, $3, $4, $5}'
)
[[ -n "${total_kb:-}" && "$total_kb" -ge "$min_total_kb" ]] || die "filesystem is smaller than the approved 100G-class baseline"
[[ "$used_percent" -lt "$deploy_block_percent" ]] || die "filesystem use is ${used_percent}%; regular deployment blocks at 70%"
[[ "$available_kb" -ge "$forced_free_kb" ]] || die "forced free-space reserve is below 10G"
[[ -r "$state_file" ]] || die "disk/quiesce state artifact is missing"
grep -qx 'STATE=normal' "$state_file" || die "disk/quiesce state is not normal"
grep -qx 'QUIESCED=0' "$state_file" || die "application is quiesced"
backup_used_kb="$(du -sk "$backup_root" 2>/dev/null | awk '{print $1}' || printf '0\n')"
[[ "${backup_used_kb:-0}" -le "$backup_budget_kb" ]] || die "backup area exceeds its 8G budget"

systemd_active=0
systemctl is-active --quiet "${manifest[SYSTEMD_UNIT]}" && systemd_active=1
mapfile -t compose_apps < <(docker ps -q --filter "label=com.docker.compose.project=$compose_project" --filter 'label=com.docker.compose.service=sub2api')
mapfile -t all_compose_apps < <(docker ps -q --filter 'label=com.docker.compose.service=sub2api')
select_runner "$requested_runner" "${#compose_apps[@]}" "${#all_compose_apps[@]}" "$systemd_active" >/dev/null || die "zero, dual, ambiguous, or requested-runner mismatch"

old_app_image=""; old_app_commit=""; old_postgres_image=""; old_redis_image=""
old_postgres_id=""; old_redis_id=""; old_systemd_binary=""; old_systemd_sha=""; old_systemd_version=""
release_binary=""; target_version=""
if [[ "$requested_runner" == "compose" ]]; then
  export SUB2API_IMAGE="$app_image" SUB2API_RELEASE_COMMIT="$target_commit"
  "${compose[@]}" config --quiet
  resolved_compose_json="$("${compose[@]}" config --format json)"
  mapfile -t resolved_images < <(jq -er '.services | to_entries[] | .value.image' <<<"$resolved_compose_json")
  [[ "${#resolved_images[@]}" -eq 3 ]] || die "expected exactly three resolved production images"
  for image in "${resolved_images[@]}"; do is_digest_ref "$image" || die "compose resolved a mutable image"; done
  resolved_app_image="$(jq -er '.services.sub2api.image' <<<"$resolved_compose_json")"
  resolved_postgres_image="$(jq -er '.services.postgres.image' <<<"$resolved_compose_json")"
  resolved_redis_image="$(jq -er '.services.redis.image' <<<"$resolved_compose_json")"
  [[ "$resolved_app_image" == "$app_image" ]] || die "resolved SUB2API_IMAGE does not match --app-image"
  old_app_image="$(docker inspect --format '{{.Config.Image}}' "${compose_apps[0]}")"
  old_app_commit="$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "${compose_apps[0]}")"
  old_postgres_id="$("${compose[@]}" ps -q postgres)"; old_redis_id="$("${compose[@]}" ps -q redis)"
  [[ -n "$old_postgres_id" && -n "$old_redis_id" ]] || die "postgres and redis containers must already exist"
  old_postgres_image="$(docker inspect --format '{{.Config.Image}}' "$old_postgres_id")"
  old_redis_image="$(docker inspect --format '{{.Config.Image}}' "$old_redis_id")"
  is_digest_ref "$old_app_image" && is_digest_ref "$old_postgres_image" && is_digest_ref "$old_redis_image" || die "running compose images are not immutable"
  [[ "$old_app_commit" =~ ^[0-9a-f]{40}$ ]] || die "running compose revision is not immutable"
  compose_data_images_unchanged "$resolved_postgres_image" "$resolved_redis_image" "$old_postgres_image" "$old_redis_image" || die "Compose data image drift blocks app-only deployment before quiesce"
else
  [[ -f "$app_binary" ]] || die "systemd artifact is not a regular file"
  [[ "$(sha256sum "$app_binary" | awk '{print $1}')" == "$app_sha256" ]] || die "systemd artifact digest mismatch"
  target_version="$("$app_binary" --version 2>&1)"
  grep -Fq "$target_commit" <<<"$target_version" || die "systemd artifact version does not contain target commit"
  old_systemd_binary="$(readlink -f "${manifest[SYSTEMD_CURRENT_LINK]}")"
  [[ -x "$old_systemd_binary" && "$old_systemd_binary" == "${manifest[SYSTEMD_RELEASE_ROOT]}"/* ]] || die "current systemd binary is outside immutable releases"
  old_systemd_sha="$(sha256sum "$old_systemd_binary" | awk '{print $1}')"
  old_systemd_version="$("$old_systemd_binary" --version 2>&1)"
fi

backup_dir=""

probe_health() {
  local attempt
  for attempt in $(seq 1 60); do
    if curl --fail --silent --show-error --max-time 5 "${manifest[HEALTH_URL]}" >/dev/null; then return 0; fi
    sleep 2
  done
  return 1
}

probe_compose_infrastructure() {
  local table_probe
  "${compose[@]}" exec -T postgres sh -ceu 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"' || return 1
  table_probe="$("${compose[@]}" exec -T postgres sh -ceu 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -Atc "SELECT to_regclass('\''public.grok_video_request_owners'\'') IS NOT NULL, to_regclass('\''public.grok_video_create_idempotency'\'') IS NOT NULL, to_regclass('\''public.grok_image_create_idempotency'\'') IS NOT NULL;"')" || return 1
  required_grok_media_tables_present compose "$table_probe" || return 1
  "${compose[@]}" exec -T redis sh -ceu 'redis-cli --no-auth-warning ping' | grep -qx PONG || return 1
}

probe_systemd_infrastructure() {
  local table_probe
  systemctl is-active --quiet "${manifest[POSTGRES_SYSTEMD_UNIT]}" || return 1
  systemctl is-active --quiet "${manifest[REDIS_SYSTEMD_UNIT]}" || return 1
  runuser -u "${manifest[POSTGRES_OS_USER]}" -- pg_isready -U "${manifest[POSTGRES_USER]}" -d "${manifest[POSTGRES_DATABASE]}" || return 1
  table_probe="$(runuser -u "${manifest[POSTGRES_OS_USER]}" -- psql -U "${manifest[POSTGRES_USER]}" -d "${manifest[POSTGRES_DATABASE]}" -v ON_ERROR_STOP=1 -Atc "SELECT to_regclass('public.grok_video_request_owners') IS NOT NULL, to_regclass('public.grok_video_create_idempotency') IS NOT NULL, to_regclass('public.grok_image_create_idempotency') IS NOT NULL;")" || return 1
  required_grok_media_tables_present systemd "$table_probe" || return 1
  redis-cli --no-auth-warning ping | grep -qx PONG || return 1
}

deploy_compose_app() {
  "${compose[@]}" pull --quiet sub2api
  docker image inspect "$app_image" >/dev/null
  probe_compose_infrastructure
  "${compose[@]}" up -d --no-deps --force-recreate sub2api
  app_id="$("${compose[@]}" ps -q sub2api)"
  [[ -n "$app_id" ]] || die "application container was not recreated"
  [[ "$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$app_id")" == "$target_commit" ]] || die "running revision does not match target commit"
  [[ "$(docker inspect --format '{{.Config.Image}}' "$app_id")" == "$app_image" ]] || die "running image does not match target digest"
}

probe_compose_data_services() {
  "${compose[@]}" exec -T postgres sh -ceu 'pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"' || return 1
  "${compose[@]}" exec -T redis sh -ceu 'redis-cli --no-auth-warning ping' | grep -qx PONG || return 1
}

probe_systemd_data_services() {
  systemctl is-active --quiet "${manifest[POSTGRES_SYSTEMD_UNIT]}" || return 1
  systemctl is-active --quiet "${manifest[REDIS_SYSTEMD_UNIT]}" || return 1
  runuser -u "${manifest[POSTGRES_OS_USER]}" -- pg_isready -U "${manifest[POSTGRES_USER]}" -d "${manifest[POSTGRES_DATABASE]}" || return 1
  redis-cli --no-auth-warning ping | grep -qx PONG || return 1
}

recover_compose() {
  export SUB2API_IMAGE="$old_app_image" POSTGRES_IMAGE="$old_postgres_image" REDIS_IMAGE="$old_redis_image" SUB2API_RELEASE_COMMIT="$old_app_commit"
  if ! "${compose[@]}" stop sub2api; then echo "recovery step failed: stop compose app" >&2; return 1; fi
  if ! docker image inspect "$old_app_image" >/dev/null; then echo "recovery step failed: old app image is unavailable" >&2; return 1; fi
  [[ "$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$old_app_image")" == "$old_app_commit" ]] || { echo "recovery step failed: old app image revision mismatch" >&2; return 1; }
  if ! "${compose[@]}" config --quiet; then echo "recovery step failed: restore compose configuration" >&2; return 1; fi
  if ! "${compose[@]}" create --no-build --no-deps --force-recreate sub2api; then echo "recovery step failed: restore stopped compose app artifact" >&2; return 1; fi
  recovered_id="$("${compose[@]}" ps -aq sub2api)"
  [[ -n "$recovered_id" ]] || { echo "recovery step failed: app container missing" >&2; return 1; }
  [[ "$(docker inspect --format '{{.Config.Image}}' "$recovered_id")" == "$old_app_image" ]] || { echo "recovery step failed: old app digest mismatch" >&2; return 1; }
  [[ "$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$recovered_id")" == "$old_app_commit" ]] || { echo "recovery step failed: old app version mismatch" >&2; return 1; }
  [[ -z "$("${compose[@]}" ps --status running -q sub2api)" ]] || { echo "recovery step failed: compose app restarted during recovery" >&2; return 1; }
  if ! probe_compose_data_services; then echo "recovery step failed: live PostgreSQL readiness or Redis PONG" >&2; return 1; fi
}

recover_systemd() {
  if ! systemctl stop "${manifest[SYSTEMD_UNIT]}"; then echo "recovery step failed: stop systemd app" >&2; return 1; fi
  if ! ln -s "$old_systemd_binary" "${manifest[SYSTEMD_CURRENT_LINK]}.rollback.$$"; then echo "recovery step failed: create old release link" >&2; return 1; fi
  if ! mv -Tf "${manifest[SYSTEMD_CURRENT_LINK]}.rollback.$$" "${manifest[SYSTEMD_CURRENT_LINK]}"; then echo "recovery step failed: activate old release link" >&2; return 1; fi
  [[ "$(sha256sum "$(readlink -f "${manifest[SYSTEMD_CURRENT_LINK]}")" | awk '{print $1}')" == "$old_systemd_sha" ]] || { echo "recovery step failed: old app digest mismatch" >&2; return 1; }
  [[ "$("${manifest[SYSTEMD_CURRENT_LINK]}" --version 2>&1)" == "$old_systemd_version" ]] || { echo "recovery step failed: old app version mismatch" >&2; return 1; }
  if systemctl is-active --quiet "${manifest[SYSTEMD_UNIT]}"; then echo "recovery step failed: systemd app restarted during recovery" >&2; return 1; fi
  if ! probe_systemd_data_services; then echo "recovery step failed: live PostgreSQL readiness or Redis PONG" >&2; return 1; fi
}

rollback() {
  local reason="${1:-unknown}"
  (( rollback_started == 0 )) || return 1
  rollback_started=1
  trap - ERR INT TERM
  echo "automatic recovery started: $reason" >&2
  if [[ "$requested_runner" == "compose" ]]; then
    if ! recover_compose; then
      write_state recovery_failed "$requested_runner" 1 recovery_failed
      echo "automatic recovery failed; backups retained and application remains quiesced" >&2
      return 1
    fi
  else
    if ! recover_systemd; then
      write_state recovery_failed "$requested_runner" 1 recovery_failed
      echo "automatic recovery failed; backups retained and application remains quiesced" >&2
      return 1
    fi
  fi
  write_state recovery_failed "$requested_runner" 1 compatibility_unproven
  echo "automatic recovery retained backups and restored only application artifact/config; compatibility/data-safety unproven; application remains quiesced" >&2
  return 1
}

on_error() {
  local line="$1" status="$2"
  if ! rollback "line $line"; then exit 79; fi
  exit "$status"
}
trap 'on_error "$LINENO" "$?"' ERR
trap 'if ! rollback "signal"; then exit 79; fi; exit 130' INT TERM

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="$backup_root/${timestamp}-${target_commit}"
mkdir -p "$backup_dir"
transaction_started=1
write_state deploying "$requested_runner" 1 deploy_quiesce

if [[ "$requested_runner" == "compose" ]]; then
  "${compose[@]}" stop sub2api
  "${compose[@]}" exec -T postgres sh -ceu 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' >"$backup_dir/postgres.dump"
  "${compose[@]}" exec -T redis sh -ceu 'redis-cli --no-auth-warning SAVE >/dev/null'
  docker cp "$old_redis_id:/data/dump.rdb" "$backup_dir/redis.rdb"
  printf 'RUNNER=compose\nOLD_APP_IMAGE=%s\nOLD_APP_COMMIT=%s\nOLD_POSTGRES_IMAGE=%s\nOLD_REDIS_IMAGE=%s\n' "$old_app_image" "$old_app_commit" "$old_postgres_image" "$old_redis_image" >"$backup_dir/release.manifest"
else
  systemctl stop "${manifest[SYSTEMD_UNIT]}"
  runuser -u "${manifest[POSTGRES_OS_USER]}" -- pg_dump -U "${manifest[POSTGRES_USER]}" -d "${manifest[POSTGRES_DATABASE]}" -Fc >"$backup_dir/postgres.dump"
  redis-cli --no-auth-warning SAVE >/dev/null
  install -m 0640 "${manifest[REDIS_RDB_PATH]}" "$backup_dir/redis.rdb"
  printf 'RUNNER=systemd\nOLD_BINARY=%s\nOLD_SHA256=%s\nOLD_VERSION_SHA256=%s\n' "$old_systemd_binary" "$old_systemd_sha" "$(printf '%s' "$old_systemd_version" | sha256sum | awk '{print $1}')" >"$backup_dir/release.manifest"
fi
sha256sum "$backup_dir/postgres.dump" "$backup_dir/redis.rdb" "$backup_dir/release.manifest" >"$backup_dir/SHA256SUMS"
tar -C "$backup_dir" -cf "$backup_dir/metadata.tar" release.manifest SHA256SUMS
failpoint after_backup

if [[ "$requested_runner" == "compose" ]]; then
  deploy_compose_app
else
  release_dir="${manifest[SYSTEMD_RELEASE_ROOT]}/$target_commit"
  release_binary="$release_dir/${manifest[SYSTEMD_BINARY_NAME]}"
  if [[ -e "$release_binary" ]]; then
    [[ "$(sha256sum "$release_binary" | awk '{print $1}')" == "$app_sha256" ]] || die "existing immutable release digest mismatch"
  else
    install -D -m 0755 "$app_binary" "$release_binary"
  fi
  ln -s "$release_binary" "${manifest[SYSTEMD_CURRENT_LINK]}.next.$$"
  mv -Tf "${manifest[SYSTEMD_CURRENT_LINK]}.next.$$" "${manifest[SYSTEMD_CURRENT_LINK]}"
  systemctl start "${manifest[POSTGRES_SYSTEMD_UNIT]}" "${manifest[REDIS_SYSTEMD_UNIT]}" "${manifest[SYSTEMD_UNIT]}"
  [[ "$(sha256sum "$(readlink -f "${manifest[SYSTEMD_CURRENT_LINK]}")" | awk '{print $1}')" == "$app_sha256" ]] || die "running systemd binary digest mismatch"
  [[ "$("${manifest[SYSTEMD_CURRENT_LINK]}" --version 2>&1)" == "$target_version" ]] || die "running systemd binary version mismatch"
fi
failpoint after_migration_restart

probe_health || die "health probe failed"
if [[ "$requested_runner" == "compose" ]]; then probe_compose_infrastructure || die "compose infrastructure probe failed"; else probe_systemd_infrastructure || die "systemd infrastructure probe failed"; fi
failpoint after_infrastructure_probes

smoke_base="${manifest[HEALTH_URL]%/health}"
smoke_key="deploy-${target_commit}"
smoke_payload="$(jq -nc --arg model "${SUB2API_SMOKE_MODEL:-grok-imagine-video}" --arg prompt "${SUB2API_SMOKE_PROMPT:-deployment health probe}" '{model:$model,prompt:$prompt,duration:5,resolution:"480p"}')"
create_response="$(curl --fail --silent --show-error --max-time 120 -H "Authorization: Bearer $SUB2API_SMOKE_API_KEY" -H 'Content-Type: application/json' -H "Idempotency-Key: $smoke_key" --data "$smoke_payload" "$smoke_base/v1/videos/generations")"
request_id="$(jq -er '.request_id // .id' <<<"$create_response")"
[[ -n "$request_id" ]] || die "create probe did not return a request id"
deadline=$((SECONDS + ${SUB2API_SMOKE_TIMEOUT_SECONDS:-900}))
status=""
while (( SECONDS < deadline )); do
  status_response="$(curl --fail --silent --show-error --max-time 30 -H "Authorization: Bearer $SUB2API_SMOKE_API_KEY" "$smoke_base/v1/videos/$request_id")"
  status="$(jq -r '.status // empty' <<<"$status_response")"
  case "$status" in completed|succeeded|success) break ;; failed|error|cancelled) die "status probe reached terminal failure" ;; esac
  sleep 10
done
[[ "$status" =~ ^(completed|succeeded|success)$ ]] || die "status probe timed out"
curl --fail --silent --show-error --max-time 60 -H "Authorization: Bearer $SUB2API_SMOKE_API_KEY" -H 'Range: bytes=0-0' "$smoke_base/v1/videos/$request_id/content" >/dev/null
failpoint after_paid_probes

grep -Eq 'roll_size[[:space:]]+50mb' "${manifest[EXPECTED_MOUNT_TARGET]}/deploy/Caddyfile" || die "Caddy log roll_size is not 50MB"
grep -Eq 'roll_keep[[:space:]]+10' "${manifest[EXPECTED_MOUNT_TARGET]}/deploy/Caddyfile" || die "Caddy log roll_keep is not 10"
journalctl --disk-usage
systemd-analyze cat-config systemd/journald.conf | grep -Eq 'SystemMaxUse[[:space:]]*=[[:space:]]*4G' || die "journald SystemMaxUse=4G was not read back"
systemd-analyze cat-config systemd/journald.conf | grep -Eq 'RuntimeMaxUse[[:space:]]*=[[:space:]]*512M' || die "journald RuntimeMaxUse=512M was not read back"
systemctl is-enabled --quiet sub2api-disk-guard.timer
systemctl is-active --quiet sub2api-disk-guard.timer
systemctl show sub2api-disk-guard.timer -p ActiveState -p LastTriggerUSec -p NextElapseUSecRealtime
df -hP "${manifest[EXPECTED_MOUNT_TARGET]}"
du -xhd1 "${manifest[EXPECTED_MOUNT_TARGET]}"
failpoint after_policy_readback

write_state normal "$requested_runner" 0 deploy_verified
transaction_started=0
trap - ERR INT TERM
echo "production deploy verified: runner=$requested_runner commit=$target_commit request_id=$request_id"
