#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
VALIDATOR="$REPO_ROOT/.github/scripts/validate-release-candidate.sh"
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-candidate.XXXXXX")
VERSION_FILE="$TEST_ROOT/VERSION"
NOTES_DIRECTORY="$TEST_ROOT/releases"

cleanup() {
  case "$TEST_ROOT" in
    *sub2api-release-candidate.*) find "$TEST_ROOT" -depth -delete ;;
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

write_valid_notes() {
  local notes_file="$1"
  cat > "$notes_file" <<'EOF'
# TokenSupply v1.2.3-ts.4

## Highlights

- Adds a production-ready release workflow with immutable, versioned notes.

## Fork Changes

- Keeps fork-specific deployment behavior traceable to one release candidate.

## Upstream Changes

- Baseline advances from the previous completed upstream release to version 1.2.3.

## Configuration And Migrations

- None. Existing configuration and database migrations remain compatible.

## Deployment And Rollback

- Deploy through the managed updater and retain the previous completed image digest for rollback.

## Verification

- Candidate validation, release safety fixtures, unit tests, integration tests, and frontend checks pass.

## Known Limitations

- None. Operational observation is still required after production deployment.
EOF
}

mkdir -p "$NOTES_DIRECTORY"

printf '%s\n' '1.2.3' > "$VERSION_FILE"
bash "$VALIDATOR" "$VERSION_FILE" "$NOTES_DIRECTORY" >/dev/null

printf '%s\n' '1.2.3-ts.0' > "$VERSION_FILE"
expect_failure 'Invalid release candidate version' \
  bash "$VALIDATOR" "$VERSION_FILE" "$NOTES_DIRECTORY"

printf '%s\n' '1.2.3-ts.4' > "$VERSION_FILE"
expect_failure 'Release notes are missing' \
  bash "$VALIDATOR" "$VERSION_FILE" "$NOTES_DIRECTORY"

NOTES_FILE="$NOTES_DIRECTORY/v1.2.3-ts.4.md"
write_valid_notes "$NOTES_FILE"
bash "$VALIDATOR" "$VERSION_FILE" "$NOTES_DIRECTORY" >/dev/null

printf '\nTODO: replace before release.\n' >> "$NOTES_FILE"
expect_failure 'template placeholders' \
  bash "$VALIDATOR" "$VERSION_FILE" "$NOTES_DIRECTORY"

echo "Release candidate validation tests passed"
