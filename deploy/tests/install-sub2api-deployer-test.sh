#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
INSTALLER_SOURCE="$REPO_ROOT/deploy/install-sub2api-deployer.sh"
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-installer-test.XXXXXX")
cleanup() {
  if [[ "${KEEP_TEST_DIR:-0}" == 1 ]]; then
    echo "preserved installer test directory: $TEST_DIR" >&2
  else
    rm -rf -- "$TEST_DIR"
  fi
}
trap cleanup EXIT

FAKE_BIN="$TEST_DIR/fake-bin"
mkdir -p "$FAKE_BIN"

cat > "$FAKE_BIN/id" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == "-u" ]] && { echo 0; exit 0; }
exec /usr/bin/id "$@"
EOF

cat > "$FAKE_BIN/stat" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-c" ]]; then
  case "${2:-}" in
    %u) echo 0 ;;
    %u:%a) echo 0:600 ;;
    %a) echo 755 ;;
    %u:%g) echo 0:0 ;;
    %i) echo 424242 ;;
    *) echo "unsupported fake stat format: ${2:-}" >&2; exit 1 ;;
  esac
  exit 0
fi
echo "unsupported fake stat invocation" >&2
exit 1
EOF

cat > "$FAKE_BIN/chown" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$FAKE_BIN/getent" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "group" && "${2:-}" == "sub2api-deployer" ]] || exit 2
[[ -f "$FAKE_CONTROL_DIR/socket-group" ]] || exit 2
printf '%s\n' 'sub2api-deployer:x:987:'
EOF

cat > "$FAKE_BIN/groupadd" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "--system" && "${2:-}" == "sub2api-deployer" ]]
: > "$FAKE_CONTROL_DIR/socket-group"
EOF

cat > "$FAKE_BIN/groupdel" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "sub2api-deployer" ]]
rm -f -- "$FAKE_CONTROL_DIR/socket-group"
EOF

cat > "$FAKE_BIN/sleep" <<'EOF'
#!/usr/bin/env bash
# State transitions are controlled by the fake systemd/curl implementations.
exit 0
EOF

cat > "$FAKE_BIN/ss" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat > "$FAKE_BIN/flock" <<'EOF'
#!/usr/bin/env bash
if [[ "${FAKE_FAIL_ROLLBACK_STATE_LOCK:-0}" == 1 && -f "$FAKE_CONTROL_DIR/restart-failed" ]]; then
  exit 1
fi
exit 0
EOF

cat > "$FAKE_BIN/systemctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_SYSTEMCTL_LOG"
command_name="${1:-}"
case "$command_name" in
  is-enabled)
    [[ -f "$FAKE_CONTROL_DIR/enabled" ]]
    ;;
  is-active)
    [[ -f "$FAKE_CONTROL_DIR/active" ]]
    ;;
  show)
    if [[ " $* " == *" --property=ActiveState "* ]]; then
      if [[ -f "$FAKE_CONTROL_DIR/active" ]]; then echo active; else echo inactive; fi
    elif [[ " $* " == *" --property=MainPID "* ]]; then
      if [[ "${FAKE_NONZERO_MAINPID_AFTER_RESTART:-0}" == 1 && -f "$FAKE_CONTROL_DIR/restart-failed" ]]; then
        echo 4242
      elif [[ -f "$FAKE_CONTROL_DIR/active" ]]; then
        echo 4242
      else
        echo 0
      fi
    else
      echo "unsupported fake systemctl show invocation: $*" >&2
      exit 1
    fi
    ;;
  enable)
    : > "$FAKE_CONTROL_DIR/enabled"
    ;;
  disable)
    rm -f -- "$FAKE_CONTROL_DIR/enabled"
    ;;
  stop)
    if [[ "${FAKE_FAIL_ROLLBACK_STOP:-0}" == 1 && -f "$FAKE_CONTROL_DIR/restart-failed" ]]; then
      exit 1
    fi
    rm -f -- "$FAKE_CONTROL_DIR/active"
    ;;
  start)
    : > "$FAKE_CONTROL_DIR/active"
    ;;
  restart)
    if [[ "${FAKE_FAIL_RESTART:-0}" == 1 ]]; then
      : > "$FAKE_CONTROL_DIR/restart-failed"
      exit 1
    fi
    : > "$FAKE_CONTROL_DIR/active"
    ;;
  reload)
    if [[ "${2:-}" == nginx && "${FAKE_FAIL_NGINX_RELOAD_ONCE:-0}" == 1 && ! -f "$FAKE_CONTROL_DIR/nginx-reload-failed" ]]; then
      : > "$FAKE_CONTROL_DIR/nginx-reload-failed"
      exit 1
    fi
    ;;
  daemon-reload)
    ;;
  *)
    echo "unsupported fake systemctl invocation: $*" >&2
    exit 1
    ;;
