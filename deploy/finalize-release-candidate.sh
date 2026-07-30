#!/usr/bin/env bash

set -euo pipefail
umask 022

[[ $# -eq 2 ]] || { echo "usage: $0 <candidate-dir> <image-digest>" >&2; exit 2; }
CANDIDATE_DIR=$1
IMAGE_DIGEST=$2
[[ "$IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "invalid image digest: $IMAGE_DIGEST" >&2; exit 1; }
[[ -f "$CANDIDATE_DIR/candidate-build.json" ]] || { echo "candidate build metadata is missing" >&2; exit 1; }
[[ ! -e "$CANDIDATE_DIR/MANIFEST.sha256" && ! -e "$CANDIDATE_DIR/candidate.json" ]] || {
  echo "candidate was already finalized" >&2
  exit 1
}

VERSION=$(jq -er '.version' "$CANDIDATE_DIR/candidate-build.json")
COMMIT=$(jq -er '.commit' "$CANDIDATE_DIR/candidate-build.json")
IMAGE=$(jq -er '.image' "$CANDIDATE_DIR/candidate-build.json")
CANDIDATE_IMAGE=$(jq -er '.candidate_image' "$CANDIDATE_DIR/candidate-build.json")
WORKFLOW_COMMIT=$(jq -er '.workflow_commit' "$CANDIDATE_DIR/candidate-build.json")
PREFLIGHT_WORKFLOW_BLOB=$(jq -er '.workflow_blobs["release-preflight.yml"]' "$CANDIDATE_DIR/candidate-build.json")
PROMOTE_WORKFLOW_BLOB=$(jq -er '.workflow_blobs["promote-release.yml"]' "$CANDIDATE_DIR/candidate-build.json")
RELEASE_WORKFLOW_BLOB=$(jq -er '.workflow_blobs["release.yml"]' "$CANDIDATE_DIR/candidate-build.json")
CONTROL_SHA="sha256:$(sha256sum "$CANDIDATE_DIR/CONTROL-PLANE-MANIFEST.json" | awk '{print $1}')"
DEPLOYER_CHECKSUMS="$CANDIDATE_DIR/release-assets/sub2api-deployer-checksums.txt"
DEPLOYER_CHECKSUMS_SHA="sha256:$(sha256sum "$DEPLOYER_CHECKSUMS" | awk '{print $1}')"

asset_digest() {
  local name=$1
  local digest
  digest=$(awk -v name="$name" '$2 == name || $2 == "*" name {print "sha256:" $1; exit}' "$DEPLOYER_CHECKSUMS")
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "missing deployer digest: $name" >&2; exit 1; }
  printf '%s\n' "$digest"
}

jq -n \
  --arg version "$VERSION" \
  --arg commit "$COMMIT" \
  --arg image "$IMAGE" \
  --arg candidate_image "$CANDIDATE_IMAGE" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg control_sha "$CONTROL_SHA" \
  --arg deployer_checksums_sha "$DEPLOYER_CHECKSUMS_SHA" \
  --arg deployer_amd64_sha "$(asset_digest sub2api-deployer-linux-amd64)" \
  --arg deployer_arm64_sha "$(asset_digest sub2api-deployer-linux-arm64)" \
  --arg bundle_amd64_sha "$(asset_digest sub2api-deployer-linux-amd64.tar.gz)" \
  --arg bundle_arm64_sha "$(asset_digest sub2api-deployer-linux-arm64.tar.gz)" \
  --arg workflow_commit "$WORKFLOW_COMMIT" \
  --arg preflight_workflow_blob "$PREFLIGHT_WORKFLOW_BLOB" \
  --arg promote_workflow_blob "$PROMOTE_WORKFLOW_BLOB" \
  --arg release_workflow_blob "$RELEASE_WORKFLOW_BLOB" \
  '{
    schema: 2,
    version: $version,
    commit: $commit,
    image: $image,
    candidate_image: $candidate_image,
    image_digest: $image_digest,
    immutable_candidate_image: ($candidate_image + "@" + $image_digest),
    architectures: ["amd64", "arm64"],
    control_plane_manifest_sha256: $control_sha,
    workflow_commit: $workflow_commit,
    workflow_blobs: {
      "release-preflight.yml": $preflight_workflow_blob,
      "promote-release.yml": $promote_workflow_blob,
      "release.yml": $release_workflow_blob
    },
    deployer_checksums_sha256: $deployer_checksums_sha,
    deployer_assets: {
      "sub2api-deployer-linux-amd64": $deployer_amd64_sha,
      "sub2api-deployer-linux-arm64": $deployer_arm64_sha,
      "sub2api-deployer-linux-amd64.tar.gz": $bundle_amd64_sha,
      "sub2api-deployer-linux-arm64.tar.gz": $bundle_arm64_sha
    }
  }' > "$CANDIDATE_DIR/candidate.json"

rm -f -- "$CANDIDATE_DIR/candidate-build.json"
rm -rf -- "$CANDIDATE_DIR/oci-context"
(
  cd -- "$CANDIDATE_DIR"
  find . -type l -print -quit | grep -q . && { echo "candidate must not contain symlinks" >&2; exit 1; }
  mapfile -t files < <(find . -type f ! -name MANIFEST.sha256 -print | sed 's#^./##' | LC_ALL=C sort)
  sha256sum "${files[@]}" > MANIFEST.sha256
)
