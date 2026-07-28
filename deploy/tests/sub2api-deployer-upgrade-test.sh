#!/usr/bin/env bash

set -euo pipefail
umask 077

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-deployer-upgrade-test.XXXXXX")
trap 'rm -rf -- "$TEST_DIR"' EXIT

ROOT="$TEST_DIR/root"
FAKE_BIN="$TEST_DIR/bin"
CONTAINER_ID=$(printf 'a%.0s' {1..64})
WRONG_CONTAINER_ID=$(printf 'b%.0s' {1..64})
TARGET_VERSION="0.1.166-ts.3"
TARGET_DIGEST="sha256:$(printf 'c%.0s' {1..64})"
TARGET_IMAGE="ghcr.io/ssharkkky/sub2api@$TARGET_DIGEST"

mkdir -p \
  "$FAKE_BIN" \
  "$ROOT/etc/sub2api-deployer" \
  "$ROOT/run/sub2api-deployer" \
  "$ROOT/usr/local/sbin" \
  "$ROOT/var/lib/sub2api-deployer"

cat > "$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  inspect)
    case "$4" in
      '{{.Id}}') printf '%s\n' "$FAKE_CONTAINER_ID" ;;
      '{{trimPrefix "/" .Name}}') printf '%s\n' 'sub2api-green' ;;
      '{{.Config.Image}}') printf '%s\n' "$FAKE_TARGET_IMAGE" ;;
      *) exit 2 ;;
    esac
    ;;
  cp)
    install -m 0755 "$FAKE_CANDIDATE_BINARY" "$3"
    ;;
  *) exit 2 ;;
esac
EOF

cat > "$FAKE_BIN/systemctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_SYSTEMCTL_LOG"
[[ "$1" == "restart" && "$2" == "sub2api-deployer.service" ]]
EOF

cat > "$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$FAKE_HEALTH"
EOF

cat > "$FAKE_BIN/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$FAKE_BIN/flock" <<'EOF'
#!/usr/bin/env bash
[[ "$1" == "-n" && "$2" == "9" ]]
EOF
chmod 0755 "$FAKE_BIN/docker" "$FAKE_BIN/systemctl" "$FAKE_BIN/curl" "$FAKE_BIN/sleep" "$FAKE_BIN/flock"

cat > "$TEST_DIR/candidate" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "-config" && -n "$2" && "$3" == "-check" ]]
EOF
chmod 0755 "$TEST_DIR/candidate"

HELPER="$TEST_DIR/sub2api-deployer-upgrade"
sed \
  -e "s#^REQUEST=.*#REQUEST=\"$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json\"#" \
  -e "s#^STATE=.*#STATE=\"$ROOT/var/lib/sub2api-deployer/state.json\"#" \
  -e "s#^CONFIG=.*#CONFIG=\"$ROOT/etc/sub2api-deployer/config.json\"#" \
  -e "s#^BINARY=.*#BINARY=\"$ROOT/usr/local/sbin/sub2api-deployer\"#" \
  -e "s#^LOCK=.*#LOCK=\"$ROOT/run/sub2api-deployer-install.lock\"#" \
  -e "s#^SOCKET=.*#SOCKET=\"$ROOT/run/sub2api-deployer/deployer.sock\"#" \
  -e "s#^NEXT_BINARY=.*#NEXT_BINARY=\"$ROOT/usr/local/sbin/.sub2api-deployer.next\"#" \
  "$REPO_ROOT/deploy/sub2api-deployer-upgrade.sh" > "$HELPER"
chmod 0755 "$HELPER"

write_fixture() {
  local current_marker="$1"
  printf '#!/usr/bin/env bash\n# %s\nexit 0\n' "$current_marker" > "$ROOT/usr/local/sbin/sub2api-deployer"
  chmod 0755 "$ROOT/usr/local/sbin/sub2api-deployer"
  printf '{}\n' > "$ROOT/etc/sub2api-deployer/config.json"
  jq -n \
    --arg job_id "control-plane-upgrade-0001" \
    --arg id "$CONTAINER_ID" \
    --arg version "$TARGET_VERSION" \
    --arg image "$TARGET_IMAGE" \
    '{schema: 1, job_id: $job_id, container_id: $id, container_name: "sub2api-green", target_version: $version, expected_image: $image, expected_image_digest: ($image | split("@") | .[1])}' \
    > "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json"
  jq -n \
    --arg job_id "control-plane-upgrade-0001" \
    --arg id "$CONTAINER_ID" \
    --arg version "$TARGET_VERSION" \
    --arg image "$TARGET_IMAGE" \
    '{degraded: false, active_container_id: $id, active_container: "sub2api-green", active_version: $version, active_image: $image, job: {id: $job_id, status: "succeeded"}}' \
    > "$ROOT/var/lib/sub2api-deployer/state.json"
  rm -f -- "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json.status"
}

run_helper() {
  PATH="$FAKE_BIN:$PATH" \
  FAKE_CONTAINER_ID="$CONTAINER_ID" \
  FAKE_TARGET_IMAGE="$TARGET_IMAGE" \
  FAKE_CANDIDATE_BINARY="$TEST_DIR/candidate" \
  FAKE_SYSTEMCTL_LOG="$TEST_DIR/systemctl.log" \
  FAKE_HEALTH="$1" \
    "$HELPER"
}

write_fixture previous-success
SUCCESS_HEALTH=$(jq -cn \
  --arg id "$CONTAINER_ID" \
  --arg version "$TARGET_VERSION" \
  '{status:"ok", degraded:false, job_running:false, active_container_id:$id, active_version:$version, control_plane_upgrade_ready:true}')
run_helper "$SUCCESS_HEALTH"
cmp -s "$TEST_DIR/candidate" "$ROOT/usr/local/sbin/sub2api-deployer"
[[ ! -e "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json" ]]
jq -e '.status == "succeeded" and .job_id == "control-plane-upgrade-0001"' \
  "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json.status" >/dev/null

write_fixture previous-failure
cp -a -- "$ROOT/usr/local/sbin/sub2api-deployer" "$TEST_DIR/expected-restored"
FAILURE_HEALTH=$(jq -cn \
  --arg id "$WRONG_CONTAINER_ID" \
  --arg version "$TARGET_VERSION" \
  '{status:"ok", degraded:false, job_running:false, active_container_id:$id, active_version:$version, control_plane_upgrade_ready:true}')
if run_helper "$FAILURE_HEALTH" >"$TEST_DIR/failure.log" 2>&1; then
  echo "identity-mismatched deployer health unexpectedly passed" >&2
  exit 1
fi
cmp -s "$TEST_DIR/expected-restored" "$ROOT/usr/local/sbin/sub2api-deployer"
[[ -f "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json" ]]
jq -e '.status == "failed" and (.error | contains("health verification"))' \
  "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json.status" >/dev/null

write_fixture missing-state
rm -f -- "$ROOT/var/lib/sub2api-deployer/state.json"
if run_helper "$SUCCESS_HEALTH" >"$TEST_DIR/missing-state.log" 2>&1; then
  echo "missing state unexpectedly passed" >&2
  exit 1
fi
jq -e '.status == "failed" and (.error | contains("state file is missing"))' \
  "$ROOT/var/lib/sub2api-deployer/control-plane-upgrade.json.status" >/dev/null

echo "sub2api deployer control-plane upgrade tests passed"
