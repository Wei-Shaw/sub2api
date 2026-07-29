#!/usr/bin/env bash

set -euo pipefail
umask 022

[[ $# -eq 5 ]] || {
  echo "usage: $0 <artifacts.json> <deployer-dist> <manifest-output> <version> <commit>" >&2
  exit 2
}

ARTIFACTS=$1
DIST_DIR=$2
MANIFEST_OUTPUT=$3
VERSION=$4
COMMIT=$5

[[ -f "$ARTIFACTS" ]] || { echo "GoReleaser artifacts file is missing: $ARTIFACTS" >&2; exit 1; }
[[ "$VERSION" =~ ^[0-9][0-9A-Za-z.-]{0,63}$ ]] || { echo "invalid version: $VERSION" >&2; exit 1; }
[[ "$COMMIT" =~ ^[0-9a-f]{40,64}$ ]] || { echo "invalid commit: $COMMIT" >&2; exit 1; }

mkdir -p -- "$DIST_DIR" "$(dirname -- "$MANIFEST_OUTPUT")"
DIST_DIR=$(cd -- "$DIST_DIR" && pwd -P)

declare -A DIGESTS=()
for arch in amd64 arm64; do
  artifact=$(jq -er --arg arch "$arch" '
    [
      .[]
      | select(.type == "Binary" and .goos == "linux" and .goarch == $arch)
      | select((.extra.ID // "") == "sub2api-deployer" or .name == "sub2api-deployer")
      | .path
    ]
    | if length == 1 then .[0] else error("expected exactly one linux/" + $arch + " deployer artifact") end
  ' "$ARTIFACTS")
  if [[ "$artifact" != /* ]]; then
    artifact="$(dirname -- "$ARTIFACTS")/../$artifact"
  fi
  [[ -f "$artifact" && -x "$artifact" ]] || { echo "deployer artifact is missing or not executable: $artifact" >&2; exit 1; }
  output="$DIST_DIR/sub2api-deployer-linux-$arch"
  install -m 0755 "$artifact" "$output"
  DIGESTS[$arch]="sha256:$(sha256sum "$output" | awk '{print $1}')"
done

temporary_manifest="$MANIFEST_OUTPUT.tmp.$$"
trap 'rm -f -- "$temporary_manifest"' EXIT
jq -n \
  --arg version "$VERSION" \
  --arg commit "$COMMIT" \
  --arg amd64_sha "${DIGESTS[amd64]}" \
  --arg arm64_sha "${DIGESTS[arm64]}" \
  '{
    schema: 1,
    version: $version,
    commit: $commit,
    runtime_payload: {
      "linux/amd64": {
        path: "/opt/sub2api-control-plane/sub2api-deployer",
        sha256: $amd64_sha
      },
      "linux/arm64": {
        path: "/opt/sub2api-control-plane/sub2api-deployer",
        sha256: $arm64_sha
      }
    }
  }' > "$temporary_manifest"
chmod 0644 "$temporary_manifest"
mv -f -- "$temporary_manifest" "$MANIFEST_OUTPUT"
trap - EXIT

printf 'sha256:%s\n' "$(sha256sum "$MANIFEST_OUTPUT" | awk '{print $1}')"
