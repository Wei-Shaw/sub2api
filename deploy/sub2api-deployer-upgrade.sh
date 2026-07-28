#!/usr/bin/env bash

set -euo pipefail
umask 077

REQUEST=/var/lib/sub2api-deployer/control-plane-upgrade.json
STATUS="$REQUEST.status"
STATE=/var/lib/sub2api-deployer/state.json
CONFIG=/etc/sub2api-deployer/config.json
BINARY=/usr/local/sbin/sub2api-deployer
LOCK=/run/sub2api-deployer-install.lock
SOCKET=/run/sub2api-deployer/deployer.sock
NEXT_BINARY=/usr/local/sbin/.sub2api-deployer.next
JOB_ID=""
CONTAINER_ID=""
TARGET_VERSION=""

write_status() {
  local state="$1"
  local message="${2:-}"
  local temp_status="$STATUS.tmp.$$"
  [[ -n "$JOB_ID" && -n "$CONTAINER_ID" && -n "$TARGET_VERSION" ]] || return 0
  jq -n \
    --arg job_id "$JOB_ID" \
    --arg container_id "$CONTAINER_ID" \
    --arg target_version "$TARGET_VERSION" \
    --arg status "$state" \
    --arg error "$message" \
    '{schema: 1, job_id: $job_id, container_id: $container_id, target_version: $target_version, status: $status}
     | if $error == "" then . else .error = $error end' > "$temp_status"
  chmod 0600 "$temp_status"
  mv -f -- "$temp_status" "$STATUS"
}

record_unexpected_failure() {
  local line="$1"
  local exit_status="$2"
  trap - ERR
  set +e
  write_status failed "upgrade helper exited unexpectedly at line $line (status $exit_status)"
  exit "$exit_status"
}

trap 'record_unexpected_failure "$LINENO" "$?"' ERR

fail() {
  local message="$*"
  write_status failed "$message" || true
  echo "sub2api-deployer control-plane upgrade: $message" >&2
  exit 1
}

for command in docker flock install jq systemctl curl mktemp sha256sum; do
  command -v "$command" >/dev/null || fail "missing command: $command"
done

[[ -f "$REQUEST" && ! -L "$REQUEST" ]] || exit 0

# Load enough identity before strict validation so failures in the validation
# path can still be associated with the pending application deployment.
JOB_ID=$(jq -r '.job_id // empty' "$REQUEST" 2>/dev/null || true)
CONTAINER_ID=$(jq -r '.container_id // empty' "$REQUEST" 2>/dev/null || true)
TARGET_VERSION=$(jq -r '.target_version // empty' "$REQUEST" 2>/dev/null || true)
[[ -f "$STATE" && ! -L "$STATE" ]] || fail "state file is missing or unsafe"
[[ -f "$CONFIG" && ! -L "$CONFIG" ]] || fail "config file is missing or unsafe"
[[ -x "$BINARY" && ! -L "$BINARY" ]] || fail "installed deployer binary is missing or unsafe"

exec 9<>"$LOCK"
chmod 0600 "$LOCK"
flock -n 9 || fail "another deployer installation or upgrade is active"

JOB_ID=$(jq -er '.job_id | strings | select(test("^[0-9A-Za-z._:-]{8,128}$"))' "$REQUEST")
CONTAINER_ID=$(jq -er '.container_id | strings | select(test("^[0-9a-f]{64}$"))' "$REQUEST")
CONTAINER_NAME=$(jq -er '.container_name | strings | select(test("^[A-Za-z0-9][A-Za-z0-9_.-]+$"))' "$REQUEST")
TARGET_VERSION=$(jq -er '.target_version | strings | select(test("^[0-9][0-9A-Za-z.-]{0,63}$"))' "$REQUEST")
EXPECTED_IMAGE=$(jq -er '.expected_image | strings | select(length > 0)' "$REQUEST")
EXPECTED_DIGEST=$(jq -er '.expected_image_digest | strings | select(test("^sha256:[0-9a-f]{64}$"))' "$REQUEST")
jq -e '.schema == 1' "$REQUEST" >/dev/null || fail "unsupported request schema"

jq -e \
  --arg id "$CONTAINER_ID" \
  --arg name "$CONTAINER_NAME" \
  --arg version "$TARGET_VERSION" \
  --arg image "$EXPECTED_IMAGE" '
    .degraded == false
    and .active_container_id == $id
    and .active_container == $name
    and .active_version == $version
    and .active_image == $image
    and .job.status == "succeeded"
  ' "$STATE" >/dev/null || fail "request no longer matches the successful active deployment"

ACTUAL_ID=$(docker inspect "$CONTAINER_ID" --format '{{.Id}}')
ACTUAL_NAME=$(docker inspect "$CONTAINER_ID" --format '{{trimPrefix "/" .Name}}')
ACTUAL_IMAGE=$(docker inspect "$CONTAINER_ID" --format '{{.Config.Image}}')
[[ "$ACTUAL_ID" == "$CONTAINER_ID" ]] || fail "container identity drifted"
[[ "$ACTUAL_NAME" == "$CONTAINER_NAME" ]] || fail "container name drifted"
[[ "$EXPECTED_IMAGE" == *"@$EXPECTED_DIGEST" ]] || fail "requested immutable image does not contain expected digest"
[[ "$ACTUAL_IMAGE" == "$EXPECTED_IMAGE" ]] || fail "running container immutable image does not match request"

STAGE=$(mktemp -d "$(dirname -- "$REQUEST")/control-plane-upgrade.XXXXXX")
cleanup() {
  rm -f -- "$NEXT_BINARY"
  rm -rf -- "$STAGE"
}
trap cleanup EXIT

docker cp "$CONTAINER_ID:/app/sub2api-deployer" "$STAGE/sub2api-deployer"
chmod 0755 "$STAGE/sub2api-deployer"
"$STAGE/sub2api-deployer" -config "$CONFIG" -check >/dev/null

CURRENT_SHA=$(sha256sum "$BINARY" | awk '{print $1}')
TARGET_SHA=$(sha256sum "$STAGE/sub2api-deployer" | awk '{print $1}')
if [[ "$CURRENT_SHA" == "$TARGET_SHA" ]]; then
  rm -f -- "$REQUEST"
  write_status succeeded
  exit 0
fi

install -m 0755 "$BINARY" "$STAGE/sub2api-deployer.previous"
install -m 0755 "$STAGE/sub2api-deployer" "$NEXT_BINARY"
mv -f -- "$NEXT_BINARY" "$BINARY"
if systemctl restart sub2api-deployer.service; then
  for ((attempt=0; attempt<40; attempt++)); do
    if HEALTH=$(curl --fail --silent --max-time 2 --unix-socket "$SOCKET" http://localhost/v1/health 2>/dev/null) &&
      jq -e \
        --arg id "$CONTAINER_ID" \
        --arg version "$TARGET_VERSION" '
          .status == "ok"
          and .degraded == false
          and .job_running == false
          and .active_container_id == $id
          and .active_version == $version
          and .control_plane_upgrade_ready == true
        ' <<<"$HEALTH" >/dev/null; then
      rm -f -- "$REQUEST"
      write_status succeeded
      exit 0
    fi
    sleep 0.25
  done
fi

install -m 0755 "$STAGE/sub2api-deployer.previous" "$NEXT_BINARY"
mv -f -- "$NEXT_BINARY" "$BINARY"
systemctl restart sub2api-deployer.service || fail "new deployer failed and previous deployer could not be restarted"
fail "new deployer failed health verification; previous binary was restored"
