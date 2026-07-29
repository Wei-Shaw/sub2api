#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-candidate.XXXXXX")
cleanup() {
  rm -rf -- "$TEST_ROOT"
}
trap cleanup EXIT

fail() { echo "$*" >&2; exit 1; }
expect_failure() {
  local expected=$1
  shift
  local output
  if output=$("$@" 2>&1); then fail "command unexpectedly succeeded: $*"; fi
  [[ "$output" == *"$expected"* ]] || fail "failure did not contain '$expected': $output"
}

VERSION=$(tr -d '\r\n' < "$REPO_ROOT/backend/cmd/server/VERSION")
COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD)
DIST="$TEST_ROOT/dist"
mkdir -p "$DIST/linux-amd64" "$DIST/linux-arm64"
for arch in amd64 arm64; do
  printf '#!/bin/sh\necho server-%s\n' "$arch" > "$DIST/linux-$arch/sub2api"
  printf '#!/bin/sh\necho deployer-%s\n' "$arch" > "$DIST/linux-$arch/sub2api-deployer"
  chmod +x "$DIST/linux-$arch/sub2api" "$DIST/linux-$arch/sub2api-deployer"
  tar -C "$DIST/linux-$arch" -czf "$DIST/sub2api_${VERSION}_linux_${arch}.tar.gz" sub2api
done
jq -n --arg version "$VERSION" '[
  {type:"Binary", name:"sub2api", goos:"linux", goarch:"amd64", path:"dist/linux-amd64/sub2api", extra:{ID:"sub2api"}},
  {type:"Binary", name:"sub2api", goos:"linux", goarch:"arm64", path:"dist/linux-arm64/sub2api", extra:{ID:"sub2api"}},
  {type:"Binary", name:"sub2api-deployer", goos:"linux", goarch:"amd64", path:"dist/linux-amd64/sub2api-deployer", extra:{ID:"sub2api-deployer"}},
  {type:"Binary", name:"sub2api-deployer", goos:"linux", goarch:"arm64", path:"dist/linux-arm64/sub2api-deployer", extra:{ID:"sub2api-deployer"}},
  {type:"Archive", name:("sub2api_"+$version+"_linux_amd64.tar.gz"), path:("dist/sub2api_"+$version+"_linux_amd64.tar.gz"), extra:{ID:"default"}},
  {type:"Archive", name:("sub2api_"+$version+"_linux_arm64.tar.gz"), path:("dist/sub2api_"+$version+"_linux_arm64.tar.gz"), extra:{ID:"default"}}
]' > "$DIST/artifacts.json"

CANDIDATE="$TEST_ROOT/candidate"
"$REPO_ROOT/deploy/prepare-release-candidate.sh" \
  "$DIST/artifacts.json" "$CANDIDATE" "$VERSION" "$COMMIT" ghcr.io/example/sub2api
[[ -x "$CANDIDATE/oci-context/linux/amd64/sub2api-deployer" ]] || fail "OCI deployer was not prepared"
DIGEST="sha256:$(printf 'a%.0s' {1..64})"
"$REPO_ROOT/deploy/finalize-release-candidate.sh" "$CANDIDATE" "$DIGEST"
"$REPO_ROOT/deploy/verify-release-candidate.sh" "$CANDIDATE" "$VERSION" "$COMMIT"

jq '.image_digest = "sha256:'"$(printf 'b%.0s' {1..64})"'"' "$CANDIDATE/candidate.json" > "$TEST_ROOT/bad.json"
mv "$TEST_ROOT/bad.json" "$CANDIDATE/candidate.json"
expect_failure 'FAILED' bash -c 'cd "$1" && sha256sum --check MANIFEST.sha256' _ "$CANDIDATE"

echo "release candidate artifact tests passed"
