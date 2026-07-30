#!/usr/bin/env bash

set -euo pipefail
umask 077

SOURCE_DIR=""
ASSET_DIR=""
NGINX_SITE=""
NGINX_PROBE_URL=""
NGINX_PROBE_HOST=""
INSTALL_DIR="/opt/sub2api"
CONTAINER_NAME="sub2api"
SOCKET_GID=""
SOCKET_GID_SET=0
SOCKET_GROUP_NAME="sub2api-deployer"
SOCKET_GROUP_CREATED=0
DEPLOYER_BINARY=""
DEPLOYER_CHECKSUMS=""
DOCKER_CONFIG_SOURCE=""
ACTIVATION_VERSION=""
ALLOW_SOURCE_BUILD=0
IMAGE_REPOSITORY="ghcr.io/ssharkkky/sub2api"
IMAGE_SOURCE_LABEL="https://github.com/ssharkkky/sub2api"
UPDATE_PROTOCOL_LABEL="2"
TEMP_DIR=""
COMMITTED=0
MUTATION_STARTED=0
ROLLBACK_RUNNING=0
ROLLBACK_INCOMPLETE=0
SERVICE_WAS_ENABLED=0
SERVICE_WAS_ACTIVE=0
UPGRADE_TIMER_WAS_ENABLED=0
UPGRADE_TIMER_WAS_ACTIVE=0
UPGRADE_TIMER_EXISTED=0
UPGRADE_SERVICE_WAS_ACTIVE=0
ACTIVATOR_FROZEN=0

CONFIG_DIR="/etc/sub2api-deployer"
CONFIG_FILE="$CONFIG_DIR/config.json"
DOCKER_CONFIG_DIR="$CONFIG_DIR/docker"
DOCKER_CONFIG_FILE="$DOCKER_CONFIG_DIR/config.json"
SERVICE_FILE="/etc/systemd/system/sub2api-deployer.service"
UPGRADE_SERVICE_FILE="/etc/systemd/system/sub2api-deployer-upgrade.service"
UPGRADE_TIMER_FILE="/etc/systemd/system/sub2api-deployer-upgrade.timer"
TMPFILES_FILE="/etc/tmpfiles.d/sub2api-deployer.conf"
RUNTIME_PRESERVE_DROPIN="/run/systemd/system/sub2api-deployer.service.d/10-preserve-runtime.conf"
INSTALLED_BINARY="/usr/local/sbin/sub2api-deployer"
INSTALLED_UPGRADER="/usr/local/sbin/sub2api-deployer-upgrade"
STATE_DIR="/var/lib/sub2api-deployer"
CONTROL_PLANE_UPGRADE_REQUEST="$STATE_DIR/control-plane-upgrade.json"
IMAGE_STATE_FILE="$STATE_DIR/image.env"
RUNTIME_DIR="$STATE_DIR/runtime"
RUNTIME_MARKER="$RUNTIME_DIR/active-slot"
STATE_FILE="$STATE_DIR/state.json"
NGINX_STATE_DIR="$STATE_DIR/nginx"
MANAGED_UPSTREAM_FILE="$NGINX_STATE_DIR/managed-upstream.conf"
NGINX_LOADER_FILE="/etc/nginx/conf.d/sub2api-managed-upstream.conf"
INSTALL_LOCK_FILE="/run/sub2api-deployer-install.lock"
INSTALL_LOCK_FD=""
STATE_LOCK_FD=""
ACTIVATION_LOCK_FD=""
TARGET_ACTIVATION_LOCK_FD=""
SOCKET_DIRECTORY="/run/sub2api-deployer"
SOCKET_DIRECTORY_INODE=""

declare -a BACKUP_TARGETS=()
declare -a BACKUP_FILES=()
declare -a BACKUP_EXISTED=()
declare -a DIRECTORY_TARGETS=()
declare -a DIRECTORY_EXISTED=()
declare -a DIRECTORY_MODES=()
declare -a DIRECTORY_OWNERS=()

usage() {
  cat <<'EOF'
Usage: sudo install-sub2api-deployer.sh --nginx-site <site-file> --nginx-probe-url <url> [options]

Options:
  --assets-dir <path>          Directory containing the packaged deploy assets
  --source <repo>              Source checkout used only to build the deployer locally
  --install-dir <path>          Compose working directory (default: /opt/sub2api)
  --container <name>            Current application container (default: sub2api)
  --socket-gid <gid>            Supplementary GID granted to app containers
  --nginx-probe-url <url>       Required loopback Nginx health URL (for example http://127.0.0.1/health)
  --nginx-probe-host <host>     Optional Host header used to select the Nginx virtual host
  --deployer-binary <path>      Prebuilt release binary (recommended)
  --deployer-checksums <path>   Release checksum file for the prebuilt binary
  --docker-config <path>        Optional Docker config.json for private registries
  --activation-version <ver>    Version shown in the post-install UI deployment reminder
  --allow-source-build          Explicitly permit a dev-identity source build fallback

With a prebuilt binary, assets default to the deploy directory beside this
installer. When no prebuilt binary is supplied, --source is required and the
installer builds cmd/deployer with Go. Fresh installs create a dedicated
sub2api-deployer system group unless --socket-gid is supplied; upgrades retain
the configured GID. Installation uses best-effort
transactional rollback for files, Nginx, and the previous systemd service state;
an incomplete rollback preserves its recovery backups. The running application
container is never recreated or stopped.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source) SOURCE_DIR="${2:-}"; shift 2 ;;
    --assets-dir) ASSET_DIR="${2:-}"; shift 2 ;;
    --nginx-site) NGINX_SITE="${2:-}"; shift 2 ;;
    --nginx-probe-url) NGINX_PROBE_URL="${2:-}"; shift 2 ;;
    --nginx-probe-host) NGINX_PROBE_HOST="${2:-}"; shift 2 ;;
    --install-dir) INSTALL_DIR="${2:-}"; shift 2 ;;
    --container) CONTAINER_NAME="${2:-}"; shift 2 ;;
    --socket-gid) SOCKET_GID="${2:-}"; SOCKET_GID_SET=1; shift 2 ;;
    --deployer-binary) DEPLOYER_BINARY="${2:-}"; shift 2 ;;
    --deployer-checksums) DEPLOYER_CHECKSUMS="${2:-}"; shift 2 ;;
    --docker-config) DOCKER_CONFIG_SOURCE="${2:-}"; shift 2 ;;
    --activation-version) ACTIVATION_VERSION="${2:-}"; shift 2 ;;
    --allow-source-build) ALLOW_SOURCE_BUILD=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

fail() {
  echo "$*" >&2
  exit 1
}

