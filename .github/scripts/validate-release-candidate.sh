#!/usr/bin/env bash

set -euo pipefail

VERSION_FILE=${1:-backend/cmd/server/VERSION}
NOTES_DIRECTORY=${2:-docs/releases}

fail() {
  echo "$*" >&2
  exit 1
}

[[ -f "$VERSION_FILE" ]] || fail "VERSION file is missing: $VERSION_FILE"
VERSION=$(tr -d '\r\n' < "$VERSION_FILE")

if [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Upstream baseline $VERSION does not require fork release notes"
  exit 0
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+-ts\.([1-9][0-9]*)$ ]]; then
  fail "Invalid release candidate version: $VERSION (expected X.Y.Z or X.Y.Z-ts.N with N >= 1)"
fi

RELEASE_TAG="v${VERSION}"
NOTES_FILE="${NOTES_DIRECTORY%/}/${RELEASE_TAG}.md"
[[ -f "$NOTES_FILE" ]] || fail "Release notes are missing for $RELEASE_TAG: $NOTES_FILE"

FIRST_LINE=$(sed -n '1p' "$NOTES_FILE")
[[ "$FIRST_LINE" == "# TokenSupply $RELEASE_TAG" ]] || \
  fail "Release notes must start with: # TokenSupply $RELEASE_TAG"

required_headings=(
  '## Highlights'
  '## Fork Changes'
  '## Upstream Changes'
  '## Configuration And Migrations'
  '## Deployment And Rollback'
  '## Verification'
  '## Known Limitations'
)

for heading in "${required_headings[@]}"; do
  grep -Fqx -- "$heading" "$NOTES_FILE" || \
    fail "Release notes are missing required heading: $heading"
done

NOTES_SIZE=$(wc -c < "$NOTES_FILE" | tr -d '[:space:]')
[[ "$NOTES_SIZE" =~ ^[0-9]+$ && "$NOTES_SIZE" -ge 400 ]] || \
  fail "Release notes must contain at least 400 bytes of meaningful content"

if grep -Eiq -- 'Replace this text|X\.Y\.Z-ts\.N|<previous>|<current>|(^|[^[:alnum:]_])(TODO|TBD)([^[:alnum:]_]|$)' "$NOTES_FILE"; then
  fail "Release notes still contain template placeholders"
fi

echo "Validated release candidate $RELEASE_TAG using $NOTES_FILE"
