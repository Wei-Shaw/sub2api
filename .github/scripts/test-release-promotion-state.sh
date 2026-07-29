#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
STATE_SCRIPT="$REPO_ROOT/.github/scripts/release-promotion-state.sh"
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-promotion.XXXXXX")
ORIGIN="$TEST_ROOT/origin.git"
REPO="$TEST_ROOT/repo"

cleanup() {
  case "$TEST_ROOT" in
    *sub2api-release-promotion.*) find "$TEST_ROOT" -depth -delete ;;
    *) echo "Refusing to clean unexpected test path: $TEST_ROOT" >&2 ;;
  esac
}
trap cleanup EXIT

fail() {
  echo "$*" >&2
  exit 1
}

expect_failure() {
  local expected="$1"
  shift
  local output
  if output=$("$@" 2>&1); then
    fail "Command unexpectedly succeeded: $*"
  fi
  [[ "$output" == *"$expected"* ]] || \
    fail "Failure did not contain '$expected': $output"
}

git init -q --bare "$ORIGIN"
git init -q -b main "$REPO"
git -C "$REPO" config user.name "Promotion State Test"
git -C "$REPO" config user.email "promotion-state@example.invalid"
git -C "$REPO" remote add origin "$ORIGIN"
printf 'first\n' > "$REPO/state.txt"
git -C "$REPO" add state.txt
git -C "$REPO" commit -q -m first
FIRST=$(git -C "$REPO" rev-parse HEAD)
git -C "$REPO" push -q -u origin main
git -C "$REPO" fetch -q origin main

RESULT=$(cd "$REPO" && bash "$STATE_SCRIPT" inspect "$FIRST" 1.0.0-ts.1 refs/remotes/origin/main origin)
[[ "$RESULT" == "false" ]] || fail "Missing tag should require creation"

printf 'second\n' >> "$REPO/state.txt"
git -C "$REPO" commit -q -am second
SECOND=$(git -C "$REPO" rev-parse HEAD)
git -C "$REPO" push -q origin main
git -C "$REPO" fetch -q origin main
git -C "$REPO" checkout -q "$FIRST"
expect_failure 'moved to' bash -c 'cd "$1" && bash "$2" inspect "$3" 1.0.0-ts.1 refs/remotes/origin/main origin' \
  _ "$REPO" "$STATE_SCRIPT" "$FIRST"
expect_failure 'moved to' bash -c 'cd "$1" && bash "$2" require-current-main "$3" refs/remotes/origin/main' \
  _ "$REPO" "$STATE_SCRIPT" "$FIRST"

git -C "$REPO" tag -a v1.0.0-ts.1 -m release "$FIRST"
git -C "$REPO" push -q origin refs/tags/v1.0.0-ts.1
RESULT=$(cd "$REPO" && bash "$STATE_SCRIPT" inspect "$FIRST" 1.0.0-ts.1 refs/remotes/origin/main origin)
[[ "$RESULT" == "true" ]] || fail "Matching annotated tag should be recoverable after main advances"

git -C "$REPO" tag v1.0.0-ts.2 "$FIRST"
git -C "$REPO" push -q origin refs/tags/v1.0.0-ts.2
expect_failure 'not annotated' bash -c 'cd "$1" && bash "$2" inspect "$3" 1.0.0-ts.2 refs/remotes/origin/main origin' \
  _ "$REPO" "$STATE_SCRIPT" "$FIRST"

git -C "$REPO" tag -a v1.0.0-ts.3 -m wrong "$SECOND"
git -C "$REPO" push -q origin refs/tags/v1.0.0-ts.3
expect_failure 'points to' bash -c 'cd "$1" && bash "$2" inspect "$3" 1.0.0-ts.3 refs/remotes/origin/main origin' \
  _ "$REPO" "$STATE_SCRIPT" "$FIRST"

git -C "$REPO" checkout -q "$SECOND"
(cd "$REPO" && bash "$STATE_SCRIPT" require-current-main "$SECOND" refs/remotes/origin/main)
expect_failure '40-character' bash -c 'cd "$1" && bash "$2" inspect invalid 1.0.0-ts.4 refs/remotes/origin/main origin' \
  _ "$REPO" "$STATE_SCRIPT"
expect_failure 'X.Y.Z-ts.N' bash -c 'cd "$1" && bash "$2" inspect "$3" invalid refs/remotes/origin/main origin' \
  _ "$REPO" "$STATE_SCRIPT" "$SECOND"

git -C "$REPO" switch -q --orphan unrelated
printf 'unrelated\n' > "$REPO/unrelated.txt"
git -C "$REPO" add unrelated.txt
git -C "$REPO" commit -q -m unrelated
git -C "$REPO" push -q --force origin HEAD:main
git -C "$REPO" fetch -q origin main
git -C "$REPO" checkout -q "$FIRST"
expect_failure 'not an ancestor' bash -c 'cd "$1" && bash "$2" inspect "$3" 1.0.0-ts.1 refs/remotes/origin/main origin' \
  _ "$REPO" "$STATE_SCRIPT" "$FIRST"

echo "Release promotion state tests passed"
