#!/usr/bin/env bash

set -euo pipefail
umask 077

[[ "${SUB2API_SYSTEMD_TEST_EPHEMERAL:-}" == "1" ]] || {
  echo "refusing to modify systemd outside an explicitly ephemeral test host" >&2
  exit 2
}
[[ $(id -u) == 0 ]] || { echo "systemd integration test must run as root" >&2; exit 2; }
[[ $(uname -s) == Linux ]] || { echo "systemd integration test requires Linux" >&2; exit 2; }
[[ $# == 4 ]] || { echo "usage: $0 <deployer-a> <deployer-b> <deployer-c> <deployer-d>" >&2; exit 2; }
for binary in "$@"; do
  [[ -x "$binary" ]] || { echo "deployer test binary is not executable: $binary" >&2; exit 2; }
done

BINARY_A=$(readlink -f -- "$1")
BINARY_B=$(readlink -f -- "$2")
BINARY_C=$(readlink -f -- "$3")
BINARY_D=$(readlink -f -- "$4")
REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
INSTALLED_BINARY=/usr/local/sbin/sub2api-deployer
REAL_BINARY=/usr/local/sbin/sub2api-deployer.systemd-test-real
HEALTH_FIXTURE=/usr/local/libexec/sub2api-systemd-health-fixture.py
CONFIG_DIR=/etc/sub2api-deployer
CONFIG_FILE="$CONFIG_DIR/config.json"
STATE_ROOT=/var/lib/sub2api-deployer/systemd-activation-test
REQUEST="$STATE_ROOT/control-plane-upgrade.json"
STATUS="$REQUEST.status"
STATE_FILE="$STATE_ROOT/state.json"
SOCKET_FILE=/run/sub2api-deployer/deployer.sock
REJECT_SHA_FILE="$STATE_ROOT/reject-service-sha"
SERVICE_FILE=/etc/systemd/system/sub2api-deployer.service
UPGRADE_FILE=/etc/systemd/system/sub2api-deployer-upgrade.service
TIMER_FILE=/etc/systemd/system/sub2api-deployer-upgrade.timer
TIMER_DROPIN=/etc/systemd/system/sub2api-deployer-upgrade.timer.d/90-systemd-test.conf
ARM_FILE="$STATE_ROOT/arm-wrapper-delay"
SLEEPING_FILE="$STATE_ROOT/wrapper-sleeping"
SLEPT_FILE="$STATE_ROOT/wrapper-slept"

for path in "$INSTALLED_BINARY" "$REAL_BINARY" "$HEALTH_FIXTURE" "$CONFIG_FILE" "$SERVICE_FILE" "$UPGRADE_FILE" "$TIMER_FILE"; do
  [[ ! -e "$path" && ! -L "$path" ]] || {
    echo "ephemeral host is not clean; refusing to replace $path" >&2
    exit 2
  }
done
systemctl is-system-running --wait >/dev/null 2>&1 || {
  state=$(systemctl is-system-running 2>/dev/null || true)
  [[ "$state" == running || "$state" == degraded ]] || { echo "systemd is not usable: $state" >&2; exit 2; }
}
systemctl is-active --quiet docker.service || { echo "docker.service must be active" >&2; exit 2; }

cleanup() {
  set +e
  systemctl disable --now sub2api-deployer-upgrade.timer >/dev/null 2>&1
  systemctl stop sub2api-deployer-upgrade.service sub2api-deployer.service >/dev/null 2>&1
  rm -f -- "$TIMER_DROPIN" "$UPGRADE_FILE" "$TIMER_FILE" "$SERVICE_FILE"
  rmdir -- "$(dirname -- "$TIMER_DROPIN")" >/dev/null 2>&1
  systemctl daemon-reload >/dev/null 2>&1
  rm -f -- "$INSTALLED_BINARY" "$REAL_BINARY" "$HEALTH_FIXTURE" "$CONFIG_FILE" "$SOCKET_FILE"
  rm -rf -- "$STATE_ROOT"
  rmdir -- "$CONFIG_DIR" >/dev/null 2>&1
}
trap cleanup EXIT

install -d -m 0700 "$CONFIG_DIR" "$STATE_ROOT"
install -d -m 0755 "$(dirname -- "$HEALTH_FIXTURE")" "$(dirname -- "$SOCKET_FILE")"
install -m 0755 "$BINARY_A" "$REAL_BINARY"
install -m 0755 "$REPO_ROOT/deploy/tests/systemd-health-fixture.py" "$HEALTH_FIXTURE"
cat > "$INSTALLED_BINARY" <<EOF
#!/usr/bin/env bash
set -euo pipefail
if [[ "\${1:-}" == "--activate-staged-control-plane" && -e "$ARM_FILE" && ! -e "$SLEPT_FILE" ]]; then
  : > "$SLEPT_FILE"
  : > "$SLEEPING_FILE"
  sleep 30
fi
exec "$REAL_BINARY" "\$@"
EOF
chmod 0755 "$INSTALLED_BINARY"

jq \
  --arg state "$STATE_ROOT/state.json" \
  --arg request "$REQUEST" '
    .state_path = $state
    | .control_plane_upgrade_path = $request
    | .control_plane_upgrade_command = ["/usr/bin/systemctl", "start", "--no-block", "sub2api-deployer-upgrade.service"]
  ' "$REPO_ROOT/deploy/sub2api-deployer.example.json" > "$CONFIG_FILE"
"$INSTALLED_BINARY" -config "$CONFIG_FILE" -check >/dev/null

cat > "$SERVICE_FILE" <<'EOF'
[Unit]
Description=Sub2API systemd integration test daemon

[Service]
Type=simple
ExecStart=/bin/sleep infinity
EOF
install -m 0644 "$REPO_ROOT/deploy/sub2api-deployer-upgrade.service" "$UPGRADE_FILE"
install -m 0644 "$REPO_ROOT/deploy/sub2api-deployer-upgrade.timer" "$TIMER_FILE"
install -d -m 0755 "$(dirname -- "$TIMER_DROPIN")"
cat > "$TIMER_DROPIN" <<'EOF'
[Timer]
OnBootSec=
OnUnitInactiveSec=
OnBootSec=1s
OnUnitInactiveSec=2s
AccuracySec=100ms
EOF
chmod 0644 "$SERVICE_FILE" "$TIMER_DROPIN"
systemctl daemon-reload
systemd-analyze verify "$SERVICE_FILE" "$UPGRADE_FILE" "$TIMER_FILE"
systemctl start sub2api-deployer.service
DAEMON_PID=$(systemctl show --property=MainPID --value sub2api-deployer.service)
[[ "$DAEMON_PID" =~ ^[1-9][0-9]*$ ]]

EFFECTIVE=$(systemctl show --property=ExecStart --value sub2api-deployer-upgrade.service)
[[ -z "$(systemctl show --property=DropInPaths --value sub2api-deployer-upgrade.service)" ]]
[[ $(grep -oF 'argv[]=' <<<"$EFFECTIVE" | wc -l) == 1 ]]
[[ $(grep -oE '(^|[;{][[:space:]]*)path=' <<<"$EFFECTIVE" | wc -l) == 1 ]]
[[ "$EFFECTIVE" == *'path=/usr/local/sbin/sub2api-deployer ;'* ]]
[[ "$EFFECTIVE" == *'argv[]=/usr/local/sbin/sub2api-deployer --activate-staged-control-plane ;'* ]]

# No request is the normal path. It must stay silent and leave the daemon and
# state untouched across both manual invocations and real timer cycles.
START_EPOCH=$(date +%s)
for _ in 1 2 3; do
  systemctl start sub2api-deployer-upgrade.service
  ! systemctl is-failed --quiet sub2api-deployer-upgrade.service
done
[[ ! -e "$REQUEST" && ! -e "$STATUS" ]]
[[ $(systemctl show --property=MainPID --value sub2api-deployer.service) == "$DAEMON_PID" ]]

PREVIOUS_INVOCATION=$(systemctl show --property=InvocationID --value sub2api-deployer-upgrade.service)
systemctl enable --now sub2api-deployer-upgrade.timer
CHANGES=0
for _ in $(seq 1 80); do
  CURRENT_INVOCATION=$(systemctl show --property=InvocationID --value sub2api-deployer-upgrade.service)
  if [[ -n "$CURRENT_INVOCATION" && "$CURRENT_INVOCATION" != "$PREVIOUS_INVOCATION" ]]; then
    CHANGES=$((CHANGES + 1))
    PREVIOUS_INVOCATION=$CURRENT_INVOCATION
  fi
  (( CHANGES >= 2 )) && break
  sleep 0.25
done
(( CHANGES >= 2 )) || { echo "timer did not execute two clean no-request cycles" >&2; exit 1; }
! systemctl is-failed --quiet sub2api-deployer-upgrade.service
[[ ! -e "$STATUS" ]]
[[ $(systemctl show --property=MainPID --value sub2api-deployer.service) == "$DAEMON_PID" ]]
if journalctl -u sub2api-deployer-upgrade.service --since "@$START_EPOCH" --no-pager -o cat | \
    grep -Eq 'configuration is valid|activate staged control plane'; then
  echo "no-request timer path emitted application log noise" >&2
  exit 1
fi

# Kill the activator before it can consume a malformed request. The next timer
# run must retain and then quarantine the request; a following no-op run must
# clear systemd's failed state without touching the live daemon.
printf '%s\n' '{malformed' > "$REQUEST"
chmod 0600 "$REQUEST"
: > "$ARM_FILE"
systemctl start --no-block sub2api-deployer-upgrade.service
for _ in $(seq 1 40); do
  [[ -e "$SLEEPING_FILE" ]] && break
  sleep 0.1
done
[[ -e "$SLEEPING_FILE" ]] || { echo "activator wrapper did not enter the injected crash window" >&2; exit 1; }
systemctl kill --kill-who=main --signal=KILL sub2api-deployer-upgrade.service
for _ in $(seq 1 120); do
  [[ ! -e "$REQUEST" && -e "$STATUS" ]] && break
  sleep 0.25
done
[[ ! -e "$REQUEST" && -f "$STATUS" ]] || { echo "timer did not converge the killed activation request" >&2; exit 1; }
jq -e '.status == "failed" and .error_class == "permanent"' "$STATUS" >/dev/null
find "$STATE_ROOT/quarantine" -maxdepth 1 -type f -name 'control-plane-upgrade.*.json' | grep -q .
for _ in $(seq 1 40); do
  ! systemctl is-failed --quiet sub2api-deployer-upgrade.service && break
  sleep 0.25
done
! systemctl is-failed --quiet sub2api-deployer-upgrade.service
[[ $(systemctl show --property=MainPID --value sub2api-deployer.service) == "$DAEMON_PID" ]]

# The timer recovery checks above deliberately use a wrapper and a dummy live
# service. Switch to real release-identity binaries and a real Unix-socket
# health fixture for the host replacement transaction.
systemctl disable --now sub2api-deployer-upgrade.timer
systemctl stop sub2api-deployer.service
install -m 0755 "$BINARY_A" "$INSTALLED_BINARY"
rm -f -- "$REAL_BINARY" "$ARM_FILE" "$SLEEPING_FILE" "$SLEPT_FILE" "$STATUS"

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Sub2API systemd integration test health fixture

[Service]
Type=simple
ExecStart=/usr/bin/python3 $HEALTH_FIXTURE --socket $SOCKET_FILE --binary $INSTALLED_BINARY --state $STATE_FILE --reject-sha-file $REJECT_SHA_FILE
EOF
chmod 0644 "$SERVICE_FILE"
systemctl daemon-reload
systemd-analyze verify "$SERVICE_FILE" "$UPGRADE_FILE" "$TIMER_FILE"

sha256_uri() {
  sha256sum "$1" | awk '{print "sha256:" $1}'
}

stage_activation() {
  local job_id=$1
  local target_version=$2
  local expected_commit=$3
  local target_binary=$4
  local stage="$STATE_ROOT/control-plane-staging/$job_id"
  local target_sha manifest_sha created_at arch image_digest expected_image container_id
  target_sha=$(sha256_uri "$target_binary")
  created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  arch=$(uname -m)
  case "$arch" in
    x86_64) arch=amd64 ;;
    aarch64) arch=arm64 ;;
    *) echo "unsupported systemd test architecture: $arch" >&2; return 1 ;;
  esac
  image_digest="sha256:$(printf '%064d' 9)"
  expected_image="example.invalid/sub2api@$image_digest"
  container_id=$(printf '%064d' "${target_version##*.}")

  install -d -m 0700 "$STATE_ROOT/control-plane-staging"
  install -d -m 0700 "$stage"
  install -m 0755 "$target_binary" "$stage/sub2api-deployer"
  jq -n \
    --arg version "$target_version" \
    --arg commit "$expected_commit" \
    --arg arch "$arch" \
    --arg sha "$target_sha" \
    '{schema:1,version:$version,commit:$commit,arch:$arch,runtime_payload:[{type:"sub2api-deployer",path:"sub2api-deployer",sha256:$sha,owner:0,group:0,mode:493}]}' \
    > "$stage/CONTROL-PLANE-MANIFEST.json"
  chmod 0644 "$stage/CONTROL-PLANE-MANIFEST.json"
  manifest_sha=$(sha256_uri "$stage/CONTROL-PLANE-MANIFEST.json")

  jq -n \
    --arg container_id "$container_id" \
    --arg version "$target_version" \
    --arg image "$expected_image" \
    --arg job_id "$job_id" \
    '{active_container:"sub2api-blue",active_container_id:$container_id,active_version:$version,active_image:$image,degraded:false,job:{id:$job_id,status:"succeeded"}}' \
    > "$STATE_FILE"
  chmod 0600 "$STATE_FILE"
  jq -n \
    --arg job_id "$job_id" \
    --arg container_id "$container_id" \
    --arg version "$target_version" \
    --arg updated_at "$created_at" \
    '{schema:1,job_id:$job_id,container_id:$container_id,target_version:$version,status:"pending",attempt:0,max_attempts:5,updated_at:$updated_at}' \
    > "$STATUS"
  chmod 0600 "$STATUS"
  jq -n \
    --arg job_id "$job_id" \
    --arg container_id "$container_id" \
    --arg version "$target_version" \
    --arg image "$expected_image" \
    --arg image_digest "$image_digest" \
    --arg manifest "$stage/CONTROL-PLANE-MANIFEST.json" \
    --arg manifest_sha "$manifest_sha" \
    --arg commit "$expected_commit" \
    --arg arch "$arch" \
    --arg created_at "$created_at" \
    --arg staged_binary "$stage/sub2api-deployer" \
    --arg target_sha "$target_sha" \
    '{schema:2,payload_schema:1,job_id:$job_id,container_id:$container_id,container_name:"sub2api-blue",target_version:$version,expected_image:$image,expected_image_digest:$image_digest,staged_manifest:$manifest,staged_manifest_sha256:$manifest_sha,expected_commit:$commit,expected_arch:$arch,max_attempts:5,created_at:$created_at,assets:[{type:"sub2api-deployer",staged_path:$staged_binary,sha256:$target_sha,owner:0,group:0,mode:493}]}' \
    > "$REQUEST.next"
  chmod 0600 "$REQUEST.next"
  mv -f -- "$REQUEST.next" "$REQUEST"
}

