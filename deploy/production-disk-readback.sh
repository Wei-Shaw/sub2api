#!/usr/bin/env bash
set -Eeuo pipefail

readonly lock_file="/run/lock/sub2api-prod-deploy.lock"
readonly state_file="/run/sub2api-prod-quiesce.state"
readonly manifest_file="/etc/sub2api/production-deploy.conf"
readonly min_total_kb=94371840
readonly forced_free_kb=10485760

die() {
  echo "disk guard failed: $*" >&2
  exit 2
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

valid_absolute_path() {
  [[ "$1" == /* && "$1" != *$'\n'* && "$1" != *$'\r'* && "$1" != *'..'* ]]
}

declare -A manifest=()
load_manifest() {
  local path="$1" line key value
  [[ -r "$path" ]] || die "required non-secret manifest is unreadable: $path"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=([^[:space:]]+)$ ]] || die "invalid manifest line"
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
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
}

write_state() {
  local state="$1" runner="$2" quiesced="$3" used_percent="$4" reason="$5"
  local temp_file="${state_file}.tmp.$$"
  umask 022
  {
    printf 'STATE=%s\n' "$state"
    printf 'RUNNER=%s\n' "$runner"
    printf 'QUIESCED=%s\n' "$quiesced"
    printf 'USED_PERCENT=%s\n' "$used_percent"
    printf 'REASON=%s\n' "$reason"
    printf 'UPDATED_AT=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } >"$temp_file"
  chmod 0644 "$temp_file"
  mv -f "$temp_file" "$state_file"
}

read_previous_state() {
  previous_state="normal"
  previous_runner=""
  previous_quiesced="0"
  [[ -r "$state_file" ]] || return 0
  local line key value
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^([A-Z_]+)=([A-Za-z0-9_.:/@+-]*)$ ]] || die "invalid state artifact"
    key="${BASH_REMATCH[1]}"
    value="${BASH_REMATCH[2]}"
    case "$key" in
      STATE) previous_state="$value" ;;
      RUNNER) previous_runner="$value" ;;
      QUIESCED) previous_quiesced="$value" ;;
    esac
  done <"$state_file"
  case "$previous_state" in normal|alert|block|incident|deploying|recovery_failed) ;; *) die "unknown prior state" ;; esac
  case "$previous_runner" in ""|compose|systemd) ;; *) die "unknown prior runner" ;; esac
  [[ "$previous_quiesced" == "0" || "$previous_quiesced" == "1" ]] || die "invalid prior quiesce flag"
}

compute_state() {
  local prior="$1" percent="$2" next="normal"
  case "$prior" in
    recovery_failed|deploying)
      printf '%s\n' "$prior"
      return
      ;;
    incident) (( percent < 70 )) || next="incident" ;;
    block) (( percent < 75 )) || next="block" ;;
  esac
  if (( percent >= 90 )); then
    next="incident"
  elif (( percent >= 80 )) && [[ "$next" != "incident" ]]; then
    next="block"
  elif (( percent >= 70 )) && [[ "$next" == "normal" ]]; then
    next="alert"
  fi
  printf '%s\n' "$next"
}

select_detected_runner() {
  local compose_count="$1" all_compose_count="$2" systemd_active="$3"
  (( compose_count >= 0 && all_compose_count >= 0 )) || return 1
  (( compose_count <= 1 && all_compose_count == compose_count )) || return 1
  (( systemd_active == 0 || systemd_active == 1 )) || return 1
  local active_count=$((compose_count + systemd_active))
  (( active_count == 1 )) || return 1
  if (( compose_count == 1 )); then printf 'compose\n'; else printf 'systemd\n'; fi
}

if [[ "${SUB2API_DISK_GUARD_TEST_MODE:-0}" == "1" ]]; then
  case "${1:-}" in
    --self-test-transition)
      [[ "$#" -eq 3 && "$3" =~ ^[0-9]+$ ]] || exit 64
      compute_state "$2" "$3"
      exit 0
      ;;
    --self-test-runner)
      [[ "$#" -eq 4 ]] || exit 64
      select_detected_runner "$2" "$3" "$4"
      exit $?
      ;;
    --self-test-action)
      [[ "$#" -eq 3 ]] || exit 64
      case "$2:$3" in
        stop:compose|stop:systemd|start:compose|start:systemd) printf '%s %s\n' "$2" "$3" ;;
        *) exit 64 ;;
      esac
      exit 0
      ;;
    *) exit 64 ;;
  esac
fi

for command_name in flock df du awk journalctl docker find grep dirname systemctl findmnt blkid readlink date chmod mv curl seq sleep; do
  require_command "$command_name"
done
docker compose version >/dev/null 2>&1 || die "docker compose plugin is unavailable"
docker info >/dev/null 2>&1 || die "Docker daemon is unavailable for runner detection"

exec 9>"$lock_file"
flock -n 9 || die "deployment or another disk guard holds $lock_file"
load_manifest "$manifest_file"

expected_target="${manifest[EXPECTED_MOUNT_TARGET]}"
expected_source="${manifest[EXPECTED_MOUNT_SOURCE]}"
actual_target="$(findmnt -T "$expected_target" -n -o TARGET --first)"
actual_source="$(findmnt -T "$expected_target" -n -o SOURCE --first)"
[[ "$actual_target" == "$expected_target" ]] || die "approved mount target mismatch"
[[ "$(readlink -f "$actual_source")" == "$(readlink -f "$expected_source")" ]] || die "approved mount source mismatch"
actual_uuid="$(blkid -s UUID -o value "$(readlink -f "$actual_source")")"
[[ "${actual_uuid,,}" == "${manifest[EXPECTED_MOUNT_UUID],,}" ]] || die "approved mount UUID mismatch"

compose_project="${manifest[COMPOSE_PROJECT]}"
compose_file="${manifest[COMPOSE_FILE]}"
compose=(docker compose --project-name "$compose_project" --file "$compose_file")
systemd_active=0
systemctl is-active --quiet "${manifest[SYSTEMD_UNIT]}" && systemd_active=1
mapfile -t compose_apps < <(docker ps -q --filter "label=com.docker.compose.project=$compose_project" --filter 'label=com.docker.compose.service=sub2api')
mapfile -t all_compose_apps < <(docker ps -q --filter 'label=com.docker.compose.service=sub2api')

read_previous_state
detected_runner=""
if detected_runner="$(select_detected_runner "${#compose_apps[@]}" "${#all_compose_apps[@]}" "$systemd_active")"; then
  :
elif [[ "$previous_quiesced" == "1" && -n "$previous_runner" && "${#compose_apps[@]}" -eq 0 && "${#all_compose_apps[@]}" -eq 0 && "$systemd_active" -eq 0 ]]; then
  detected_runner="$previous_runner"
else
  die "zero, dual, or ambiguous active runners"
fi

read -r total_kb used_kb available_kb used_percent < <(
  df -Pk "$expected_target" | awk 'NR==2 {gsub(/%/, "", $5); print $2, $3, $4, $5}'
)
[[ -n "${total_kb:-}" && "$total_kb" -ge "$min_total_kb" ]] || die "filesystem is below the approved 100G-class baseline"
state="$(compute_state "$previous_state" "$used_percent")"
[[ "$state" != "deploying" ]] || die "stale deploying state requires operator recovery"
[[ "$state" != "recovery_failed" ]] || die "recovery_failed state requires operator recovery"

stop_runner() {
  case "$1" in
    compose) "${compose[@]}" stop sub2api ;;
    systemd) systemctl stop "${manifest[SYSTEMD_UNIT]}" ;;
    *) return 2 ;;
  esac
}

start_runner() {
  case "$1" in
    compose) "${compose[@]}" up -d --no-deps sub2api ;;
    systemd) systemctl start "${manifest[SYSTEMD_UNIT]}" ;;
    *) return 2 ;;
  esac
}

runner_is_active() {
  case "$1" in
    compose)
      local active_apps all_active_apps
      active_apps="$(docker ps -q --filter "label=com.docker.compose.project=$compose_project" --filter 'label=com.docker.compose.service=sub2api')" || return 2
      all_active_apps="$(docker ps -q --filter 'label=com.docker.compose.service=sub2api')" || return 2
      [[ -n "$active_apps" && "$active_apps" != *$'\n'* && "$all_active_apps" == "$active_apps" ]]
      ;;
    systemd) [[ "$(systemctl show "${manifest[SYSTEMD_UNIT]}" -p ActiveState --value)" == "active" ]] ;;
    *) return 2 ;;
  esac
}

runner_is_inactive() {
  case "$1" in
    compose)
      local active_apps all_active_apps
      active_apps="$(docker ps -q --filter "label=com.docker.compose.project=$compose_project" --filter 'label=com.docker.compose.service=sub2api')" || return 2
      all_active_apps="$(docker ps -q --filter 'label=com.docker.compose.service=sub2api')" || return 2
      [[ -z "$active_apps" && -z "$all_active_apps" ]]
      ;;
    systemd)
      local active_state
      active_state="$(systemctl show "${manifest[SYSTEMD_UNIT]}" -p ActiveState --value)" || return 2
      [[ "$active_state" == "inactive" || "$active_state" == "failed" ]]
      ;;
    *) return 2 ;;
  esac
}

health_is_ready() {
  local attempt
  for attempt in $(seq 1 10); do
    if curl --fail --silent --show-error --max-time 5 "${manifest[HEALTH_URL]}" >/dev/null; then return 0; fi
    sleep 2
  done
  return 1
}

quiesced="$previous_quiesced"
if [[ "$state" == "block" || "$state" == "incident" ]]; then
  runner_active=0
  if runner_is_active "$detected_runner"; then
    runner_active=1
  else
    runner_check_status=$?
    (( runner_check_status == 1 )) || die "unable to read runner state before quiesce"
  fi
  if [[ "$quiesced" != "1" || "$runner_active" == "1" ]]; then
    if ! stop_runner "$detected_runner"; then
      echo "disk guard: stop command failed; verifying runner state" >&2
    fi
    if ! runner_is_inactive "$detected_runner"; then
      write_state recovery_failed "$detected_runner" 0 "$used_percent" quiesce_stop_failed
      die "recovery failed: unable to verify all application intake stopped"
    fi
  fi
  quiesced=1
elif [[ "$quiesced" == "1" ]]; then
  if ! start_runner "$detected_runner"; then
    write_state recovery_failed "$detected_runner" 1 "$used_percent" hysteresis_restart_failed
    die "recovery failed: application remains quiesced"
  fi
  if ! runner_is_active "$detected_runner" || ! health_is_ready; then
    if ! stop_runner "$detected_runner"; then
      echo "disk guard: recovery verification failed and re-quiesce command also failed" >&2
    fi
    write_state recovery_failed "$detected_runner" 1 "$used_percent" hysteresis_verification_failed
    die "recovery failed: restart verification failed; application remains quiesced"
  fi
  quiesced=0
fi
write_state "$state" "$detected_runner" "$quiesced" "$used_percent" disk_threshold

size_kb() {
  local total=0 path
  for path in "$@"; do
    if [[ -e "$path" ]]; then total=$((total + $(du -sk "$path" | awk '{print $1}'))); fi
  done
  printf '%s\n' "$total"
}

check_budget() {
  local name="$1" limit_kb="$2" actual_kb
  shift 2
  actual_kb="$(size_kb "$@")"
  printf 'budget name=%s used_kb=%s limit_kb=%s\n' "$name" "$actual_kb" "$limit_kb"
  (( actual_kb <= limit_kb )) || die "$name exceeds budget"
}

deploy_root="$expected_target"
check_budget os_runtime 20971520 /usr /var/cache /var/lib/dpkg /var/lib/containerd
check_budget postgres_redis 26214400 "$deploy_root/postgres_data" "$deploy_root/redis_data" /var/lib/postgresql /var/lib/redis
check_budget releases 8388608 "${manifest[SYSTEMD_RELEASE_ROOT]}" /var/lib/docker/image /var/lib/docker/overlay2
check_budget media_temp 20971520 "$deploy_root/data/media-temp" "$deploy_root/data/tmp"
check_budget app_logs 5242880 "$deploy_root/data/logs" /var/lib/docker/containers /var/log/caddy
check_budget journald 4194304 /var/log/journal /run/log/journal
check_budget backups 8388608 "${manifest[BACKUP_ROOT]}"
(( available_kb >= forced_free_kb )) || die "forced free-space reserve is below 10G; no automatic reclaim is authorized"

journalctl --disk-usage
if [[ "$detected_runner" == "compose" && "$quiesced" == "0" ]]; then
  app_id="${compose_apps[0]}"
  log_driver="$(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "$app_id")"
  log_size="$(docker inspect --format '{{index .HostConfig.LogConfig.Config "max-size"}}' "$app_id")"
  log_files="$(docker inspect --format '{{index .HostConfig.LogConfig.Config "max-file"}}' "$app_id")"
  log_compress="$(docker inspect --format '{{index .HostConfig.LogConfig.Config "compress"}}' "$app_id")"
  [[ "$log_driver" == "json-file" && "$log_size" == "100m" && "$log_files" == "10" && "$log_compress" == "true" ]] || die "Sub2API log rotation drifted"
  log_path="$(docker inspect --format '{{.LogPath}}' "$app_id")"
  if [[ -n "$log_path" ]] && find "$(dirname "$log_path")" -maxdepth 1 -type f -name '*.log.*' -mtime +7 -print -quit | grep -q .; then
    die "rotated log exceeds the 7-day ceiling; exact-target approval is required"
  fi
fi
grep -Eq 'roll_size[[:space:]]+50mb' "$deploy_root/deploy/Caddyfile" || die "Caddy roll_size drifted"
grep -Eq 'roll_keep[[:space:]]+10' "$deploy_root/deploy/Caddyfile" || die "Caddy roll_keep drifted"
df -hP "$expected_target"
du -xhd1 "$expected_target"
echo "disk guard state=$state runner=$detected_runner quiesced=$quiesced used_percent=$used_percent available_kb=$available_kb"

case "$state" in incident|block) exit 2 ;; alert) exit 1 ;; esac
