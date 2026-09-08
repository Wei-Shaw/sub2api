#!/bin/bash

set -euo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${TEST_DIR}/.." && pwd)"
SCRIPT="${DEPLOY_DIR}/apple-container.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/sub2api-apple-test.XXXXXX")"
STATE_DIR="${TEST_ROOT}/state"
ENV_FILE="${TEST_ROOT}/sub2api.env"

cleanup() {
    rm -rf "${TEST_ROOT}"
}
trap cleanup EXIT

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

assert_exists() {
    [[ -e "$1" ]] || fail "Expected path to exist: $1"
}

assert_missing() {
    [[ ! -e "$1" ]] || fail "Expected path to be absent: $1"
}

export FAKE_CONTAINER_STATE="${STATE_DIR}"
export PATH="${TEST_DIR}/fixtures/bin:${PATH}"
export SUB2API_ENV_FILE="${ENV_FILE}"

mkdir -p "${STATE_DIR}"

"${SCRIPT}" init
[[ "$(stat -f '%Lp' "${ENV_FILE}")" == "600" ]] || fail "init did not create a mode-600 env file"
grep -q '^POSTGRES_PASSWORD=change_this_secure_password$' "${ENV_FILE}" && fail "init retained the placeholder password"
cat >>"${ENV_FILE}" <<'EOF'
REDIS_SAVE_SECONDS=900
REDIS_SAVE_CHANGES=2
REDIS_LATENCY_MONITOR_THRESHOLD_MS=100
EOF

chmod 644 "${ENV_FILE}"
if "${SCRIPT}" up >/dev/null 2>&1; then
    fail "up accepted an insecure env file"
fi
chmod 600 "${ENV_FILE}"

"${SCRIPT}" up
assert_exists "${STATE_DIR}/containers/sub2api-apple"
assert_exists "${STATE_DIR}/containers/sub2api-apple-postgres"
assert_exists "${STATE_DIR}/containers/sub2api-apple-redis"
assert_exists "${STATE_DIR}/running/sub2api-apple"
grep -q '^REDIS_SAVE_SECONDS=900$' "${STATE_DIR}/env-files/sub2api-apple-redis" || fail "Redis save interval was not propagated"
grep -q '^REDIS_SAVE_CHANGES=2$' "${STATE_DIR}/env-files/sub2api-apple-redis" || fail "Redis save change threshold was not propagated"
grep -q '^REDIS_LATENCY_MONITOR_THRESHOLD_MS=100$' "${STATE_DIR}/env-files/sub2api-apple-redis" || fail "Redis latency threshold was not propagated"
grep -Fq '${REDIS_SAVE_SECONDS:-60}' "${STATE_DIR}/create-args/sub2api-apple-redis" || fail "Redis save interval is missing from the runtime command"
grep -Fq '${REDIS_LATENCY_MONITOR_THRESHOLD_MS:-0}' "${STATE_DIR}/create-args/sub2api-apple-redis" || fail "Redis latency threshold is missing from the runtime command"
"${SCRIPT}" status >/dev/null

"${SCRIPT}" up --recreate
assert_exists "${STATE_DIR}/running/sub2api-apple"
"${SCRIPT}" down
assert_missing "${STATE_DIR}/running/sub2api-apple"
assert_missing "${STATE_DIR}/running/sub2api-apple-postgres"
assert_missing "${STATE_DIR}/running/sub2api-apple-redis"

"${SCRIPT}" destroy --yes
assert_missing "${STATE_DIR}/containers/sub2api-apple"
assert_missing "${STATE_DIR}/networks/sub2api-apple"
assert_exists "${STATE_DIR}/volumes/sub2api-apple-data"

"${SCRIPT}" up
"${SCRIPT}" destroy --volumes --yes
assert_missing "${STATE_DIR}/volumes/sub2api-apple-data"
assert_missing "${STATE_DIR}/volumes/sub2api-apple-postgres-data"
assert_missing "${STATE_DIR}/volumes/sub2api-apple-redis-data"

touch "${STATE_DIR}/system-running"
touch "${STATE_DIR}/containers/sub2api-apple"
touch "${STATE_DIR}/unowned/container/sub2api-apple"
if "${SCRIPT}" status >/dev/null 2>&1; then
    fail "status accepted an unowned same-name container"
fi

printf 'Apple container lifecycle tests passed.\n'
