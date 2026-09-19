#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
RESOLVER="$SCRIPT_DIR/resolve-version.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

make_tree() {
  mkdir -p "$1/backend/cmd/server" "$1/backend/scripts"
  cp "$RESOLVER" "$1/backend/scripts/resolve-version.sh"
  chmod +x "$1/backend/scripts/resolve-version.sh"
  printf '%s\n' '9.9.9' > "$1/backend/cmd/server/VERSION"
}

assert_version() {
  EXPECTED="$1"
  ROOT="$2"
  ACTUAL="$(cd "$ROOT/backend" && ./scripts/resolve-version.sh)"
  if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "expected $EXPECTED, got $ACTUAL" >&2
    exit 1
  fi
}

# A source archive carries the exact tag through export-subst and has no .git.
ARCHIVE_TREE="$TMP_DIR/archive"
make_tree "$ARCHIVE_TREE"
printf '%s\n' 'v1.2.3' > "$ARCHIVE_TREE/backend/cmd/server/VERSION_TAG"
assert_version '1.2.3' "$ARCHIVE_TREE"

# Unexpanded markers and decorated non-tag descriptions must fall back safely.
FALLBACK_TREE="$TMP_DIR/fallback"
make_tree "$FALLBACK_TREE"
printf '%s\n' '$Format:%(describe:tags)$' > "$FALLBACK_TREE/backend/cmd/server/VERSION_TAG"
assert_version '9.9.9' "$FALLBACK_TREE"
printf '%s\n' 'v1.2.3-1-gdeadbeef' > "$FALLBACK_TREE/backend/cmd/server/VERSION_TAG"
assert_version '9.9.9' "$FALLBACK_TREE"

# A tagged checkout still takes precedence over the tracked VERSION file.
TAGGED_TREE="$TMP_DIR/tagged"
make_tree "$TAGGED_TREE"
printf '%s\n' '$Format:%(describe:tags)$' > "$TAGGED_TREE/backend/cmd/server/VERSION_TAG"
(cd "$TAGGED_TREE" && git init -q && git config user.email test@example.com && git config user.name test && git add . && git commit -qm initial && git tag v2.3.4)
assert_version '2.3.4' "$TAGGED_TREE"

echo 'resolve-version.sh tests passed'
