#!/usr/bin/env bash

set -euo pipefail
umask 022

[[ $# -eq 5 ]] || {
  echo "usage: $0 <artifacts.json> <candidate-dir> <version> <commit> <image-repository>" >&2
  exit 2
}

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
ARTIFACTS=$1
CANDIDATE_DIR=$2
VERSION=$3
COMMIT=$4
IMAGE_REPOSITORY=$5

[[ -f "$ARTIFACTS" ]] || { echo "GoReleaser artifacts file is missing: $ARTIFACTS" >&2; exit 1; }
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-ts\.[1-9][0-9]*)?$ ]] || { echo "invalid version: $VERSION" >&2; exit 1; }
[[ "$COMMIT" =~ ^[0-9a-f]{40,64}$ ]] || { echo "invalid commit: $COMMIT" >&2; exit 1; }
[[ "$IMAGE_REPOSITORY" =~ ^[a-z0-9][a-z0-9._/-]*$ ]] || { echo "invalid image repository: $IMAGE_REPOSITORY" >&2; exit 1; }
[[ "$(tr -d '\r\n' < "$REPO_ROOT/backend/cmd/server/VERSION")" == "$VERSION" ]] || {
  echo "candidate version does not match backend/cmd/server/VERSION" >&2
  exit 1
}
if [[ -e "$CANDIDATE_DIR" && -n "$(find "$CANDIDATE_DIR" -mindepth 1 -print -quit 2>/dev/null)" ]]; then
  echo "candidate directory must be empty: $CANDIDATE_DIR" >&2
  exit 1
fi

mkdir -p \
  "$CANDIDATE_DIR/release-assets" \
  "$CANDIDATE_DIR/oci-context/deploy" \
  "$CANDIDATE_DIR/oci-context/linux/amd64" \
  "$CANDIDATE_DIR/oci-context/linux/arm64"

artifact_path() {
  local path=$1
  if [[ "$path" == /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s/../%s\n' "$(dirname -- "$ARTIFACTS")" "$path"
  fi
}

for arch in amd64 arm64; do
  server_path=$(jq -er --arg arch "$arch" '
    [ .[]
      | select(.type == "Binary" and .goos == "linux" and .goarch == $arch)
      | select((.extra.ID // "") == "sub2api" or .name == "sub2api")
      | .path ]
    | if length == 1 then .[0] else error("expected exactly one linux/" + $arch + " server artifact") end
  ' "$ARTIFACTS")
  server_path=$(artifact_path "$server_path")
  [[ -x "$server_path" ]] || { echo "server artifact is missing or not executable: $server_path" >&2; exit 1; }
  install -m 0755 "$server_path" "$CANDIDATE_DIR/oci-context/linux/$arch/sub2api"
done

CONTROL_MANIFEST="$CANDIDATE_DIR/oci-context/CONTROL-PLANE-MANIFEST.json"
"$REPO_ROOT/deploy/prepare-control-plane-artifacts.sh" \
  "$ARTIFACTS" \
  "$CANDIDATE_DIR/release-assets" \
  "$CONTROL_MANIFEST" \
  "$VERSION" \
  "$COMMIT" >/dev/null

for arch in amd64 arm64; do
  install -m 0755 \
    "$CANDIDATE_DIR/release-assets/sub2api-deployer-linux-$arch" \
    "$CANDIDATE_DIR/oci-context/linux/$arch/sub2api-deployer"
done
install -m 0755 "$REPO_ROOT/deploy/docker-entrypoint.sh" "$CANDIDATE_DIR/oci-context/deploy/docker-entrypoint.sh"
install -m 0644 "$CONTROL_MANIFEST" "$CANDIDATE_DIR/CONTROL-PLANE-MANIFEST.json"

mapfile -t archives < <(jq -er '
  [ .[] | select(.type == "Archive" and ((.extra.ID // "default") == "default")) | .path ]
  | if length > 0 then .[] else error("no server archives were produced") end
' "$ARTIFACTS")
for archive in "${archives[@]}"; do
  source_path=$(artifact_path "$archive")
  [[ -f "$source_path" ]] || { echo "server archive is missing: $source_path" >&2; exit 1; }
  install -m 0644 "$source_path" "$CANDIDATE_DIR/release-assets/$(basename -- "$source_path")"
done

SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(git -C "$REPO_ROOT" show -s --format=%ct "$COMMIT")}
export SOURCE_DATE_EPOCH
"$REPO_ROOT/deploy/package-sub2api-deployer-bundles.sh" "$CANDIDATE_DIR/release-assets"

(
  cd -- "$CANDIDATE_DIR/release-assets"
  mapfile -t server_archives < <(find . -maxdepth 1 -type f \( -name 'sub2api_*.tar.gz' -o -name 'sub2api_*.zip' \) -print | sed 's#^./##' | LC_ALL=C sort)
  [[ ${#server_archives[@]} -gt 0 ]] || { echo "candidate has no server archives" >&2; exit 1; }
  sha256sum "${server_archives[@]}" > checksums.txt
)

jq -n \
  --arg version "$VERSION" \
  --arg commit "$COMMIT" \
  --arg image "$IMAGE_REPOSITORY:$VERSION" \
  --arg candidate_image "$IMAGE_REPOSITORY:candidate-$COMMIT" \
  '{
    schema: 1,
    version: $version,
    commit: $commit,
    image: $image,
    candidate_image: $candidate_image
  }' > "$CANDIDATE_DIR/candidate-build.json"