esac
EOF

cat > "$FAKE_BIN/nginx" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  -t)
    exit 0
    ;;
  -T)
    cat -- "$FAKE_NGINX_SITE"
    [[ -f "$FAKE_MANAGED_UPSTREAM" ]] && cat -- "$FAKE_MANAGED_UPSTREAM"
    ;;
  *)
    echo "unsupported fake nginx invocation: $*" >&2
    exit 1
    ;;
esac
EOF

cat > "$FAKE_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ " $* " == *" --unix-socket "* ]]; then
  if [[ -f "$FAKE_CONTROL_DIR/active" || ( "${FAKE_SOCKET_LIVE_AFTER_RESTART:-0}" == 1 && -f "$FAKE_CONTROL_DIR/restart-failed" ) ]]; then
    printf '%s\n' "$FAKE_DEPLOYER_HEALTH"
  else
    exit 7
  fi
else
  printf '%s\n' '{"status":"ok"}'
fi
EOF

cat > "$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
case "${1:-}" in
  inspect)
    format="${4:-}"
    case "$format" in
      '{{.Id}}') printf '%s\n' "$FAKE_CONTAINER_ID" ;;
      '{{json .Config.Labels}}')
        printf '%s\n' '{"com.docker.compose.project":"sub2api","com.docker.compose.service":"sub2api"}'
        ;;
      '{{.Config.Image}}') printf '%s\n' "$FAKE_CURRENT_IMAGE" ;;
      *) echo "unsupported fake docker inspect: $*" >&2; exit 1 ;;
    esac
    ;;
  port)
    printf '%s\n' "$FAKE_DOCKER_PORT_OUTPUT"
    ;;
  exec)
    if [[ " $* " == *" /app/sub2api --version "* ]]; then
      printf 'Sub2API %s (commit: test)\n' "$FAKE_CURRENT_VERSION"
    elif [[ " $* " == *" stat -c %i /run/sub2api-deployer "* ]]; then
      printf '%s\n' 424242
    elif [[ " $* " == *" /proc/1/status "* ]]; then
      printf '%s\n' 1000
    elif [[ " $* " == *" test -S /run/sub2api-deployer/deployer.sock "* ]]; then
      [[ "${FAKE_CONTAINER_SOCKET_UNAVAILABLE:-0}" != 1 ]]
    else
      echo "unsupported fake docker exec: $*" >&2
      exit 1
    fi
    ;;
  compose)
    if [[ " $* " == *" config --quiet "* ]]; then
      exit 0
    fi
    if [[ " $* " == *" config --format json "* ]]; then
      jq -n \
        --arg image "$SUB2API_IMAGE" \
        --arg gid "$SUB2API_DEPLOYER_SOCKET_GID" '
          {
            services: {
              sub2api: {
                image: $image,
                group_add: [$gid],
                volumes: [
                  {type: "bind", source: "/run/sub2api-deployer", target: "/run/sub2api-deployer", read_only: true},
                  {type: "bind", source: "/var/lib/sub2api-deployer/runtime", target: "/run/sub2api-deployment", read_only: true}
                ],
                environment: {
                  UPDATE_MODE: "docker-managed",
                  UPDATE_DEPLOYER_SOCKET: "/run/sub2api-deployer/deployer.sock",
                  SERVER_SHUTDOWN_TIMEOUT: "90"
                },
                healthcheck: {test: ["CMD", "true"]}
              }
            }
          }
        '
      exit 0
    fi
    echo "unsupported fake docker compose invocation: $*" >&2
    exit 1
    ;;
  *)
    echo "unsupported fake docker invocation: $*" >&2
    exit 1
    ;;
esac
EOF

