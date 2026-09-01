#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
BACKEND_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
REPO_DIR="$(CDPATH= cd -- "$BACKEND_DIR/.." && pwd)"
VERSION_FILE="$BACKEND_DIR/cmd/server/VERSION"
ARCHIVE_VERSION_FILE="$BACKEND_DIR/cmd/server/VERSION_TAG"

normalize_version() {
  VALUE="$(printf '%s' "$1" | tr -d '\r\n')"
  case "$VALUE" in
    ''|'$Format:'*|*-[0-9]*-g[0-9a-f]*)
      return 1
      ;;
  esac

  if printf '%s' "$VALUE" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'; then
    printf '%s\n' "${VALUE#v}"
    return 0
  fi

  return 1
}

# GitHub's source archives are created with `git archive` semantics. The
# export-subst marker carries the exact tag even though those archives omit
# the .git directory needed by `git describe`.
if [ -f "$ARCHIVE_VERSION_FILE" ]; then
  ARCHIVE_VERSION="$(cat "$ARCHIVE_VERSION_FILE")"
  if normalize_version "$ARCHIVE_VERSION"; then
    exit 0
  fi
fi

# Prefer the exact release tag when building from a tagged checkout so
# source builds from vX.Y.Z don't inherit the previous VERSION file value.
if command -v git >/dev/null 2>&1; then
  TAG="$(
    git -C "$REPO_DIR" describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null || \
    git -C "$REPO_DIR" describe --tags --exact-match --match '[0-9]*' 2>/dev/null || \
    true
  )"
  if [ -n "$TAG" ]; then
    if normalize_version "$TAG"; then
      exit 0
    fi
  fi
fi

normalize_version "$(cat "$VERSION_FILE")" || {
  # Preserve the historical fallback for development checkouts that use a
  # non-semver local VERSION value.
  tr -d '\r\n' < "$VERSION_FILE"
}