reject_json_unsafe() {
  local name="$1"
  local value="$2"
  if [[ "$value" == *'"'* || "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
    fail "$name contains characters that cannot be safely written to JSON"
  fi
}

validate_container_name() {
  local label="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]]; then
    fail "$label must match ^[A-Za-z0-9][A-Za-z0-9_.-]*$"
  fi
}

validate_docker_server_version() {
  local version major minor
  version=$(docker version --format '{{.Server.Version}}') || fail "Could not read the Docker server version"
  if [[ ! "$version" =~ ^([0-9]+)\.([0-9]+)(\.|$) ]]; then
    fail "Docker server returned an unsupported version string: $version"
  fi
  major=${BASH_REMATCH[1]}
  minor=${BASH_REMATCH[2]}
  if (( major < 20 || (major == 20 && minor < 10) )); then
    fail "Docker server 20.10 or newer is required (found $version)"
  fi
}

normalize_compose_path() {
  local base="$1"
  local value="$2"
  if [[ "$value" == /* ]]; then
    readlink -f -- "$value"
  else
    readlink -f -- "$base/$value"
  fi
}

acquire_root_lock() {
  local lock_file="$1"
  local fd_variable="$2"
  local lock_dir
  lock_dir=$(dirname -- "$lock_file")
  [[ -d "$lock_dir" && ! -L "$lock_dir" ]] || fail "Lock directory is unavailable or unsafe: $lock_dir"
  if [[ -e "$lock_file" || -L "$lock_file" ]]; then
    [[ -f "$lock_file" && ! -L "$lock_file" ]] || fail "Lock path is not a regular file: $lock_file"
    [[ $(stat -c '%u' "$lock_file") == 0 ]] || fail "Lock file is not owned by root: $lock_file"
  fi
  local fd
  exec {fd}<>"$lock_file"
  chmod 0600 "$lock_file"
  [[ $(stat -c '%u:%a' "$lock_file") == "0:600" ]] || fail "Lock file is not root-only: $lock_file"
  if ! flock -n "$fd"; then
    exec {fd}>&-
    fail "Another deployer installation or state owner holds $lock_file"
  fi
  printf -v "$fd_variable" '%s' "$fd"
}

try_acquire_root_lock() {
  local lock_file="$1"
  local fd_variable="$2"
  local lock_dir fd
  lock_dir=$(dirname -- "$lock_file")
  [[ -d "$lock_dir" && ! -L "$lock_dir" ]] || return 1
  if [[ -e "$lock_file" || -L "$lock_file" ]]; then
    [[ -f "$lock_file" && ! -L "$lock_file" ]] || return 1
    [[ $(stat -c '%u' "$lock_file") == 0 ]] || return 1
  fi
  exec {fd}<>"$lock_file" || return 1
  if ! chmod 0600 "$lock_file" || [[ $(stat -c '%u:%a' "$lock_file") != "0:600" ]] || ! flock -n "$fd"; then
    exec {fd}>&-
    return 1
  fi
  printf -v "$fd_variable" '%s' "$fd"
}

prove_deployer_stopped_for_restore() {
  local active_state main_pid socket_path lock_file
  if ! systemctl stop sub2api-deployer.service >/dev/null 2>&1; then
    echo "CRITICAL: could not stop the partially installed deployer; automatic file restoration was not attempted" >&2
    return 1
  fi
  active_state=$(systemctl show --property=ActiveState --value sub2api-deployer.service 2>/dev/null) || {
    echo "CRITICAL: could not verify deployer ActiveState; automatic file restoration was not attempted" >&2
    return 1
  }
  case "$active_state" in
    inactive|failed) ;;
    *)
      echo "CRITICAL: deployer ActiveState is $active_state; automatic file restoration was not attempted" >&2
      return 1
      ;;
  esac
  main_pid=$(systemctl show --property=MainPID --value sub2api-deployer.service 2>/dev/null) || {
    echo "CRITICAL: could not verify deployer MainPID; automatic file restoration was not attempted" >&2
    return 1
  }
  if [[ "$main_pid" != 0 ]]; then
    echo "CRITICAL: deployer MainPID is $main_pid; automatic file restoration was not attempted" >&2
    return 1
  fi
  socket_path="${OLD_SOCKET_PATH:-/run/sub2api-deployer/deployer.sock}"
  if curl --fail --silent --max-time 2 --unix-socket "$socket_path" http://localhost/v1/health >/dev/null 2>&1; then
    echo "CRITICAL: deployer socket $socket_path still responds; automatic file restoration was not attempted" >&2
    return 1
  fi
  if [[ -z "$STATE_LOCK_FD" ]]; then
    lock_file="$(dirname -- "${OLD_STATE_FILE:-$STATE_FILE}")/deployer.lock"
    if ! try_acquire_root_lock "$lock_file" STATE_LOCK_FD; then
      echo "CRITICAL: deployer state lock $lock_file is still held; automatic file restoration was not attempted" >&2
      return 1
    fi
  fi
}

release_state_lock() {
  if [[ -n "$STATE_LOCK_FD" ]]; then
    flock -u "$STATE_LOCK_FD" >/dev/null 2>&1 || true
    exec {STATE_LOCK_FD}>&-
    STATE_LOCK_FD=""
  fi
}

release_activation_lock() {
  if [[ -n "$ACTIVATION_LOCK_FD" ]]; then
    flock -u "$ACTIVATION_LOCK_FD" >/dev/null 2>&1 || true
    exec {ACTIVATION_LOCK_FD}>&-
    ACTIVATION_LOCK_FD=""
  fi
  if [[ -n "$TARGET_ACTIVATION_LOCK_FD" ]]; then
    flock -u "$TARGET_ACTIVATION_LOCK_FD" >/dev/null 2>&1 || true
    exec {TARGET_ACTIVATION_LOCK_FD}>&-
    TARGET_ACTIVATION_LOCK_FD=""
  fi
}

validate_effective_activator_exec() {
  local effective="$1"
  perl -e '
    use strict; use warnings;
    my $value = do { local $/; <STDIN> };
    exit 1 unless (() = $value =~ /argv\[\]=/g) == 1;
    exit 1 unless (() = $value =~ /(?:^|[;{]\s*)path=/g) == 1;
    my ($path) = $value =~ /(?:^|[;{]\s*)path=([^;]*?)\s*;/;
    my ($argv) = $value =~ /(?:^|[;{]\s*)argv\[\]=([^;]*?)\s*;/;
    exit 1 unless defined($path) && defined($argv);
    exit 1 unless $path eq "/usr/local/sbin/sub2api-deployer";
    exit 1 unless $argv eq "/usr/local/sbin/sub2api-deployer --activate-staged-control-plane";
  ' <<<"$effective"
}

restore_frozen_activator_state() {
  if (( UPGRADE_TIMER_WAS_ACTIVE == 1 )); then
    systemctl start sub2api-deployer-upgrade.timer >/dev/null 2>&1 || true
  else
    systemctl stop sub2api-deployer-upgrade.timer >/dev/null 2>&1 || true
  fi
  if (( UPGRADE_SERVICE_WAS_ACTIVE == 1 )); then
    systemctl start sub2api-deployer-upgrade.service >/dev/null 2>&1 || true
  fi
}

assert_container_name_id() {
  local container="$1"
  local expected_id="$2"
  local actual_id
  actual_id=$(docker inspect "$container" --format '{{.Id}}') || \
    fail "Container name $container could not be revalidated"
  [[ "$actual_id" == "$expected_id" ]] || \
    fail "Container name $container no longer resolves to immutable ID $expected_id"
}

load_single_loopback_port() {
  local container_id="$1"
  local output
  output=$(docker port "$container_id" 8080/tcp) || \
    fail "Could not inspect the port mapping for container $container_id"
  if [[ "$output" == *$'\n'* ]] || ! [[ "$output" =~ ^127\.0\.0\.1:([0-9]+)$ ]]; then
    fail "Container $container_id must expose 8080/tcp through exactly one 127.0.0.1 host mapping"
  fi
  INSPECTED_PORT="${BASH_REMATCH[1]}"
  if (( 10#$INSPECTED_PORT < 1 || 10#$INSPECTED_PORT > 65535 )); then
    fail "Container $container_id returned an invalid loopback host port"
  fi
}

load_container_runtime() {
  local container_id="$1"
  local version_output
  INSPECTED_IMAGE=$(docker inspect "$container_id" --format '{{.Config.Image}}') || \
    fail "Could not inspect the image for container $container_id"
  [[ -n "$INSPECTED_IMAGE" ]] || fail "Container $container_id has an empty Config.Image"
  version_output=$(docker exec "$container_id" /app/sub2api --version 2>&1) || \
    fail "Could not execute /app/sub2api --version in container $container_id"
  INSPECTED_VERSION=$(sed -n 's/.*Sub2API \([^ ]*\).*/\1/p' <<<"$version_output" | tail -1)
  if [[ -z "$INSPECTED_VERSION" ]] || ! [[ "$INSPECTED_VERSION" =~ ^[0-9][0-9A-Za-z.-]{0,63}$ ]]; then
    fail "Container $container_id returned an invalid application version"
  fi
}

backup_target() {
  local target="$1"
  local existing
  for existing in "${BACKUP_TARGETS[@]:-}"; do
    [[ "$existing" == "$target" ]] && return
  done
  local index=${#BACKUP_TARGETS[@]}
  BACKUP_TARGETS+=("$target")
  BACKUP_FILES+=("$TEMP_DIR/backup-$index")
  if [[ -e "$target" || -L "$target" ]]; then
    BACKUP_EXISTED+=(1)
    cp -a -- "$target" "${BACKUP_FILES[$index]}"
  else
    BACKUP_EXISTED+=(0)
  fi
}

restore_backups() {
  local index target failed=0
  for ((index=${#BACKUP_TARGETS[@]} - 1; index >= 0; index--)); do
    target="${BACKUP_TARGETS[$index]}"
    if [[ "${BACKUP_EXISTED[$index]}" == 1 ]]; then
      if ! mkdir -p -- "$(dirname -- "$target")" || \
        ! rm -f -- "$target" || \
        ! cp -a -- "${BACKUP_FILES[$index]}" "$target"; then
        echo "CRITICAL: failed to restore $target" >&2
        failed=1
      fi
    else
      if ! rm -f -- "$target"; then
        echo "CRITICAL: failed to remove newly installed file $target" >&2
        failed=1
      fi
    fi
  done
  return "$failed"
}

backup_directory_metadata() {
  local target="$1"
  DIRECTORY_TARGETS+=("$target")
  if [[ -d "$target" && ! -L "$target" ]]; then
    DIRECTORY_EXISTED+=(1)
    DIRECTORY_MODES+=("$(stat -c '%a' "$target")")
    DIRECTORY_OWNERS+=("$(stat -c '%u:%g' "$target")")
  else
    DIRECTORY_EXISTED+=(0)
    DIRECTORY_MODES+=("")
    DIRECTORY_OWNERS+=("")
  fi
}

restore_directory_metadata() {
  local index target failed=0
  for ((index=${#DIRECTORY_TARGETS[@]} - 1; index >= 0; index--)); do
    target="${DIRECTORY_TARGETS[$index]}"
    if [[ "${DIRECTORY_EXISTED[$index]}" == 1 ]]; then
      if ! chown "${DIRECTORY_OWNERS[$index]}" "$target" || ! chmod "${DIRECTORY_MODES[$index]}" "$target"; then
        echo "CRITICAL: failed to restore directory metadata for $target" >&2
        failed=1
      fi
    elif [[ -d "$target" ]] && ! rmdir -- "$target" 2>/dev/null; then
      echo "WARNING: newly created directory remains non-empty: $target" >&2
    fi
  done
  return "$failed"
}

rollback_install() {
  (( ROLLBACK_RUNNING == 1 )) && return
  ROLLBACK_RUNNING=1
  set +e
  local failed=0 files_restored=1 metadata_restored=1 daemon_reloaded=1 nginx_restored=1 timer_must_stop=0
  echo "Installation failed; restoring the previous deployer and Nginx state..." >&2
  if (( UPGRADE_TIMER_EXISTED == 1 || UPGRADE_TIMER_WAS_ENABLED == 1 || UPGRADE_TIMER_WAS_ACTIVE == 1 )) || \
    systemctl is-active --quiet sub2api-deployer-upgrade.timer >/dev/null 2>&1; then
    timer_must_stop=1
  fi
  if ! systemctl stop sub2api-deployer-upgrade.timer >/dev/null 2>&1; then
    if (( timer_must_stop == 1 )); then
      echo "CRITICAL: failed to stop the control-plane upgrade timer before restoration" >&2
      ROLLBACK_INCOMPLETE=1
      return 0
    fi
  fi
  if ! prove_deployer_stopped_for_restore; then
    ROLLBACK_INCOMPLETE=1
    return 0
  fi
  if ! restore_backups; then
    files_restored=0
    failed=1
  fi
  if ! restore_directory_metadata; then
    metadata_restored=0
    failed=1
  fi
  rm -f -- "$RUNTIME_PRESERVE_DROPIN"
  rmdir -- "$(dirname -- "$RUNTIME_PRESERVE_DROPIN")" 2>/dev/null || true
  if ! systemctl daemon-reload >/dev/null 2>&1; then
    echo "CRITICAL: systemd daemon-reload failed after restoration" >&2
    daemon_reloaded=0
    failed=1
  fi
  if nginx -t >/dev/null 2>&1; then
    if ! systemctl reload nginx >/dev/null 2>&1; then
      echo "CRITICAL: failed to reload restored Nginx configuration" >&2
      nginx_restored=0
      failed=1
    fi
  else
    echo "CRITICAL: restored Nginx configuration did not validate; Nginx was not reloaded" >&2
    nginx_restored=0
    failed=1
  fi
  if (( files_restored == 1 && metadata_restored == 1 && daemon_reloaded == 1 && nginx_restored == 1 )); then
    if (( SERVICE_WAS_ENABLED == 1 )); then
      systemctl enable sub2api-deployer.service >/dev/null 2>&1 || failed=1
    else
      systemctl disable sub2api-deployer.service >/dev/null 2>&1 || failed=1
    fi
    if (( UPGRADE_TIMER_EXISTED == 1 )); then
      if (( UPGRADE_TIMER_WAS_ENABLED == 1 )); then
        systemctl enable sub2api-deployer-upgrade.timer >/dev/null 2>&1 || failed=1
      else
        systemctl disable sub2api-deployer-upgrade.timer >/dev/null 2>&1 || failed=1
      fi
    fi
    release_state_lock
    if (( SERVICE_WAS_ACTIVE == 1 )); then
      if ! systemctl start sub2api-deployer.service >/dev/null 2>&1; then
        echo "CRITICAL: failed to restart the restored deployer" >&2
        failed=1
      fi
    else
      systemctl stop sub2api-deployer.service >/dev/null 2>&1 || failed=1
    fi
    if (( UPGRADE_TIMER_EXISTED == 1 )); then
      if (( UPGRADE_TIMER_WAS_ACTIVE == 1 )); then
        systemctl start sub2api-deployer-upgrade.timer >/dev/null 2>&1 || failed=1
      else
        systemctl stop sub2api-deployer-upgrade.timer >/dev/null 2>&1 || failed=1
      fi
    fi
  else
    echo "CRITICAL: deployer remains stopped because its prior installation was not fully restored" >&2
  fi
  if (( failed == 1 )); then
    ROLLBACK_INCOMPLETE=1
    echo "CRITICAL: automatic rollback was incomplete; inspect Nginx and sub2api-deployer before retrying" >&2
  fi
  set -e
}

on_exit() {
  local status=$?
  trap - EXIT
  if (( status != 0 && MUTATION_STARTED == 1 && COMMITTED == 0 )); then
    rollback_install
  elif (( status != 0 && ACTIVATOR_FROZEN == 1 && COMMITTED == 0 )); then
    restore_frozen_activator_state
  fi
  if (( status != 0 && SOCKET_GROUP_CREATED == 1 && COMMITTED == 0 )); then
    groupdel "$SOCKET_GROUP_NAME" >/dev/null 2>&1 || true
  fi
  if (( ROLLBACK_INCOMPLETE == 1 )); then
    echo "Recovery backups were preserved at $TEMP_DIR" >&2
  elif [[ -n "$TEMP_DIR" && -d "$TEMP_DIR" ]]; then
    rm -rf -- "$TEMP_DIR"
  fi
  release_activation_lock
  exit "$status"
}
trap on_exit EXIT

count_effective_directive() {
  local file="$1"
  local expected="$2"
  awk -v expected="$expected" '
    {
      line = $0
      sub(/#.*/, "", line)
      gsub(/[[:space:]]+/, " ", line)
      sub(/^ /, "", line)
      sub(/ $/, "", line)
      if (line == expected) count++
    }
    END { print count + 0 }
  ' "$file"
}

probe_nginx() {
  local -a args=(--fail --silent --show-error --max-time 10)
  local body
  if [[ -n "$NGINX_PROBE_HOST" ]]; then
    args+=(--header "Host: $NGINX_PROBE_HOST")
  fi
  body=$(curl "${args[@]}" "$NGINX_PROBE_URL")
  jq -e '.status == "ok"' <<<"$body" >/dev/null
}

validate_idle_state_file() {
  local state_file="$1"
  [[ -f "$state_file" ]] || fail "Existing deployer state is missing: $state_file"
  jq -e '
    type == "object"
    and (.active_slot | type == "string" and length > 0)
    and (.active_container | type == "string" and length > 0)
    and (.active_port | type == "number" and . >= 1 and . <= 65535)
    and .degraded == false
    and (
      (.job // null) == null
      or (
        (.job | type == "object")
        and (.job.status | type == "string")
        and .job.status != "running"
        and .job.status != "rollback_failed"
        and .job.status != "degraded"
      )
    )
  ' "$state_file" >/dev/null || \
    fail "Existing deployer state is invalid, degraded, or has a deployment requiring recovery: $state_file"
}

load_compose_identity() {
  local container="$1"
  local expected_service="$2"
  local labels actual_service
  INSPECTED_CONTAINER_ID=$(docker inspect "$container" --format '{{.Id}}') || \
    fail "Could not inspect Compose ownership for container $container"
  [[ "$INSPECTED_CONTAINER_ID" =~ ^[0-9a-f]{64}$ ]] || \
    fail "Container $container returned an invalid immutable Docker ID"
  labels=$(docker inspect "$INSPECTED_CONTAINER_ID" --format '{{json .Config.Labels}}') || \
    fail "Container $container disappeared while its ownership was being verified"
  COMPOSE_PROJECT=$(jq -er '.["com.docker.compose.project"] | strings | select(length > 0)' <<<"$labels") || \
    fail "Container $container has no Docker Compose project label"
  actual_service=$(jq -er '.["com.docker.compose.service"] | strings | select(length > 0)' <<<"$labels") || \
    fail "Container $container has no Docker Compose service label"
  [[ "$actual_service" == "$expected_service" ]] || \
    fail "Container $container belongs to Compose service $actual_service, expected $expected_service"
  assert_container_name_id "$container" "$INSPECTED_CONTAINER_ID"
}

validate_existing_compose_contract() {
  local config_file="$1"
  local compose_file managed_file env_file image_env_file
  jq -e \
    --arg repository "$IMAGE_REPOSITORY" \
    --arg source "$IMAGE_SOURCE_LABEL" \
    --arg protocol "$UPDATE_PROTOCOL_LABEL" '
      .image_repository == $repository
      and .required_image_labels == {
        "org.opencontainers.image.source": $source,
        "io.tokensupply.sub2api.update-protocol": $protocol
      }
      and .compose_service == "sub2api"
      and .image_environment == "SUB2API_IMAGE"
      and .container_port == 8080
      and (.compose_files | type == "array" and length == 2 and all(.[]; type == "string" and length > 0 and (contains("\n") | not) and (contains("\r") | not)))
      and (.compose_env_files | type == "array" and length == 2 and all(.[]; type == "string" and length > 0 and (contains("\n") | not) and (contains("\r") | not)))
    ' "$config_file" >/dev/null || \
    fail "Existing deployer Compose/image trust configuration is not the supported managed-update contract"

  compose_file=$(jq -er '.compose_files[0]' "$config_file")
  managed_file=$(jq -er '.compose_files[1]' "$config_file")
  env_file=$(jq -er '.compose_env_files[0]' "$config_file")
  image_env_file=$(jq -er '.compose_env_files[1]' "$config_file")
  [[ $(normalize_compose_path "$INSTALL_DIR" "$compose_file") == "$INSTALL_DIR/compose.yaml" ]] || \
    fail "Existing compose_files must use $INSTALL_DIR/compose.yaml as the base file"
  [[ $(normalize_compose_path "$INSTALL_DIR" "$managed_file") == "$INSTALL_DIR/compose.deployer.yml" ]] || \
    fail "Existing compose_files must use $INSTALL_DIR/compose.deployer.yml as the managed override"
  [[ $(normalize_compose_path "$INSTALL_DIR" "$env_file") == "$INSTALL_DIR/.env" ]] || \
    fail "Existing compose_env_files must use $INSTALL_DIR/.env first"
  [[ $(normalize_compose_path "$INSTALL_DIR" "$image_env_file") == "$(readlink -f -- "$OLD_IMAGE_STATE_FILE")" ]] || \
    fail "Existing compose_env_files must use its configured image state file last"
}

compose_preflight() {
  local config_file="$1"
  local staged_override="${2:-}"
  local docker_binary work_dir project service image_environment
  local rendered_config="$TEMP_DIR/compose-preflight.json"
  local -a env_files=() compose_files=() args=()
  local value

  docker_binary=$(jq -er '.docker_binary | strings | select(length > 0)' "$config_file")
  work_dir=$(jq -er '.compose_work_dir | strings | select(length > 0)' "$config_file")
  project=$(jq -er '.compose_project | strings | select(length > 0)' "$config_file")
  service=$(jq -er '.compose_service | strings | select(length > 0)' "$config_file")
  image_environment=$(jq -er '.image_environment | strings | select(test("^[A-Za-z_][A-Za-z0-9_]*$"))' "$config_file")
  [[ "$docker_binary" == "$(command -v docker)" ]] || fail "Final deployer config does not use the discovered Docker binary"
  [[ "$work_dir" == "$INSTALL_DIR" ]] || fail "Final deployer config has an unexpected Compose working directory"
  [[ "$service" == "sub2api" ]] || fail "Final deployer config has an unexpected Compose service"

  while IFS= read -r value; do env_files+=("$value"); done < <(jq -er '.compose_env_files[]' "$config_file")
  while IFS= read -r value; do compose_files+=("$value"); done < <(jq -er '.compose_files[]' "$config_file")
  [[ ${#env_files[@]} -eq 2 && "${env_files[0]}" == "$INSTALL_DIR/.env" && "${env_files[1]}" == "$IMAGE_STATE_FILE" ]] || \
    fail "Final deployer config has unexpected Compose env files"
  [[ ${#compose_files[@]} -eq 2 && "${compose_files[0]}" == "$INSTALL_DIR/compose.yaml" && "${compose_files[1]}" == "$INSTALL_DIR/compose.deployer.yml" ]] || \
    fail "Final deployer config has unexpected Compose files"
  if [[ -n "$staged_override" ]]; then
    env_files[1]="$TEMP_DIR/image.env"
    compose_files[1]="$staged_override"
  fi

  args=(compose --project-name "$project" --project-directory "$work_dir")
  for value in "${env_files[@]}"; do args+=(--env-file "$value"); done
  for value in "${compose_files[@]}"; do args+=(-f "$value"); done
  env "$image_environment=$CURRENT_IMAGE" "SUB2API_DEPLOYER_SOCKET_GID=$SOCKET_GID" \
    "$docker_binary" "${args[@]}" config --quiet
  env "$image_environment=$CURRENT_IMAGE" "SUB2API_DEPLOYER_SOCKET_GID=$SOCKET_GID" \
    "$docker_binary" "${args[@]}" config --format json > "$rendered_config"
  jq -e \
    --arg service "$service" \
    --arg image "$CURRENT_IMAGE" \
    --arg gid "$SOCKET_GID" '
      .services[$service] as $app
      | ($app | type == "object")
        and $app.image == $image
        and (($app.group_add // []) | map(tostring) | index($gid) != null)
        and (($app.volumes // []) | any(.[];
          .type == "bind"
          and .source == "/run/sub2api-deployer"
          and .target == "/run/sub2api-deployer"
          and .read_only == true
        ))
        and (($app.volumes // []) | any(.[];
          .type == "bind"
          and .source == "/var/lib/sub2api-deployer/runtime"
          and .target == "/run/sub2api-deployment"
          and .read_only == true
        ))
        and $app.environment.UPDATE_MODE == "docker-managed"
        and $app.environment.UPDATE_DEPLOYER_SOCKET == "/run/sub2api-deployer/deployer.sock"
        and $app.environment.SERVER_SHUTDOWN_TIMEOUT == "90"
        and (($app.healthcheck.test // []) | length > 0)
    ' "$rendered_config" >/dev/null || \
    fail "Effective Compose service does not contain the required managed-update override"
}

if [[ $(id -u) -ne 0 ]]; then
  fail "This installer must run as root"
fi
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
if [[ -z "$ASSET_DIR" && -d "$SCRIPT_DIR/deploy" ]]; then
  ASSET_DIR="$SCRIPT_DIR/deploy"
fi
if [[ -z "$ASSET_DIR" && -n "$SOURCE_DIR" ]]; then
  ASSET_DIR="$SOURCE_DIR/deploy"
fi
if [[ -z "$ASSET_DIR" || ! -d "$ASSET_DIR" ]]; then
  fail "--assets-dir must point to the packaged deploy assets (or use an installer beside deploy/)"
fi
for source_asset in compose.deployer.yml sub2api-deployer.service sub2api-deployer-upgrade.service sub2api-deployer-upgrade.timer sub2api-deployer-tmpfiles.conf sub2api-managed-upstream.conf; do
  [[ -f "$ASSET_DIR/$source_asset" ]] || fail "Deploy assets are missing $source_asset"
done
if [[ -z "$NGINX_SITE" || ! -f "$NGINX_SITE" ]]; then
  fail "--nginx-site must point to the active Nginx site file"
fi
case "$INSTALL_DIR" in
  /root|/root/*|/home|/home/*|/run/user|/run/user/*)
    fail "--install-dir must not be under /root, /home, or /run/user because the hardened deployer service cannot read those paths"
    ;;
esac
if [[ ! -f "$INSTALL_DIR/compose.yaml" || ! -f "$INSTALL_DIR/.env" ]]; then
  fail "$INSTALL_DIR must contain compose.yaml and .env"
fi
if ! [[ "$NGINX_PROBE_URL" =~ ^http://127\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}(:[0-9]+)?/health$ ]] && \
  ! [[ "$NGINX_PROBE_URL" =~ ^http://\[::1\](:[0-9]+)?/health$ ]]; then
  fail "--nginx-probe-url must use a literal loopback IP over http with the exact /health path"
fi
if [[ -n "$NGINX_PROBE_HOST" ]] && ! [[ "$NGINX_PROBE_HOST" =~ ^[A-Za-z0-9.-]+(:[0-9]+)?$ ]]; then
  fail "--nginx-probe-host must be a hostname with an optional numeric port"
fi
NGINX_SITE=$(readlink -f "$NGINX_SITE")
ASSET_DIR=$(readlink -f "$ASSET_DIR")
if [[ -n "$SOURCE_DIR" ]]; then
  SOURCE_DIR=$(readlink -f "$SOURCE_DIR")
fi
INSTALL_DIR=$(readlink -f "$INSTALL_DIR")
case "$INSTALL_DIR" in
  /root|/root/*|/home|/home/*|/run/user|/run/user/*)
    fail "--install-dir must not be under /root, /home, or /run/user because the hardened deployer service cannot read those paths"
    ;;
esac
if (( SOCKET_GID_SET == 1 )) && { ! [[ "$SOCKET_GID" =~ ^[0-9]+$ ]] || (( 10#$SOCKET_GID > 2147483647 )); }; then
  fail "--socket-gid must be a valid numeric GID"
fi
if [[ -n "$ACTIVATION_VERSION" ]] && ! [[ "$ACTIVATION_VERSION" =~ ^[0-9][0-9A-Za-z.-]{0,63}$ ]]; then
  fail "--activation-version is invalid"
fi
if [[ -n "$DEPLOYER_BINARY" && -z "$DEPLOYER_CHECKSUMS" ]] || [[ -z "$DEPLOYER_BINARY" && -n "$DEPLOYER_CHECKSUMS" ]]; then
  fail "--deployer-binary and --deployer-checksums must be supplied together"
fi
if [[ -n "$DOCKER_CONFIG_SOURCE" ]]; then
  [[ -f "$DOCKER_CONFIG_SOURCE" ]] || fail "Docker config not found: $DOCKER_CONFIG_SOURCE"
  DOCKER_CONFIG_SOURCE=$(readlink -f "$DOCKER_CONFIG_SOURCE")
  jq -e 'type == "object"' "$DOCKER_CONFIG_SOURCE" >/dev/null || fail "--docker-config must contain a valid Docker config JSON object"
fi
for value in "$SOURCE_DIR" "$ASSET_DIR" "$NGINX_SITE" "$NGINX_PROBE_URL" "$NGINX_PROBE_HOST" "$INSTALL_DIR" "$CONTAINER_NAME" "$DOCKER_CONFIG_SOURCE"; do
  reject_json_unsafe "path, URL, host, or container name" "$value"
done
validate_container_name "--container" "$CONTAINER_NAME"

for command in docker nginx systemctl ss sed grep readlink install mktemp sha256sum curl awk jq cp mv chmod chown stat rmdir perl flock env; do
  command -v "$command" >/dev/null || fail "Missing command: $command"
done
validate_docker_server_version
if [[ -z "$DEPLOYER_BINARY" ]]; then
  (( ALLOW_SOURCE_BUILD == 1 )) || fail "Source builds are disabled by default; use a verified release binary or pass --allow-source-build for a dev-identity fallback"
  [[ -n "$SOURCE_DIR" && -f "$SOURCE_DIR/backend/go.mod" ]] || fail "--source must point to a Sub2API repository checkout when building the deployer"
  command -v go >/dev/null || fail "Missing command: go (or provide a release deployer binary)"
else
  [[ -f "$DEPLOYER_BINARY" ]] || fail "Deployer binary not found: $DEPLOYER_BINARY"
  [[ -f "$DEPLOYER_CHECKSUMS" ]] || fail "Checksum file not found: $DEPLOYER_CHECKSUMS"
fi

# Serialize installers before any persisted deployer configuration or state is
# examined. The lock lives in a root-owned runtime directory and is root-only.
acquire_root_lock "$INSTALL_LOCK_FILE" INSTALL_LOCK_FD

if systemctl is-enabled --quiet sub2api-deployer.service >/dev/null 2>&1; then
  SERVICE_WAS_ENABLED=1
fi
if systemctl is-active --quiet sub2api-deployer.service >/dev/null 2>&1; then
  SERVICE_WAS_ACTIVE=1
fi
if systemctl is-enabled --quiet sub2api-deployer-upgrade.timer >/dev/null 2>&1; then
  UPGRADE_TIMER_WAS_ENABLED=1
fi
if systemctl is-active --quiet sub2api-deployer-upgrade.timer >/dev/null 2>&1; then
  UPGRADE_TIMER_WAS_ACTIVE=1
fi
if systemctl is-active --quiet sub2api-deployer-upgrade.service >/dev/null 2>&1; then
  UPGRADE_SERVICE_WAS_ACTIVE=1
fi
if [[ -e "$UPGRADE_TIMER_FILE" ]]; then
  UPGRADE_TIMER_EXISTED=1
fi

TEMP_DIR=$(mktemp -d /tmp/sub2api-deployer-install.XXXXXX)

if [[ -n "$DOCKER_CONFIG_SOURCE" ]]; then
  install -m 0600 "$DOCKER_CONFIG_SOURCE" "$TEMP_DIR/docker-config.json"
fi

if [[ -n "$DEPLOYER_BINARY" ]]; then
  BINARY_NAME=$(basename "$DEPLOYER_BINARY")
  EXPECTED_SHA=$(awk -v name="$BINARY_NAME" '$2 == name || $2 == "*" name {print $1; exit}' "$DEPLOYER_CHECKSUMS")
  [[ "$EXPECTED_SHA" =~ ^[0-9a-fA-F]{64}$ ]] || fail "No checksum found for $BINARY_NAME"
  ACTUAL_SHA=$(sha256sum "$DEPLOYER_BINARY" | awk '{print $1}')
  [[ "$ACTUAL_SHA" == "$EXPECTED_SHA" ]] || fail "Deployer binary checksum mismatch"
  install -m 0755 "$DEPLOYER_BINARY" "$TEMP_DIR/sub2api-deployer"
else
  echo "Building deployer from source..."
  (cd "$SOURCE_DIR/backend" && go build -trimpath -ldflags='-X main.BuildType=dev' -o "$TEMP_DIR/sub2api-deployer" ./cmd/deployer)
fi

EXISTING_INSTALL=0
OLD_IMAGE_STATE_FILE=""
OLD_UPSTREAM_FILE=""
OLD_STATE_FILE=""
OLD_RUNTIME_MARKER=""
OLD_SOCKET_PATH=""
CURRENT_IMAGE=""
CURRENT_VERSION=""
CURRENT_PORT=""
GREEN_PORT=""
COMPOSE_PROJECT=""
CONFIGURED_SERVICE="sub2api"
ACTIVE_CONTAINER=""
ACTIVE_PORT=""

if [[ -f "$CONFIG_FILE" ]]; then
  EXISTING_INSTALL=1
  EXISTING_WORK_DIR=$(jq -er '.compose_work_dir | strings | select(length > 0)' "$CONFIG_FILE")
  EXISTING_SITE=$(jq -er '.nginx_site_path | strings | select(length > 0)' "$CONFIG_FILE")
  EXISTING_WORK_DIR=$(readlink -f "$EXISTING_WORK_DIR")
  EXISTING_SITE=$(readlink -f "$EXISTING_SITE")
  [[ "$EXISTING_WORK_DIR" == "$INSTALL_DIR" ]] || fail "Existing deployer uses compose directory $EXISTING_WORK_DIR; rerun with that --install-dir"
  [[ "$EXISTING_SITE" == "$NGINX_SITE" ]] || fail "Existing deployer uses Nginx site $EXISTING_SITE; rerun with that --nginx-site"
  OLD_IMAGE_STATE_FILE=$(jq -er '.image_state_path | strings | select(length > 0)' "$CONFIG_FILE")
  OLD_UPSTREAM_FILE=$(jq -er '.nginx_upstream_path | strings | select(length > 0)' "$CONFIG_FILE")
  OLD_STATE_FILE=$(jq -er '.state_path | strings | select(length > 0)' "$CONFIG_FILE")
  EXISTING_CONTROL_PLANE_UPGRADE_REQUEST=$(jq -er '.control_plane_upgrade_path | strings | select(length > 0)' "$CONFIG_FILE")
  OLD_RUNTIME_MARKER=$(jq -er '.deployment_state_path | strings | select(length > 0)' "$CONFIG_FILE")
  OLD_SOCKET_PATH=$(jq -er '.socket_path | strings | select(length > 0)' "$CONFIG_FILE")
  CONFIGURED_SERVICE=$(jq -er '.compose_service | strings | select(length > 0)' "$CONFIG_FILE")
  CONFIGURED_PROJECT=$(jq -er '.compose_project | strings | select(length > 0)' "$CONFIG_FILE")
  for value in "$OLD_IMAGE_STATE_FILE" "$OLD_UPSTREAM_FILE" "$OLD_STATE_FILE" "$OLD_RUNTIME_MARKER" "$OLD_SOCKET_PATH" "$EXISTING_CONTROL_PLANE_UPGRADE_REQUEST" "$CONFIGURED_PROJECT" "$CONFIGURED_SERVICE"; do
    reject_json_unsafe "Existing deployer path or Compose identity" "$value"
  done
  for value in "$OLD_IMAGE_STATE_FILE" "$OLD_UPSTREAM_FILE" "$OLD_STATE_FILE" "$OLD_RUNTIME_MARKER" "$OLD_SOCKET_PATH" "$EXISTING_CONTROL_PLANE_UPGRADE_REQUEST"; do
    [[ "$value" == /* ]] || fail "Existing deployer paths must be absolute: $value"
  done
  [[ $(dirname -- "$EXISTING_CONTROL_PLANE_UPGRADE_REQUEST") == "$(dirname -- "$OLD_STATE_FILE")" ]] || \
    fail "Existing control_plane_upgrade_path must use the same directory as state_path"
  validate_container_name "Existing compose_service" "$CONFIGURED_SERVICE"
  [[ "$CONFIGURED_PROJECT" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || fail "Existing compose_project is invalid"
  [[ "$OLD_RUNTIME_MARKER" == "$RUNTIME_MARKER" ]] || \
    fail "Existing deployment marker path $OLD_RUNTIME_MARKER cannot be changed while retained containers may still mount it"
  [[ "$OLD_SOCKET_PATH" == "/run/sub2api-deployer/deployer.sock" ]] || \
    fail "Existing deployer socket path $OLD_SOCKET_PATH cannot be changed while application containers are running"
  validate_existing_compose_contract "$CONFIG_FILE"
  if (( SOCKET_GID_SET == 0 )); then
    SOCKET_GID=$(jq -er '.socket_gid | numbers' "$CONFIG_FILE")
  fi

  # Freeze the activator before the first host mutation. Holding the same lock
  # as the Go activator prevents the timer from observing a partially updated
  # binary or unit set.
  systemctl stop sub2api-deployer-upgrade.timer
  systemctl stop sub2api-deployer-upgrade.service
  EXISTING_ACTIVATION_LOCK_FILE="$(dirname -- "$EXISTING_CONTROL_PLANE_UPGRADE_REQUEST")/control-plane-activation.lock"
  acquire_root_lock "$EXISTING_ACTIVATION_LOCK_FILE" ACTIVATION_LOCK_FD
  if [[ "$EXISTING_ACTIVATION_LOCK_FILE" != "$STATE_DIR/control-plane-activation.lock" ]]; then
    acquire_root_lock "$STATE_DIR/control-plane-activation.lock" TARGET_ACTIVATION_LOCK_FD
  fi
  ACTIVATOR_FROZEN=1
  [[ ! -e "$EXISTING_CONTROL_PLANE_UPGRADE_REQUEST" && ! -L "$EXISTING_CONTROL_PLANE_UPGRADE_REQUEST" ]] || \
    fail "Refusing host migration while a control-plane activation request is pending"

  if (( SERVICE_WAS_ACTIVE == 1 )); then
    EXISTING_HEALTH=$(curl --fail --silent --show-error --max-time 10 --unix-socket "$OLD_SOCKET_PATH" http://localhost/v1/health) || \
      fail "Existing deployer is active but its configured health endpoint is unavailable"
    jq -e '.status == "ok" and .degraded == false and .job_running == false' <<<"$EXISTING_HEALTH" >/dev/null || \
      fail "Existing deployer is not healthy and idle"

    SOCKET_DIRECTORY_INODE=$(stat -c '%i' "$SOCKET_DIRECTORY") || \
      fail "Could not capture the existing deployer socket directory inode"
    [[ "$SOCKET_DIRECTORY_INODE" =~ ^[0-9]+$ ]] || fail "Existing deployer socket directory returned an invalid inode"

    # Load a volatile compatibility drop-in before stopping an older unit. The
    # ts.4 unit otherwise deletes RuntimeDirectory and strands the live app's
    # bind mount on an unlinked inode before the ts.6 unit can be installed.
    MUTATION_STARTED=1
    install -d -m 0755 "$(dirname -- "$RUNTIME_PRESERVE_DROPIN")"
    cat > "$RUNTIME_PRESERVE_DROPIN" <<'EOF'
[Service]
RuntimeDirectoryPreserve=yes
EOF
    chmod 0644 "$RUNTIME_PRESERVE_DROPIN"
    systemctl daemon-reload

    # Quiesce the update control plane before taking the migration snapshot.
    # The application and Nginx keep serving normally while it is stopped.
    systemctl stop sub2api-deployer.service
    [[ $(stat -c '%i' "$SOCKET_DIRECTORY") == "$SOCKET_DIRECTORY_INODE" ]] || \
      fail "Stopping the existing deployer replaced its socket directory inode"
  elif curl --fail --silent --max-time 2 --unix-socket "$OLD_SOCKET_PATH" http://localhost/v1/health >/dev/null 2>&1; then
    fail "A deployer is still accepting requests on $OLD_SOCKET_PATH outside the systemd service"
  fi

  # The deployer uses this same lock for the lifetime of its process. Once the
  # service is stopped, holding it makes every subsequent state read a stable,
  # root-only snapshot and prevents a second daemon from starting mid-upgrade.
  acquire_root_lock "$(dirname -- "$OLD_STATE_FILE")/deployer.lock" STATE_LOCK_FD
  [[ -f "$OLD_IMAGE_STATE_FILE" ]] || fail "Existing deployer image state is missing: $OLD_IMAGE_STATE_FILE"
  [[ -f "$OLD_UPSTREAM_FILE" ]] || fail "Existing managed Nginx upstream is missing: $OLD_UPSTREAM_FILE"
  validate_idle_state_file "$OLD_STATE_FILE"

  CURRENT_IMAGE=$(sed -n 's/^SUB2API_IMAGE=//p' "$OLD_IMAGE_STATE_FILE" | tail -1)
  [[ -n "$CURRENT_IMAGE" ]] || fail "Existing deployer image state has no SUB2API_IMAGE value"
  ACTIVE_CONTAINER=$(jq -er '.active_container | strings | select(length > 0)' "$OLD_STATE_FILE")
  ACTIVE_SLOT=$(jq -er '.active_slot | strings | select(length > 0)' "$OLD_STATE_FILE")
  ACTIVE_PORT=$(jq -er '.active_port | numbers' "$OLD_STATE_FILE")
  STATE_ACTIVE_CONTAINER_ID=$(jq -r '.active_container_id // "" | strings' "$OLD_STATE_FILE")
  CURRENT_VERSION=$(jq -er '.active_version | strings | select(length > 0)' "$OLD_STATE_FILE")
  STATE_ACTIVE_IMAGE=$(jq -r '.active_image // "" | strings' "$OLD_STATE_FILE")
  [[ "$CURRENT_VERSION" =~ ^[0-9][0-9A-Za-z.-]{0,63}$ ]] || \
    fail "Existing deployer state has an invalid active version"
  if [[ -n "$STATE_ACTIVE_CONTAINER_ID" ]] && ! [[ "$STATE_ACTIVE_CONTAINER_ID" =~ ^[0-9a-f]{64}$ ]]; then
    fail "Existing deployer state has an invalid active_container_id"
  fi
  INITIAL_CONTAINER=$(jq -er '.initial_container | strings | select(length > 0)' "$CONFIG_FILE")
  validate_container_name "Existing active_container" "$ACTIVE_CONTAINER"
  validate_container_name "Existing initial_container" "$INITIAL_CONTAINER"
  if [[ -f "$OLD_RUNTIME_MARKER" ]]; then
    [[ "$(<"$OLD_RUNTIME_MARKER")" == "$ACTIVE_SLOT" ]] || \
      fail "Existing deployment marker does not match active slot $ACTIVE_SLOT"
  elif [[ "$ACTIVE_CONTAINER" != "$INITIAL_CONTAINER" ]]; then
    fail "Existing managed active container has no deployment marker: $OLD_RUNTIME_MARKER"
  fi

  load_compose_identity "$ACTIVE_CONTAINER" "$CONFIGURED_SERVICE"
  [[ "$COMPOSE_PROJECT" == "$CONFIGURED_PROJECT" ]] || \
    fail "Running container belongs to Compose project $COMPOSE_PROJECT, but deployer config uses $CONFIGURED_PROJECT"
  if [[ -n "$STATE_ACTIVE_CONTAINER_ID" && "$STATE_ACTIVE_CONTAINER_ID" != "$INSPECTED_CONTAINER_ID" ]]; then
    fail "Container name $ACTIVE_CONTAINER no longer resolves to the immutable ID stored in deployer state"
  fi
  load_single_loopback_port "$INSPECTED_CONTAINER_ID"
  (( 10#$INSPECTED_PORT == 10#$ACTIVE_PORT )) || \
    fail "Running container port $INSPECTED_PORT does not match persisted active_port $ACTIVE_PORT"
  load_container_runtime "$INSPECTED_CONTAINER_ID"
  [[ "$INSPECTED_IMAGE" == "$CURRENT_IMAGE" ]] || \
    fail "Running container Config.Image does not match the persisted image state"
  [[ "$INSPECTED_VERSION" == "$CURRENT_VERSION" ]] || \
    fail "Running container version $INSPECTED_VERSION does not match persisted active_version $CURRENT_VERSION"
  if [[ -n "$STATE_ACTIVE_IMAGE" && "$STATE_ACTIVE_IMAGE" != "$INSPECTED_IMAGE" ]]; then
    fail "Running container Config.Image does not match persisted active_image"
  fi
  assert_container_name_id "$ACTIVE_CONTAINER" "$INSPECTED_CONTAINER_ID"

  if (( SERVICE_WAS_ACTIVE == 1 )); then
    jq -e \
      --arg container "$ACTIVE_CONTAINER" \
      --arg container_id "$INSPECTED_CONTAINER_ID" \
      --arg version "$CURRENT_VERSION" \
      --argjson port "$ACTIVE_PORT" \
      '.active_container == $container and .active_port == $port and .active_version == $version' \
      <<<"$EXISTING_HEALTH" >/dev/null || \
      fail "Existing deployer health did not match the locked persisted state"
  fi

  cp -a -- "$OLD_STATE_FILE" "$TEMP_DIR/state.json"
  if [[ -f "$OLD_RUNTIME_MARKER" ]]; then
    cp -a -- "$OLD_RUNTIME_MARKER" "$TEMP_DIR/active-slot"
  fi

  jq \
    --arg socket_path "/run/sub2api-deployer/deployer.sock" \
    --arg state_path "$STATE_FILE" \
    --arg image_state "$IMAGE_STATE_FILE" \
    --arg deployment_state_path "$RUNTIME_MARKER" \
    --arg upstream "$MANAGED_UPSTREAM_FILE" \
    --arg probe_url "$NGINX_PROBE_URL" \
    --arg probe_host "$NGINX_PROBE_HOST" \
    --arg compose_project "$COMPOSE_PROJECT" \
    --arg compose_work_dir "$INSTALL_DIR" \
    --arg compose_env "$INSTALL_DIR/.env" \
    --arg compose_file "$INSTALL_DIR/compose.yaml" \
    --arg compose_override "$INSTALL_DIR/compose.deployer.yml" \
    --arg image_repository "$IMAGE_REPOSITORY" \
    --arg image_source "$IMAGE_SOURCE_LABEL" \
    --arg update_protocol "$UPDATE_PROTOCOL_LABEL" \
    --arg docker_binary "$(command -v docker)" \
    --argjson socket_gid "$SOCKET_GID" \
    --arg nginx_binary "$(command -v nginx)" \
    --arg control_plane_upgrade_path "$CONTROL_PLANE_UPGRADE_REQUEST" \
    --arg systemctl_binary "$(command -v systemctl)" '
      .socket_path = $socket_path
      | .state_path = $state_path
      | .image_state_path = $image_state
      | .deployment_state_path = $deployment_state_path
      | .image_repository = $image_repository
      | .required_image_labels = {
          "org.opencontainers.image.source": $image_source,
          "io.tokensupply.sub2api.update-protocol": $update_protocol
        }
      | .docker_binary = $docker_binary
      | .compose_work_dir = $compose_work_dir
      | .compose_project = $compose_project
      | .compose_env_files = [$compose_env, $image_state]
      | .compose_files = [$compose_file, $compose_override]
      | .compose_service = "sub2api"
      | .image_environment = "SUB2API_IMAGE"
      | .container_port = 8080
      | .nginx_upstream_path = $upstream
      | .nginx_probe_url = $probe_url
      | .nginx_dump_command = [$nginx_binary, "-T"]
      | .control_plane_upgrade_path = $control_plane_upgrade_path
      | .control_plane_upgrade_command = [$systemctl_binary, "start", "--no-block", "sub2api-deployer-upgrade.service"]
      | .route_confirmation_timeout = (.route_confirmation_timeout // "10s")
      | if .health_timeout == "2m" then .health_timeout = "12m" else . end
      | .socket_gid = $socket_gid
      | if $probe_host == "" then del(.nginx_probe_host) else .nginx_probe_host = $probe_host end
    ' "$CONFIG_FILE" > "$TEMP_DIR/config.json"
  cp -a "$OLD_IMAGE_STATE_FILE" "$TEMP_DIR/image.env"
  cp -a "$OLD_UPSTREAM_FILE" "$TEMP_DIR/managed-upstream.conf"
else
  if (( SOCKET_GID_SET == 0 )); then
    for command in getent groupadd groupdel cut; do
      command -v "$command" >/dev/null || fail "Missing command: $command (or provide --socket-gid)"
    done
    if ! getent group "$SOCKET_GROUP_NAME" >/dev/null; then
      groupadd --system "$SOCKET_GROUP_NAME"
      SOCKET_GROUP_CREATED=1
    fi
    SOCKET_GID=$(getent group "$SOCKET_GROUP_NAME" | cut -d: -f3)
    [[ "$SOCKET_GID" =~ ^[0-9]+$ ]] || fail "Could not resolve the dedicated $SOCKET_GROUP_NAME group GID"
  fi
  load_compose_identity "$CONTAINER_NAME" "$CONFIGURED_SERVICE"
  load_single_loopback_port "$INSPECTED_CONTAINER_ID"
  CURRENT_PORT="$INSPECTED_PORT"
  (( 10#$CURRENT_PORT <= 65534 )) || fail "Current application port leaves no valid adjacent candidate port"
  ACTIVE_CONTAINER="$CONTAINER_NAME"
  ACTIVE_PORT="$CURRENT_PORT"
  GREEN_PORT=$((CURRENT_PORT + 1))
  if ss -Hln "sport = :$GREEN_PORT" | grep -q .; then
    fail "Candidate port $GREEN_PORT is already in use"
  fi
  load_container_runtime "$INSPECTED_CONTAINER_ID"
  CURRENT_IMAGE="$INSPECTED_IMAGE"
  CURRENT_VERSION="$INSPECTED_VERSION"
  assert_container_name_id "$CONTAINER_NAME" "$INSPECTED_CONTAINER_ID"

  printf 'SUB2API_IMAGE=%s\n' "$CURRENT_IMAGE" > "$TEMP_DIR/image.env"
  cat > "$TEMP_DIR/managed-upstream.conf" <<EOF
# Managed by sub2api-deployer. Manual edits are overwritten.
upstream sub2api_managed {
    server 127.0.0.1:$CURRENT_PORT;
    keepalive 64;
}
EOF
  jq -n \
    --argjson socket_gid "$SOCKET_GID" \
    --arg state_path "$STATE_FILE" \
    --arg image_state_path "$IMAGE_STATE_FILE" \
    --arg image_repository "$IMAGE_REPOSITORY" \
    --arg image_source "$IMAGE_SOURCE_LABEL" \
    --arg update_protocol "$UPDATE_PROTOCOL_LABEL" \
    --arg docker_binary "$(command -v docker)" \
    --arg compose_work_dir "$INSTALL_DIR" \
    --arg compose_project "$COMPOSE_PROJECT" \
    --arg deployment_state_path "$RUNTIME_MARKER" \
    --argjson current_port "$CURRENT_PORT" \
    --argjson green_port "$GREEN_PORT" \
    --arg initial_container "$CONTAINER_NAME" \
    --arg initial_version "$CURRENT_VERSION" \
    --arg nginx_upstream_path "$MANAGED_UPSTREAM_FILE" \
    --arg nginx_site_path "$NGINX_SITE" \
    --arg nginx_binary "$(command -v nginx)" \
    --arg systemctl_binary "$(command -v systemctl)" \
    --arg control_plane_upgrade_path "$CONTROL_PLANE_UPGRADE_REQUEST" \
    --arg nginx_probe_url "$NGINX_PROBE_URL" \
    --arg nginx_probe_host "$NGINX_PROBE_HOST" '
      {
        socket_path: "/run/sub2api-deployer/deployer.sock",
        socket_mode: 432,
        socket_gid: $socket_gid,
        state_path: $state_path,
        image_state_path: $image_state_path,
        image_repository: $image_repository,
        required_image_labels: {
          "org.opencontainers.image.source": $image_source,
          "io.tokensupply.sub2api.update-protocol": $update_protocol
        },
        docker_binary: $docker_binary,
        compose_work_dir: $compose_work_dir,
        compose_project: $compose_project,
        compose_env_files: [$compose_work_dir + "/.env", $image_state_path],
        compose_files: [$compose_work_dir + "/compose.yaml", $compose_work_dir + "/compose.deployer.yml"],
        compose_service: "sub2api",
        image_environment: "SUB2API_IMAGE",
        container_port: 8080,
        deployment_state_path: $deployment_state_path,
        deployment_state_file: "/run/sub2api-deployment/active-slot",
        slots: [
          {name: "sub2api-blue", host: "127.0.0.1", port: $current_port},
          {name: "sub2api-green", host: "127.0.0.1", port: $green_port}
        ],
        initial_container: $initial_container,
        initial_version: $initial_version,
        nginx_upstream_path: $nginx_upstream_path,
        nginx_site_path: $nginx_site_path,
        nginx_upstream_name: "sub2api_managed",
        nginx_test_command: [$nginx_binary, "-t"],
        nginx_dump_command: [$nginx_binary, "-T"],
        nginx_reload_command: [$systemctl_binary, "reload", "nginx"],
        control_plane_upgrade_path: $control_plane_upgrade_path,
        control_plane_upgrade_command: [$systemctl_binary, "start", "--no-block", "sub2api-deployer-upgrade.service"],
        nginx_probe_url: $nginx_probe_url,
        nginx_probe_host: $nginx_probe_host,
        health_path: "/health",
        route_confirmation_timeout: "10s",
        health_timeout: "12m",
        stabilize_duration: "20s",
        drain_duration: "15s",
        drain_timeout: "30m",
        stop_timeout: "2m"
      }
    ' > "$TEMP_DIR/config.json"
fi

reject_json_unsafe "current image" "$CURRENT_IMAGE"
reject_json_unsafe "Compose project" "$COMPOSE_PROJECT"
case "$CURRENT_IMAGE" in
  "$IMAGE_REPOSITORY":*|"$IMAGE_REPOSITORY"@sha256:*) ;;
  *) fail "Current image $CURRENT_IMAGE does not belong to configured repository $IMAGE_REPOSITORY" ;;
esac
if ! [[ "$SOCKET_GID" =~ ^[0-9]+$ ]] || (( 10#$SOCKET_GID > 2147483647 )); then
  fail "Configured socket_gid must be a valid numeric GID"
fi
if ! [[ "$COMPOSE_PROJECT" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  fail "Compose project label is invalid: $COMPOSE_PROJECT"
fi
chmod 0600 "$TEMP_DIR/image.env" "$TEMP_DIR/config.json"
chmod 0644 "$TEMP_DIR/managed-upstream.conf"
if [[ -f "$TEMP_DIR/state.json" ]]; then
  chmod 0600 "$TEMP_DIR/state.json"
fi
if [[ -f "$TEMP_DIR/active-slot" ]]; then
  chmod 0644 "$TEMP_DIR/active-slot"
fi
"$TEMP_DIR/sub2api-deployer" -config "$TEMP_DIR/config.json" -check >/dev/null
compose_preflight "$TEMP_DIR/config.json" "$ASSET_DIR/compose.deployer.yml"

EXPECTED_PROXY="proxy_pass http://127.0.0.1:$CURRENT_PORT;"
MANAGED_PROXY="proxy_pass http://sub2api_managed;"
MANAGED_MATCHES=$(count_effective_directive "$NGINX_SITE" "$MANAGED_PROXY")
if (( EXISTING_INSTALL == 1 )); then
  [[ "$MANAGED_MATCHES" -eq 1 ]] || fail "Existing Nginx site must contain exactly one active '$MANAGED_PROXY' directive"
  cp -a "$NGINX_SITE" "$TEMP_DIR/nginx-site.conf"
else
  LEGACY_MATCHES=$(count_effective_directive "$NGINX_SITE" "$EXPECTED_PROXY")
  [[ "$MANAGED_MATCHES" -eq 0 && "$LEGACY_MATCHES" -eq 1 ]] || \
    fail "Expected exactly one active '$EXPECTED_PROXY' and no active managed proxy in $NGINX_SITE"
  awk -v expected="$EXPECTED_PROXY" -v replacement="$MANAGED_PROXY" '
    {
      line = $0
      sub(/#.*/, "", line)
      gsub(/[[:space:]]+/, " ", line)
      sub(/^ /, "", line)
      sub(/ $/, "", line)
      if (line == expected) {
        sub(/proxy_pass[[:space:]]+http:\/\/127\.0\.0\.1:[0-9]+;/, replacement)
        replaced++
      }
      print
    }
    END { if (replaced != 1) exit 1 }
  ' "$NGINX_SITE" > "$TEMP_DIR/nginx-site.conf"
fi

# The supplied URL and Host must already select the live legacy route. This
# catches a wrong vhost before any Nginx file is changed.
probe_nginx || fail "The Nginx probe did not reach the current application; verify --nginx-probe-url and --nginx-probe-host"

cp -a -- "$ASSET_DIR/sub2api-deployer.service" "$TEMP_DIR/sub2api-deployer.service"
chmod 0644 "$TEMP_DIR/sub2api-deployer.service"

for target in \
  "$INSTALLED_BINARY" \
  "$INSTALLED_UPGRADER" \
  "$INSTALL_DIR/compose.deployer.yml" \
  "$IMAGE_STATE_FILE" \
  "$MANAGED_UPSTREAM_FILE" \
  "$NGINX_LOADER_FILE" \
  "$CONFIG_FILE" \
  "$DOCKER_CONFIG_FILE" \
  "$NGINX_SITE" \
  "$SERVICE_FILE" \
  "$UPGRADE_SERVICE_FILE" \
  "$UPGRADE_TIMER_FILE" \
  "$TMPFILES_FILE" \
  "$STATE_FILE" \
  "$RUNTIME_MARKER"; do
  backup_target "$target"
done
for directory in "$CONFIG_DIR" "$DOCKER_CONFIG_DIR" "$STATE_DIR" "$RUNTIME_DIR" "$NGINX_STATE_DIR"; do
  backup_directory_metadata "$directory"
done
MUTATION_STARTED=1

install -d -m 0750 "$CONFIG_DIR"
if [[ -f "$TEMP_DIR/docker-config.json" || -f "$DOCKER_CONFIG_FILE" ]]; then
  install -d -m 0700 "$DOCKER_CONFIG_DIR"
fi
install -d -m 0700 "$STATE_DIR"
install -d -m 0755 "$RUNTIME_DIR" "$NGINX_STATE_DIR"
install -m 0755 "$TEMP_DIR/sub2api-deployer" "$INSTALLED_BINARY"
install -m 0644 "$ASSET_DIR/compose.deployer.yml" "$INSTALL_DIR/compose.deployer.yml"
if [[ -f "$TEMP_DIR/docker-config.json" ]]; then
  install -m 0600 "$TEMP_DIR/docker-config.json" "$DOCKER_CONFIG_FILE"
fi
install -m 0600 "$TEMP_DIR/image.env" "$IMAGE_STATE_FILE"
compose_preflight "$TEMP_DIR/config.json"
install -m 0644 "$TEMP_DIR/managed-upstream.conf" "$MANAGED_UPSTREAM_FILE"
if [[ -f "$TEMP_DIR/state.json" ]]; then
  install -m 0600 "$TEMP_DIR/state.json" "$STATE_FILE"
fi
if [[ -f "$TEMP_DIR/active-slot" ]]; then
  install -m 0644 "$TEMP_DIR/active-slot" "$RUNTIME_MARKER"
else
  rm -f -- "$RUNTIME_MARKER"
fi
install -m 0644 "$ASSET_DIR/sub2api-managed-upstream.conf" "$NGINX_LOADER_FILE"
install -m 0600 "$TEMP_DIR/config.json" "$CONFIG_FILE"
install -m 0644 "$TEMP_DIR/nginx-site.conf" "$NGINX_SITE"

nginx -t
NGINX_DUMP_FILE="$TEMP_DIR/nginx-dump.txt"
nginx -T > "$NGINX_DUMP_FILE" 2>&1
[[ $(count_effective_directive "$NGINX_DUMP_FILE" "$MANAGED_PROXY") -eq 1 ]] || \
  fail "Effective Nginx configuration must contain exactly one active '$MANAGED_PROXY'"
[[ $(count_effective_directive "$NGINX_DUMP_FILE" 'upstream sub2api_managed {') -eq 1 ]] || \
  fail "Effective Nginx configuration must contain exactly one managed upstream definition"

systemctl reload nginx
probe_nginx || fail "Nginx reload succeeded but the managed route health probe failed"

install -m 0644 "$TEMP_DIR/sub2api-deployer.service" "$SERVICE_FILE"
install -m 0644 "$ASSET_DIR/sub2api-deployer-upgrade.service" "$UPGRADE_SERVICE_FILE"
install -m 0644 "$ASSET_DIR/sub2api-deployer-upgrade.timer" "$UPGRADE_TIMER_FILE"
install -m 0644 "$ASSET_DIR/sub2api-deployer-tmpfiles.conf" "$TMPFILES_FILE"
rm -f -- "$RUNTIME_PRESERVE_DROPIN"
rmdir -- "$(dirname -- "$RUNTIME_PRESERVE_DROPIN")" 2>/dev/null || true
systemctl daemon-reload
if (( EXISTING_INSTALL == 0 || SERVICE_WAS_ENABLED == 1 )); then
  systemctl enable sub2api-deployer.service
else
  systemctl disable sub2api-deployer.service
fi
if (( EXISTING_INSTALL == 0 || UPGRADE_TIMER_WAS_ENABLED == 1 )); then
  systemctl enable sub2api-deployer-upgrade.timer
else
  systemctl disable sub2api-deployer-upgrade.timer
fi
# Keep the timer active during verification so the daemon can prove the live
# one-click capability. The activation lock prevents it from consuming work.
systemctl start sub2api-deployer-upgrade.timer
release_state_lock
systemctl restart sub2api-deployer.service
systemctl is-active --quiet sub2api-deployer.service

DEPLOYER_READY=0
for ((attempt=0; attempt<40; attempt++)); do
  if DEPLOYER_HEALTH=$(curl --fail --silent --max-time 2 --unix-socket /run/sub2api-deployer/deployer.sock http://localhost/v1/health 2>/dev/null) && \
    jq -e \
      --arg container "$ACTIVE_CONTAINER" \
      --arg container_id "$INSPECTED_CONTAINER_ID" \
      --arg version "$CURRENT_VERSION" \
      --argjson port "$ACTIVE_PORT" \
      '.status == "ok"
       and .degraded == false
       and .job_running == false
       and .active_container == $container
       and .active_container_id == $container_id
       and .active_port == $port
       and .active_version == $version
       and .control_plane_upgrade_ready == true
       and .control_plane.activator == "go-v1"
       and .control_plane.payload_schema_min <= 1
       and .control_plane.payload_schema_max >= 1' \
      <<<"$DEPLOYER_HEALTH" >/dev/null 2>&1; then
    DEPLOYER_READY=1
    break
  fi
  sleep 0.25
done
(( DEPLOYER_READY == 1 )) || fail "Deployer health check failed after restart"

EFFECTIVE_UPGRADE_EXEC=$(systemctl show --property=ExecStart --value sub2api-deployer-upgrade.service)
validate_effective_activator_exec "$EFFECTIVE_UPGRADE_EXEC" || \
  fail "Effective control-plane activator ExecStart does not exactly match the stable deployer flag contract"
EFFECTIVE_UPGRADE_DROPINS=$(systemctl show --property=DropInPaths --value sub2api-deployer-upgrade.service)
[[ -z "${EFFECTIVE_UPGRADE_DROPINS//[[:space:]]/}" ]] || \
  fail "Effective control-plane activator must not use systemd drop-ins"
[[ ! -e "$CONTROL_PLANE_UPGRADE_REQUEST" && ! -L "$CONTROL_PLANE_UPGRADE_REQUEST" ]] || \
  fail "Refusing to validate the new activator while an activation request is pending; inspect it with the deployer control-plane status command"
release_activation_lock
ACTIVATOR_STATUS_BEFORE=absent
if [[ -f "$CONTROL_PLANE_UPGRADE_REQUEST.status" && ! -L "$CONTROL_PLANE_UPGRADE_REQUEST.status" ]]; then
  ACTIVATOR_STATUS_BEFORE=$(sha256sum "$CONTROL_PLANE_UPGRADE_REQUEST.status" | awk '{print $1}')
elif [[ -e "$CONTROL_PLANE_UPGRADE_REQUEST.status" || -L "$CONTROL_PLANE_UPGRADE_REQUEST.status" ]]; then
  fail "Existing control-plane activation status path is unsafe"
fi
systemctl start sub2api-deployer-upgrade.service
if systemctl is-failed --quiet sub2api-deployer-upgrade.service; then
  fail "The Go activator did not exit cleanly on its normal no-request path"
fi
ACTIVATOR_STATUS_AFTER=absent
if [[ -f "$CONTROL_PLANE_UPGRADE_REQUEST.status" && ! -L "$CONTROL_PLANE_UPGRADE_REQUEST.status" ]]; then
  ACTIVATOR_STATUS_AFTER=$(sha256sum "$CONTROL_PLANE_UPGRADE_REQUEST.status" | awk '{print $1}')
fi
[[ "$ACTIVATOR_STATUS_AFTER" == "$ACTIVATOR_STATUS_BEFORE" ]] || \
  fail "The Go activator changed status state while no request existed"
rm -f -- "$INSTALLED_UPGRADER"

if (( EXISTING_INSTALL == 1 && SERVICE_WAS_ACTIVE == 1 )); then
  [[ $(stat -c '%i' "$SOCKET_DIRECTORY") == "$SOCKET_DIRECTORY_INODE" ]] || \
    fail "Deployer restart replaced the socket directory inode used by the running application container"
  assert_container_name_id "$ACTIVE_CONTAINER" "$INSPECTED_CONTAINER_ID"
  CONTAINER_SOCKET_DIRECTORY_INODE=$(docker exec "$INSPECTED_CONTAINER_ID" stat -c '%i' /run/sub2api-deployer) || \
    fail "Could not inspect the deployer socket directory from the running application container"
  [[ "$CONTAINER_SOCKET_DIRECTORY_INODE" == "$SOCKET_DIRECTORY_INODE" ]] || \
    fail "The running application container is bound to a stale deployer socket directory inode"
  APPLICATION_UID=$(docker exec "$INSPECTED_CONTAINER_ID" sh -ceu "awk '/^Uid:/{print \$2}' /proc/1/status") || \
    fail "Could not identify the running application user"
  [[ "$APPLICATION_UID" =~ ^[0-9]+$ ]] || fail "The running application returned an invalid process UID"
  docker exec --user "$APPLICATION_UID" "$INSPECTED_CONTAINER_ID" sh -ceu \
    'test -S /run/sub2api-deployer/deployer.sock && test -r /run/sub2api-deployer/deployer.sock && test -w /run/sub2api-deployer/deployer.sock' || \
    fail "The running application user cannot access the restarted deployer socket"
fi

if (( EXISTING_INSTALL == 1 )); then
  if (( UPGRADE_TIMER_WAS_ACTIVE == 1 )); then
    systemctl start sub2api-deployer-upgrade.timer
  else
    systemctl stop sub2api-deployer-upgrade.timer
  fi
  if (( SERVICE_WAS_ACTIVE == 0 )); then
    systemctl stop sub2api-deployer.service
  fi
fi

COMMITTED=1

cat <<EOF

Deployer installed and verified. Nginx still routes to the previously serving
application, and no application container was stopped or recreated.
EOF

if [[ -n "$ACTIVATION_VERSION" ]]; then
  cat <<EOF

The host control plane is ready for $ACTIVATION_VERSION. Start the first managed
deployment from the administrator update page so the verified release ledger
digest is included. Do not use a hand-written request without that digest.
EOF
elif (( EXISTING_INSTALL == 0 )); then
  cat <<'EOF'

After publishing the first managed image, use the administrator update page.
It supplies the verified release ledger digest required by the deployer.
EOF
fi