assert_installed_binary() {
  local expected=$1
  [[ $(sha256_uri "$INSTALLED_BINARY") == "$(sha256_uri "$expected")" ]]
}

assert_socket_health() {
  local expected_version=$1
  local expected_binary=$2
  local expected_sha
  expected_sha=$(sha256_uri "$expected_binary")
  curl --fail --silent --show-error --unix-socket "$SOCKET_FILE" http://localhost/v1/health | \
    jq -e \
      --arg version "$expected_version" \
      --arg sha "$expected_sha" \
      '.status == "ok" and .degraded == false and .job_running == false and .control_plane_upgrade_ready == true and .build.version == $version and .build.sha256 == $sha and .control_plane.installed_sha256 == $sha' \
      >/dev/null
}

# Run two consecutive real control-plane upgrades. The second proves that the
# first replacement leaves a deployer capable of handling the next version.
stage_activation activation-a-to-b 0.1.168-ts.2 "$(printf '%040d' 2)" "$BINARY_B"
systemctl start sub2api-deployer.service
INVOCATION_A=$(systemctl show --property=InvocationID --value sub2api-deployer.service)
systemctl start sub2api-deployer-upgrade.service
assert_installed_binary "$BINARY_B"
assert_socket_health 0.1.168-ts.2 "$BINARY_B"
[[ ! -e "$REQUEST" ]]
jq -e '.status == "succeeded" and .attempt == 1' "$STATUS" >/dev/null
INVOCATION_B=$(systemctl show --property=InvocationID --value sub2api-deployer.service)
[[ "$INVOCATION_B" != "$INVOCATION_A" ]]