chmod +x "$FAKE_BIN"/*

CONTAINER_ID=$(printf 'a%.0s' {1..64})
CURRENT_IMAGE="ghcr.io/ssharkkky/sub2api:0.1.1-ts.1"
CURRENT_VERSION="0.1.1-ts.1"
CURRENT_PORT=18080

make_root() {
  local root="$1"
  mkdir -p \
    "$root/app" \
    "$root/control" \
    "$root/etc/nginx/conf.d" \
    "$root/etc/systemd/system" \
    "$root/etc/tmpfiles.d" \
    "$root/home/protected" \
    "$root/nginx" \
    "$root/run" \
    "$root/usr/local/sbin"
  cat > "$root/app/compose.yaml" <<EOF
services:
  sub2api:
    image: $CURRENT_IMAGE
EOF
  : > "$root/app/.env"
  cat > "$root/nginx/site.conf" <<EOF
location / {
    proxy_pass http://127.0.0.1:$CURRENT_PORT;
}
EOF
  sed \
    -e "s#^CONFIG_DIR=.*#CONFIG_DIR=\"$root/etc/sub2api-deployer\"#" \
    -e "s#^SERVICE_FILE=.*#SERVICE_FILE=\"$root/etc/systemd/system/sub2api-deployer.service\"#" \
    -e "s#^TMPFILES_FILE=.*#TMPFILES_FILE=\"$root/etc/tmpfiles.d/sub2api-deployer.conf\"#" \
    -e "s#^RUNTIME_PRESERVE_DROPIN=.*#RUNTIME_PRESERVE_DROPIN=\"$root/run/systemd/system/sub2api-deployer.service.d/10-preserve-runtime.conf\"#" \
    -e "s#^INSTALLED_BINARY=.*#INSTALLED_BINARY=\"$root/usr/local/sbin/sub2api-deployer\"#" \
    -e "s#^STATE_DIR=.*#STATE_DIR=\"$root/var/lib/sub2api-deployer\"#" \
    -e "s#^NGINX_LOADER_FILE=.*#NGINX_LOADER_FILE=\"$root/etc/nginx/conf.d/sub2api-managed-upstream.conf\"#" \
    -e "s#^INSTALL_LOCK_FILE=.*#INSTALL_LOCK_FILE=\"$root/run/sub2api-deployer-install.lock\"#" \
    -e "s#^SOCKET_DIRECTORY=.*#SOCKET_DIRECTORY=\"$root/run/sub2api-deployer\"#" \
    "$INSTALLER_SOURCE" > "$root/installer.sh"
  chmod +x "$root/installer.sh"
}

make_deployer_binary() {
  local path="$1"
  local marker="$2"
  cat > "$path" <<EOF
#!/usr/bin/env bash
# $marker
exit 0
EOF
  chmod +x "$path"
  (cd -- "$(dirname -- "$path")" && sha256sum "$(basename -- "$path")" > "$(basename -- "$path").sha256")
}

write_active_state() {
  local root="$1"
  mkdir -p "$root/var/lib/sub2api-deployer"
  jq -n \
    --arg id "$CONTAINER_ID" \
    --arg image "$CURRENT_IMAGE" \
    --arg version "$CURRENT_VERSION" \
    --argjson port "$CURRENT_PORT" '
      {
        active_slot: "sub2api-blue",
        active_container: "sub2api",
        active_container_id: $id,
        active_port: $port,
        active_version: $version,
        active_image: $image,
        degraded: false
      }
    ' > "$root/var/lib/sub2api-deployer/state.json"
}

run_installer() {
  local root="$1"
  local binary="$2"
  shift 2
  PATH="$FAKE_BIN:$PATH" \
  FAKE_CONTAINER_ID="$CONTAINER_ID" \
  FAKE_CURRENT_IMAGE="$CURRENT_IMAGE" \
  FAKE_CURRENT_VERSION="$CURRENT_VERSION" \
  FAKE_DOCKER_PORT_OUTPUT="127.0.0.1:$CURRENT_PORT" \
  FAKE_DEPLOYER_HEALTH="{\"status\":\"ok\",\"degraded\":false,\"job_running\":false,\"active_container\":\"sub2api\",\"active_port\":$CURRENT_PORT,\"active_version\":\"$CURRENT_VERSION\"}" \
  FAKE_CONTROL_DIR="$root/control" \
  FAKE_DOCKER_LOG="$root/docker.log" \
  FAKE_SYSTEMCTL_LOG="$root/systemctl.log" \
  FAKE_NGINX_SITE="$root/nginx/site.conf" \
  FAKE_MANAGED_UPSTREAM="$root/var/lib/sub2api-deployer/nginx/managed-upstream.conf" \
    "$root/installer.sh" \
      --source "$REPO_ROOT" \
      --install-dir "$root/app" \
      --nginx-site "$root/nginx/site.conf" \
      --nginx-probe-url http://127.0.0.1/health \
      --nginx-probe-host tokensupply.test \
      --deployer-binary "$binary" \
      --deployer-checksums "$binary.sha256" \
      "$@"
}

assert_no_application_mutation() {
  local log_file="$1"
  if grep -Eq '(^| )(stop|rm|kill|update|run|up)( |$)' "$log_file"; then
    echo "installer mutated an application container" >&2
    cat "$log_file" >&2
    exit 1
  fi
}

# A first-install failure after Nginx files are replaced must restore the legacy
# route and remove every newly installed deployer asset.
FIRST_ROOT="$TEST_DIR/first-failure"
make_root "$FIRST_ROOT"
make_deployer_binary "$FIRST_ROOT/deployer-v1" v1
cp -- "$FIRST_ROOT/nginx/site.conf" "$FIRST_ROOT/original-site.conf"
if FAKE_FAIL_NGINX_RELOAD_ONCE=1 run_installer "$FIRST_ROOT" "$FIRST_ROOT/deployer-v1" >"$FIRST_ROOT/output.log" 2>&1; then
  echo "first-install Nginx reload failure unexpectedly succeeded" >&2
  exit 1
fi
cmp -s "$FIRST_ROOT/original-site.conf" "$FIRST_ROOT/nginx/site.conf"
[[ ! -e "$FIRST_ROOT/etc/sub2api-deployer/config.json" ]]
[[ ! -e "$FIRST_ROOT/usr/local/sbin/sub2api-deployer" ]]
[[ ! -e "$FIRST_ROOT/app/compose.deployer.yml" ]]
if ! grep -Fq 'Installation failed; restoring' "$FIRST_ROOT/output.log"; then
  cat "$FIRST_ROOT/output.log" >&2
  echo "first-install failure did not enter transactional rollback" >&2
  exit 1
fi
assert_no_application_mutation "$FIRST_ROOT/docker.log"

# Complete a real first install in the hermetic host, then prepare the state a
# running deployer would have persisted.
UPGRADE_ROOT="$TEST_DIR/upgrade-failure"
make_root "$UPGRADE_ROOT"
make_deployer_binary "$UPGRADE_ROOT/deployer-v1" v1
run_installer "$UPGRADE_ROOT" "$UPGRADE_ROOT/deployer-v1" >"$UPGRADE_ROOT/install.log" 2>&1
NORMALIZED_UPGRADE_APP=$(readlink -f "$UPGRADE_ROOT/app")
if ! jq -e \
  --arg work "$NORMALIZED_UPGRADE_APP" \
  --arg state "$UPGRADE_ROOT/var/lib/sub2api-deployer/image.env" '
    .compose_files == [$work + "/compose.yaml", $work + "/compose.deployer.yml"]
    and .compose_env_files == [$work + "/.env", $state]
    and .compose_service == "sub2api"
    and .image_repository == "ghcr.io/ssharkkky/sub2api"
    and .socket_gid == 987
  ' "$UPGRADE_ROOT/etc/sub2api-deployer/config.json" >/dev/null; then
  jq . "$UPGRADE_ROOT/etc/sub2api-deployer/config.json" >&2
  echo "first install did not persist the normalized Compose contract" >&2
  exit 1
fi
[[ -f "$UPGRADE_ROOT/control/socket-group" ]]
mkdir -p "$UPGRADE_ROOT/var/lib/sub2api-deployer"
jq -n \
  --arg id "$CONTAINER_ID" \
  --arg image "$CURRENT_IMAGE" \
  --arg version "$CURRENT_VERSION" \
  --argjson port "$CURRENT_PORT" '
    {
      active_slot: "sub2api-blue",
      active_container: "sub2api",
      active_container_id: $id,
      active_port: $port,
      active_version: $version,
      active_image: $image,
      degraded: false
    }
  ' > "$UPGRADE_ROOT/var/lib/sub2api-deployer/state.json"

cp -a -- "$UPGRADE_ROOT/etc/sub2api-deployer/config.json" "$UPGRADE_ROOT/original-config.json"
cp -a -- "$UPGRADE_ROOT/var/lib/sub2api-deployer/state.json" "$UPGRADE_ROOT/original-state.json"
cp -a -- "$UPGRADE_ROOT/app/compose.deployer.yml" "$UPGRADE_ROOT/original-compose.deployer.yml"
cp -a -- "$UPGRADE_ROOT/usr/local/sbin/sub2api-deployer" "$UPGRADE_ROOT/original-deployer"
make_deployer_binary "$UPGRADE_ROOT/deployer-v2" v2

if FAKE_FAIL_RESTART=1 run_installer "$UPGRADE_ROOT" "$UPGRADE_ROOT/deployer-v2" >"$UPGRADE_ROOT/upgrade.log" 2>&1; then
  echo "upgrade restart failure unexpectedly succeeded" >&2
  exit 1
fi
cmp -s "$UPGRADE_ROOT/original-config.json" "$UPGRADE_ROOT/etc/sub2api-deployer/config.json"
cmp -s "$UPGRADE_ROOT/original-state.json" "$UPGRADE_ROOT/var/lib/sub2api-deployer/state.json"
cmp -s "$UPGRADE_ROOT/original-compose.deployer.yml" "$UPGRADE_ROOT/app/compose.deployer.yml"
cmp -s "$UPGRADE_ROOT/original-deployer" "$UPGRADE_ROOT/usr/local/sbin/sub2api-deployer"
[[ -f "$UPGRADE_ROOT/control/active" ]]
[[ -f "$UPGRADE_ROOT/control/enabled" ]]
grep -Fq 'restart sub2api-deployer.service' "$UPGRADE_ROOT/systemctl.log"
grep -Fq 'start sub2api-deployer.service' "$UPGRADE_ROOT/systemctl.log"
grep -Fq -- "--env-file $UPGRADE_ROOT/var/lib/sub2api-deployer/image.env" "$UPGRADE_ROOT/docker.log"
grep -Fq -- "-f $NORMALIZED_UPGRADE_APP/compose.deployer.yml" "$UPGRADE_ROOT/docker.log"
grep -Fq "port $CONTAINER_ID 8080/tcp" "$UPGRADE_ROOT/docker.log"
grep -Fq "exec $CONTAINER_ID /app/sub2api --version" "$UPGRADE_ROOT/docker.log"
assert_no_application_mutation "$UPGRADE_ROOT/docker.log"

# A supplied private-registry credential is copied into the hardened service's
# dedicated root-only Docker config directory and survives an ordinary upgrade.
AUTH_ROOT="$TEST_DIR/docker-auth"
make_root "$AUTH_ROOT"
make_deployer_binary "$AUTH_ROOT/deployer-v1" v1
printf '%s\n' '{"auths":{"ghcr.io":{"auth":"test-token"}}}' > "$AUTH_ROOT/source-docker-config.json"
run_installer "$AUTH_ROOT" "$AUTH_ROOT/deployer-v1" --docker-config "$AUTH_ROOT/source-docker-config.json" >"$AUTH_ROOT/install.log" 2>&1
write_active_state "$AUTH_ROOT"
jq -e '.auths["ghcr.io"].auth == "test-token"' "$AUTH_ROOT/etc/sub2api-deployer/docker/config.json" >/dev/null
grep -Fq 'Environment=DOCKER_CONFIG=/etc/sub2api-deployer/docker' "$AUTH_ROOT/etc/systemd/system/sub2api-deployer.service"
make_deployer_binary "$AUTH_ROOT/deployer-v2" v2
run_installer "$AUTH_ROOT" "$AUTH_ROOT/deployer-v2" >"$AUTH_ROOT/upgrade.log" 2>&1
jq -e '.auths["ghcr.io"].auth == "test-token"' "$AUTH_ROOT/etc/sub2api-deployer/docker/config.json" >/dev/null

# An upgrade is not committed unless the existing application user can still
# access the socket through its original bind mount after the deployer restart.
SOCKET_ROOT="$TEST_DIR/container-socket-failure"
make_root "$SOCKET_ROOT"
make_deployer_binary "$SOCKET_ROOT/deployer-v1" v1
run_installer "$SOCKET_ROOT" "$SOCKET_ROOT/deployer-v1" >"$SOCKET_ROOT/install.log" 2>&1
write_active_state "$SOCKET_ROOT"
cp -a -- "$SOCKET_ROOT/usr/local/sbin/sub2api-deployer" "$SOCKET_ROOT/original-deployer"
make_deployer_binary "$SOCKET_ROOT/deployer-v2" v2
if FAKE_CONTAINER_SOCKET_UNAVAILABLE=1 run_installer "$SOCKET_ROOT" "$SOCKET_ROOT/deployer-v2" >"$SOCKET_ROOT/upgrade.log" 2>&1; then
  echo "container-side socket verification failure unexpectedly succeeded" >&2
  exit 1
fi
grep -Fq 'running application user cannot access' "$SOCKET_ROOT/upgrade.log"
cmp -s "$SOCKET_ROOT/original-deployer" "$SOCKET_ROOT/usr/local/sbin/sub2api-deployer"
[[ -f "$SOCKET_ROOT/control/active" ]]

# ProtectHome hides Compose projects under these paths from the daemon. The
# installer rejects the layout before mutating any managed asset.
PROTECTED_ROOT="$TEST_DIR/protected-path"
make_root "$PROTECTED_ROOT"
make_deployer_binary "$PROTECTED_ROOT/deployer-v1" v1
mkdir -p "$PROTECTED_ROOT/root/app"
cp -a "$PROTECTED_ROOT/app/." "$PROTECTED_ROOT/root/app/"
if run_installer "$PROTECTED_ROOT" "$PROTECTED_ROOT/deployer-v1" --install-dir /root/protected >"$PROTECTED_ROOT/output.log" 2>&1; then
  echo "protected Compose path unexpectedly succeeded" >&2
  exit 1
fi
grep -Fq 'must not be under /root, /home, or /run/user' "$PROTECTED_ROOT/output.log"

# If rollback cannot prove exclusive ownership, it must leave the newly written
# files in place and preserve recovery backups rather than racing a live daemon.
for proof in stop mainpid socket lock; do
  PROOF_ROOT="$TEST_DIR/rollback-proof-$proof"
  make_root "$PROOF_ROOT"
  make_deployer_binary "$PROOF_ROOT/deployer-v1" v1
  run_installer "$PROOF_ROOT" "$PROOF_ROOT/deployer-v1" >"$PROOF_ROOT/install.log" 2>&1
  write_active_state "$PROOF_ROOT"
  make_deployer_binary "$PROOF_ROOT/deployer-v2" v2
  status=0
  case "$proof" in
    stop) FAKE_FAIL_ROLLBACK_STOP=1 FAKE_FAIL_RESTART=1 run_installer "$PROOF_ROOT" "$PROOF_ROOT/deployer-v2" >"$PROOF_ROOT/upgrade.log" 2>&1 || status=$? ;;
    mainpid) FAKE_NONZERO_MAINPID_AFTER_RESTART=1 FAKE_FAIL_RESTART=1 run_installer "$PROOF_ROOT" "$PROOF_ROOT/deployer-v2" >"$PROOF_ROOT/upgrade.log" 2>&1 || status=$? ;;
    socket) FAKE_SOCKET_LIVE_AFTER_RESTART=1 FAKE_FAIL_RESTART=1 run_installer "$PROOF_ROOT" "$PROOF_ROOT/deployer-v2" >"$PROOF_ROOT/upgrade.log" 2>&1 || status=$? ;;
    lock) FAKE_FAIL_ROLLBACK_STATE_LOCK=1 FAKE_FAIL_RESTART=1 run_installer "$PROOF_ROOT" "$PROOF_ROOT/deployer-v2" >"$PROOF_ROOT/upgrade.log" 2>&1 || status=$? ;;
  esac
  if (( status == 0 )); then
    echo "rollback proof failure $proof unexpectedly succeeded" >&2
    exit 1
  fi
  grep -Fq '# v2' "$PROOF_ROOT/usr/local/sbin/sub2api-deployer"
  grep -Fq 'automatic file restoration was not attempted' "$PROOF_ROOT/upgrade.log"
  grep -Fq 'Recovery backups were preserved at ' "$PROOF_ROOT/upgrade.log"
done

echo "sub2api deployer installer transaction tests passed"
