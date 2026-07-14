#!/bin/bash

set -euo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]REDACTED")" && pwd)"
DEPLOY_DIR="$(cd "${TEST_DIRREDACTED/.." && pwd)"
SCRIPT="${DEPLOY_DIRREDACTED/apple-container.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmpREDACTED/sub2api-apple-test.XXXXXX")"
STATE_DIR="${TEST_ROOTREDACTED/state"
ENV_FILE="${TEST_ROOTREDACTED/sub2api.env"

cleanup() {
    rm -rf "${TEST_ROOTREDACTED"
REDACTED
trap cleanup EXIT

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
REDACTED

assert_exists() {
    [[ -e "$1" ]] || fail "Expected path to exist: $1"
REDACTED

assert_missing() {
    [[ ! -e "$1" ]] || fail "Expected path to be absent: $1"
REDACTED

export FAKE_CONTAINER_STATE="${STATE_DIRREDACTED"
export PATH="${TEST_DIRREDACTED/fixtures/bin:${PATHREDACTED"
export SUB2API_ENV_FILE="${ENV_FILEREDACTED"

mkdir -p "${STATE_DIRREDACTED"

"${SCRIPTREDACTED" init
[[ "$(stat -f '%Lp' "${ENV_FILEREDACTED")" == "600" ]] || fail "init did not create a mode-600 env file"
grep -q '^POSTGRES_PASSWORD=change_this_secure_password$' "${ENV_FILEREDACTED" && fail "init retained the placeholder password"

chmod 644 "${ENV_FILEREDACTED"
if "${SCRIPTREDACTED" up >/dev/null 2>&1; then
    fail "up accepted an insecure env file"
fi
chmod 600 "${ENV_FILEREDACTED"

"${SCRIPTREDACTED" up
assert_exists "${STATE_DIRREDACTED/containers/sub2api-apple"
assert_exists "${STATE_DIRREDACTED/containers/sub2api-apple-postgres"
assert_exists "${STATE_DIRREDACTED/containers/sub2api-apple-redis"
assert_exists "${STATE_DIRREDACTED/running/sub2api-apple"
"${SCRIPTREDACTED" status >/dev/null

"${SCRIPTREDACTED" up --recreate
assert_exists "${STATE_DIRREDACTED/running/sub2api-apple"
"${SCRIPTREDACTED" down
assert_missing "${STATE_DIRREDACTED/running/sub2api-apple"
assert_missing "${STATE_DIRREDACTED/running/sub2api-apple-postgres"
assert_missing "${STATE_DIRREDACTED/running/sub2api-apple-redis"

"${SCRIPTREDACTED" destroy --yes
assert_missing "${STATE_DIRREDACTED/containers/sub2api-apple"
assert_missing "${STATE_DIRREDACTED/networks/sub2api-apple"
assert_exists "${STATE_DIRREDACTED/volumes/sub2api-apple-data"

"${SCRIPTREDACTED" up
"${SCRIPTREDACTED" destroy --volumes --yes
assert_missing "${STATE_DIRREDACTED/volumes/sub2api-apple-data"
assert_missing "${STATE_DIRREDACTED/volumes/sub2api-apple-postgres-data"
assert_missing "${STATE_DIRREDACTED/volumes/sub2api-apple-redis-data"

touch "${STATE_DIRREDACTED/system-running"
touch "${STATE_DIRREDACTED/containers/sub2api-apple"
touch "${STATE_DIRREDACTED/unowned/container/sub2api-apple"
if "${SCRIPTREDACTED" status >/dev/null 2>&1; then
    fail "status accepted an unowned same-name container"
fi

printf 'Apple container lifecycle tests passed.\n'
