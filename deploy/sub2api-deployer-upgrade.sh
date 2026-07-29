#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

REQUEST=/var/lib/sub2api-deployer/control-plane-upgrade.json
STATUS="$REQUEST.status"
STATE=/var/lib/sub2api-deployer/state.json
STAGING_ROOT=/var/lib/sub2api-deployer/control-plane-staging
QUARANTINE=/var/lib/sub2api-deployer/quarantine
BINARY=/usr/local/sbin/sub2api-deployer
LOCK=/run/sub2api-deployer-install.lock
SOCKET=/run/sub2api-deployer/deployer.sock
NEXT_BINARY=/usr/local/sbin/.sub2api-deployer.next
MAX_ATTEMPTS=5

JOB_ID=""
CONTAINER_ID=""
TARGET_VERSION=""
EXPECTED_COMMIT=""
EXPECTED_ARCH=""
ATTEMPT=0
STAGE=""
STAGED_BINARY=""
STAGED_SHA=""
PREVIOUS_BINARY=""
PREVIOUS_BINARY_NEXT=""
SWAPPED=0
HANDLING_FAILURE=0

read_identity() {
  local source="$1"
  [[ -f "$source" && ! -L "$source" ]] || return 1
  JOB_ID=$(jq -r '.job_id // empty' "$source" 2>/dev/null || true)
  CONTAINER_ID=$(jq -r '.container_id // empty' "$source" 2>/dev/null || true)
  TARGET_VERSION=$(jq -r '.target_version // empty' "$source" 2>/dev/null || true)
  ATTEMPT=$(jq -r '.attempt // 0' "$source" 2>/dev/null || printf '0')
  [[ "$ATTEMPT" =~ ^[0-9]+$ ]] || ATTEMPT=0
}

write_status() {
  local state="$1"
  local message="${2:-}"
  local error_class="${3:-}"
  local next_attempt_at="${4:-}"
  local temp_status="$STATUS.tmp.$$"
  local status_job_id="${JOB_ID:-unknown}"
  local status_container_id="${CONTAINER_ID:-unknown}"
  local status_target_version="${TARGET_VERSION:-unknown}"
  local updated_at
  updated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  jq -n \
    --arg job_id "$status_job_id" \
    --arg container_id "$status_container_id" \
    --arg target_version "$status_target_version" \
    --arg status "$state" \
    --arg error "$message" \
    --arg last_error "$message" \
    --arg error_class "$error_class" \
    --arg updated_at "$updated_at" \
    --arg next_attempt_at "$next_attempt_at" \
    --argjson attempt "$ATTEMPT" \
    --argjson max_attempts "$MAX_ATTEMPTS" '
      {
        schema: 1,
        job_id: $job_id,
        container_id: $container_id,
        target_version: $target_version,
        status: $status,
        attempt: $attempt,
        max_attempts: $max_attempts,
        updated_at: $updated_at
      }
      | if $error == "" then . else .error = $error | .last_error = $last_error end
      | if $error_class == "" then . else .error_class = $error_class end
      | if $next_attempt_at == "" then . else .next_attempt_at = $next_attempt_at end
    ' > "$temp_status"
  chmod 0600 "$temp_status"
  mv -f -- "$temp_status" "$STATUS"
}

quarantine_request() {
  [[ -e "$REQUEST" || -L "$REQUEST" ]] || return 0
  install -d -m 0700 "$QUARANTINE"
  mv -f -- "$REQUEST" "$QUARANTINE/control-plane-upgrade.$(date -u +%Y%m%dT%H%M%SZ).$$.json"
}