stage_activation activation-b-to-c 0.1.168-ts.3 "$(printf '%040d' 3)" "$BINARY_C"
systemctl start sub2api-deployer-upgrade.service
assert_installed_binary "$BINARY_C"
assert_socket_health 0.1.168-ts.3 "$BINARY_C"
[[ ! -e "$REQUEST" ]]
jq -e '.status == "succeeded" and .attempt == 1' "$STATUS" >/dev/null
INVOCATION_C=$(systemctl show --property=InvocationID --value sub2api-deployer.service)
[[ "$INVOCATION_C" != "$INVOCATION_B" ]]

# Replaying the already-installed SHA is a successful no-op and must not
# restart the live daemon.
stage_activation activation-c-noop 0.1.168-ts.3 "$(printf '%040d' 3)" "$BINARY_C"
systemctl start sub2api-deployer-upgrade.service
assert_installed_binary "$BINARY_C"
assert_socket_health 0.1.168-ts.3 "$BINARY_C"
[[ $(systemctl show --property=InvocationID --value sub2api-deployer.service) == "$INVOCATION_C" ]]
jq -e '.status == "succeeded" and .attempt == 1' "$STATUS" >/dev/null

# A target that cannot start must restore the previous bytes and live service.
# Removing the fault and retrying the retained request must then converge.
D_SHA=$(sha256_uri "$BINARY_D")
printf '%s\n' "$D_SHA" > "$REJECT_SHA_FILE"
chmod 0600 "$REJECT_SHA_FILE"
stage_activation activation-c-to-d-retry 0.1.168-ts.4 "$(printf '%040d' 4)" "$BINARY_D"
if systemctl start sub2api-deployer-upgrade.service; then
  echo "activation unexpectedly succeeded while target health was rejected" >&2
  exit 1
fi
assert_installed_binary "$BINARY_C"
systemctl is-active --quiet sub2api-deployer.service
assert_socket_health 0.1.168-ts.3 "$BINARY_C"
jq -e '.status == "retrying" and .attempt == 1 and .error_class == "transient"' "$STATUS" >/dev/null
[[ -f "$REQUEST" ]]
rm -f -- "$REJECT_SHA_FILE"
systemctl reset-failed sub2api-deployer-upgrade.service
systemctl start sub2api-deployer-upgrade.service
assert_installed_binary "$BINARY_D"
assert_socket_health 0.1.168-ts.4 "$BINARY_D"
[[ ! -e "$REQUEST" ]]
jq -e '.status == "succeeded" and .attempt == 2' "$STATUS" >/dev/null
systemctl is-active --quiet sub2api-deployer.service

echo "real systemd control-plane activation tests passed"