health_matches_target() {
  local health
  health=$(curl --fail --silent --max-time 2 --unix-socket "$SOCKET" http://localhost/v1/health 2>/dev/null) || return 1
  jq -e \
    --arg id "$CONTAINER_ID" \
    --arg version "$TARGET_VERSION" \
    --arg commit "$EXPECTED_COMMIT" \
    --arg arch "$EXPECTED_ARCH" '
      .status == "ok"
      and .degraded == false
      and .job_running == false
      and .active_container_id == $id
      and .active_version == $version
      and .control_plane_upgrade_ready == true
      and .build.version == $version
      and .build.commit == $commit
      and .build.type == "release"
      and .build.arch == $arch
    ' <<<"$health" >/dev/null
}

health_matches_application() {
  local health
  health=$(curl --fail --silent --max-time 2 --unix-socket "$SOCKET" http://localhost/v1/health 2>/dev/null) || return 1
  jq -e \
    --arg id "$CONTAINER_ID" \
    --arg version "$TARGET_VERSION" '
      .status == "ok"
      and .degraded == false
      and .job_running == false
      and .active_container_id == $id
      and .active_version == $version
      and .control_plane_upgrade_ready == true
    ' <<<"$health" >/dev/null
}

wait_for_health() {
  local predicate="$1"
  for ((probe=0; probe<40; probe++)); do
    if "$predicate"; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

restore_previous_binary() {
  (( SWAPPED == 1 )) || return 0
  [[ -f "$PREVIOUS_BINARY" && ! -L "$PREVIOUS_BINARY" ]] || return 1
  install -m 0755 "$PREVIOUS_BINARY" "$NEXT_BINARY"
  mv -f -- "$NEXT_BINARY" "$BINARY"
  SWAPPED=0
  systemctl restart sub2api-deployer.service || return 1
  wait_for_health health_matches_application
}

fail_permanent() {
  local message="$*"
  trap - ERR
  set +e
  HANDLING_FAILURE=1
  if ! restore_previous_binary; then
    message="$message; previous deployer could not be restored and verified"
  fi
  write_status failed "$message" permanent
  quarantine_request
  echo "sub2api-deployer control-plane upgrade: $message" >&2
  exit 1
}

fail_transient() {
  local message="$*"
  local next_attempt_at=""
  trap - ERR
  set +e
  HANDLING_FAILURE=1
  if ! restore_previous_binary; then
    message="$message; previous deployer could not be restored and verified"
    write_status failed "$message" permanent
    quarantine_request
    echo "sub2api-deployer control-plane upgrade: $message" >&2
    exit 1
  fi
  if (( ATTEMPT >= MAX_ATTEMPTS )); then
    write_status failed "$message" transient
    quarantine_request
  else
    next_attempt_at=$(date -u -d '60 seconds' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)
    write_status retrying "$message" transient "$next_attempt_at"
  fi
  echo "sub2api-deployer control-plane upgrade: $message" >&2
  exit 1
}

record_unexpected_failure() {
  local line="$1"
  local exit_status="$2"
  (( HANDLING_FAILURE == 0 )) || exit "$exit_status"
  fail_transient "activator exited unexpectedly at line $line (status $exit_status)"
}

cleanup() {
  rm -f -- "$NEXT_BINARY" "$STATUS.tmp.$$"
  [[ -z "$PREVIOUS_BINARY_NEXT" ]] || rm -f -- "$PREVIOUS_BINARY_NEXT"
}

trap cleanup EXIT
trap 'record_unexpected_failure "$LINENO" "$?"' ERR

for command in flock install jq systemctl curl sha256sum date; do
  command -v "$command" >/dev/null || fail_permanent "missing command: $command"
done

[[ -e "$REQUEST" || -L "$REQUEST" ]] || exit 0

# The manager writes status before scheduling activation, so malformed request
# data cannot erase the deployment identity needed to report a terminal state.
read_identity "$STATUS" || read_identity "$REQUEST" || true

exec 9<>"$LOCK"
chmod 0600 "$LOCK"
flock -n 9 || exit 0

[[ -f "$REQUEST" && ! -L "$REQUEST" ]] || fail_permanent "activation request is missing or unsafe"
[[ -f "$STATE" && ! -L "$STATE" ]] || fail_permanent "state file is missing or unsafe"
[[ -f "$BINARY" && -x "$BINARY" && ! -L "$BINARY" ]] || fail_permanent "installed deployer binary is missing or unsafe"
[[ -d "$STAGING_ROOT" && ! -L "$STAGING_ROOT" ]] || fail_permanent "control-plane staging root is missing or unsafe"

jq -e '
  .schema == 2
  and (.job_id | type == "string" and test("^[0-9A-Za-z._:-]{8,128}$"))
  and (.container_id | type == "string" and test("^[0-9a-f]{64}$"))
  and (.container_name | type == "string" and test("^[A-Za-z0-9][A-Za-z0-9_.-]+$"))
  and (.target_version | type == "string" and test("^[0-9][0-9A-Za-z.-]{0,63}$"))
  and (.expected_image | type == "string" and length > 0)
  and (.expected_image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.staged_binary_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.staged_manifest_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.expected_commit | type == "string" and test("^[0-9a-f]{7,64}$"))
  and (.expected_arch == "amd64" or .expected_arch == "arm64")
' "$REQUEST" >/dev/null || fail_permanent "activation request is malformed or unsupported"

JOB_ID=$(jq -r '.job_id' "$REQUEST")
CONTAINER_ID=$(jq -r '.container_id' "$REQUEST")
CONTAINER_NAME=$(jq -r '.container_name' "$REQUEST")
TARGET_VERSION=$(jq -r '.target_version' "$REQUEST")
EXPECTED_IMAGE=$(jq -r '.expected_image' "$REQUEST")
EXPECTED_DIGEST=$(jq -r '.expected_image_digest' "$REQUEST")
STAGED_BINARY=$(jq -r '.staged_binary' "$REQUEST")
STAGED_SHA=$(jq -r '.staged_binary_sha256' "$REQUEST")
STAGED_MANIFEST=$(jq -r '.staged_manifest' "$REQUEST")
STAGED_MANIFEST_SHA=$(jq -r '.staged_manifest_sha256' "$REQUEST")
EXPECTED_COMMIT=$(jq -r '.expected_commit' "$REQUEST")
EXPECTED_ARCH=$(jq -r '.expected_arch' "$REQUEST")

STAGE="$STAGING_ROOT/$JOB_ID"
PREVIOUS_BINARY="$STAGE/sub2api-deployer.previous"
PREVIOUS_BINARY_NEXT="$STAGE/.sub2api-deployer.previous.next"

# A terminal status is written before destructive cleanup. If the machine
# stops between those mutations, the timer only finishes cleanup and never
# repeats activation or turns a durable success into a failure.
TERMINAL_STATUS=$(jq -r \
  --arg job_id "$JOB_ID" \
  --arg id "$CONTAINER_ID" \
  --arg version "$TARGET_VERSION" '
    if .schema == 1
      and .job_id == $job_id
      and .container_id == $id
      and .target_version == $version
      and (.status == "succeeded" or .status == "failed")
    then .status else empty end
  ' "$STATUS" 2>/dev/null || true)
case "$TERMINAL_STATUS" in
  succeeded)
    rm -rf -- "$STAGE"
    rm -f -- "$REQUEST"
    exit 0
    ;;
  failed)
    quarantine_request
    exit 0
    ;;
esac

ATTEMPT=$((ATTEMPT + 1))
[[ -d "$STAGE" && ! -L "$STAGE" ]] || fail_permanent "verified staging directory is missing or unsafe"
[[ "$STAGED_BINARY" == "$STAGE/sub2api-deployer" ]] || fail_permanent "staged binary path escaped the verified staging directory"
[[ "$STAGED_MANIFEST" == "$STAGE/CONTROL-PLANE-MANIFEST.json" ]] || fail_permanent "staged manifest path escaped the verified staging directory"
[[ -f "$STAGED_BINARY" && -x "$STAGED_BINARY" && ! -L "$STAGED_BINARY" ]] || fail_permanent "staged deployer is missing or unsafe"
[[ -f "$STAGED_MANIFEST" && ! -L "$STAGED_MANIFEST" ]] || fail_permanent "staged manifest is missing or unsafe"
[[ "sha256:$(sha256sum "$STAGED_BINARY" | awk '{print $1}')" == "$STAGED_SHA" ]] || fail_permanent "staged deployer digest no longer matches the verified request"
[[ "sha256:$(sha256sum "$STAGED_MANIFEST" | awk '{print $1}')" == "$STAGED_MANIFEST_SHA" ]] || fail_permanent "staged manifest digest no longer matches the verified request"
[[ "$EXPECTED_IMAGE" == *"@$EXPECTED_DIGEST" ]] || fail_permanent "requested immutable image does not contain the expected digest"

jq -e \
  --arg job_id "$JOB_ID" \
  --arg id "$CONTAINER_ID" \
  --arg name "$CONTAINER_NAME" \
  --arg version "$TARGET_VERSION" \
  --arg image "$EXPECTED_IMAGE" '
    .degraded == false
    and .active_container_id == $id
    and .active_container == $name
    and .active_version == $version
    and .active_image == $image
    and .job.id == $job_id
    and .job.status == "succeeded"
  ' "$STATE" >/dev/null || fail_permanent "activation request no longer matches the successful active deployment"

CURRENT_SHA="sha256:$(sha256sum "$BINARY" | awk '{print $1}')"
if [[ "$CURRENT_SHA" == "$STAGED_SHA" ]] && health_matches_target; then
  write_status succeeded
  rm -rf -- "$STAGE"
  rm -f -- "$REQUEST"
  exit 0
fi

if [[ ! -e "$PREVIOUS_BINARY" ]]; then
  install -m 0755 "$BINARY" "$PREVIOUS_BINARY_NEXT"
  [[ "sha256:$(sha256sum "$PREVIOUS_BINARY_NEXT" | awk '{print $1}')" == "$CURRENT_SHA" ]] || \
    fail_permanent "previous deployer recovery copy failed verification"
  mv -f -- "$PREVIOUS_BINARY_NEXT" "$PREVIOUS_BINARY"
elif [[ ! -f "$PREVIOUS_BINARY" || -L "$PREVIOUS_BINARY" ]]; then
  fail_permanent "previous deployer recovery file is unsafe"
fi

if [[ "$CURRENT_SHA" != "$STAGED_SHA" ]]; then
  install -m 0755 "$STAGED_BINARY" "$NEXT_BINARY"
  mv -f -- "$NEXT_BINARY" "$BINARY"
fi
SWAPPED=1

systemctl restart sub2api-deployer.service || true
if wait_for_health health_matches_target; then
  SWAPPED=0
  write_status succeeded
  rm -rf -- "$STAGE"
  rm -f -- "$REQUEST"
  exit 0
fi

fail_transient "new deployer failed health and build identity verification"
