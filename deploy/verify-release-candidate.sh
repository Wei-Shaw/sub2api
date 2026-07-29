#!/usr/bin/env bash

set -euo pipefail

[[ $# -eq 3 ]] || { echo "usage: $0 <candidate-dir> <expected-version> <expected-commit>" >&2; exit 2; }
CANDIDATE_DIR=$1
EXPECTED_VERSION=$2
EXPECTED_COMMIT=$3

[[ -d "$CANDIDATE_DIR" ]] || { echo "candidate directory is missing: $CANDIDATE_DIR" >&2; exit 1; }
[[ "$EXPECTED_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-ts\.[1-9][0-9]*)?$ ]] || { echo "invalid expected version" >&2; exit 1; }
[[ "$EXPECTED_COMMIT" =~ ^[0-9a-f]{40,64}$ ]] || { echo "invalid expected commit" >&2; exit 1; }
[[ -f "$CANDIDATE_DIR/candidate.json" && -f "$CANDIDATE_DIR/MANIFEST.sha256" ]] || {
  echo "candidate metadata or checksum manifest is missing" >&2
  exit 1
}
[[ ! -e "$CANDIDATE_DIR/candidate-build.json" && ! -e "$CANDIDATE_DIR/oci-context" ]] || {
  echo "candidate contains unfinished build state" >&2
  exit 1
}
find "$CANDIDATE_DIR" -type l -print -quit | grep -q . && { echo "candidate contains a symlink" >&2; exit 1; }
(cd -- "$CANDIDATE_DIR" && sha256sum --check MANIFEST.sha256)

jq -e \
  --arg version "$EXPECTED_VERSION" \
  --arg commit "$EXPECTED_COMMIT" '
    .schema == 1
    and .version == $version
    and .commit == $commit
    and (.image | type == "string" and endswith(":" + $version))
    and .candidate_image == ((.image | split(":")[0:-1] | join(":")) + ":candidate-" + $commit)
    and (.image_digest | type == "string" and test("^sha256:[0-9a-f]{64}$"))
    and .immutable_candidate_image == (.candidate_image + "@" + .image_digest)
    and ((.architectures | sort) == ["amd64", "arm64"])
    and (.control_plane_manifest_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
    and (.deployer_checksums_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
    and ((.deployer_assets | keys | sort) == [
      "sub2api-deployer-linux-amd64",
      "sub2api-deployer-linux-amd64.tar.gz",
      "sub2api-deployer-linux-arm64",
      "sub2api-deployer-linux-arm64.tar.gz"
    ])
    and ([.deployer_assets[] | test("^sha256:[0-9a-f]{64}$")] | all)
  ' "$CANDIDATE_DIR/candidate.json" >/dev/null || { echo "candidate identity is invalid" >&2; exit 1; }

CONTROL_MANIFEST="$CANDIDATE_DIR/CONTROL-PLANE-MANIFEST.json"
CONTROL_SHA="sha256:$(sha256sum "$CONTROL_MANIFEST" | awk '{print $1}')"
[[ "$CONTROL_SHA" == "$(jq -er '.control_plane_manifest_sha256' "$CANDIDATE_DIR/candidate.json")" ]] || {
  echo "control-plane manifest digest does not match candidate metadata" >&2
  exit 1
}
jq -e --arg version "$EXPECTED_VERSION" --arg commit "$EXPECTED_COMMIT" '
  .schema == 1 and .version == $version and .commit == $commit
  and ((.runtime_payload | keys | sort) == ["linux/amd64", "linux/arm64"])
' "$CONTROL_MANIFEST" >/dev/null || { echo "control-plane manifest identity is invalid" >&2; exit 1; }

DEPLOYER_CHECKSUMS="$CANDIDATE_DIR/release-assets/sub2api-deployer-checksums.txt"
[[ "sha256:$(sha256sum "$DEPLOYER_CHECKSUMS" | awk '{print $1}')" == \
  "$(jq -er '.deployer_checksums_sha256' "$CANDIDATE_DIR/candidate.json")" ]] || {
  echo "deployer checksum manifest digest does not match candidate metadata" >&2
  exit 1
}
(cd -- "$CANDIDATE_DIR/release-assets" && sha256sum --check sub2api-deployer-checksums.txt && sha256sum --check checksums.txt)

for arch in amd64 arm64; do
  binary="sub2api-deployer-linux-$arch"
  binary_sha="sha256:$(sha256sum "$CANDIDATE_DIR/release-assets/$binary" | awk '{print $1}')"
  [[ "$binary_sha" == "$(jq -er --arg name "$binary" '.deployer_assets[$name]' "$CANDIDATE_DIR/candidate.json")" ]] || {
    echo "candidate deployer digest mismatch for $arch" >&2
    exit 1
  }
  [[ "$binary_sha" == "$(jq -er --arg platform "linux/$arch" '.runtime_payload[$platform].sha256' "$CONTROL_MANIFEST")" ]] || {
    echo "control-plane manifest deployer digest mismatch for $arch" >&2
    exit 1
  }
  bundle_dir=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-candidate-bundle.XXXXXX")
  trap 'rm -rf -- "$bundle_dir"' EXIT
  tar -xzf "$CANDIDATE_DIR/release-assets/$binary.tar.gz" -C "$bundle_dir"
  bundle_root="$bundle_dir/$binary"
  (cd -- "$bundle_root" && sha256sum --check MANIFEST.sha256)
  cmp "$CANDIDATE_DIR/release-assets/$binary" "$bundle_root/$binary"
  jq -e --arg version "$EXPECTED_VERSION" --arg commit "$EXPECTED_COMMIT" --arg arch "$arch" '
    .schema == 1 and .version == $version and .commit == $commit and .os == "linux" and .architecture == $arch
  ' "$bundle_root/BUNDLE-MANIFEST.json" >/dev/null
  rm -rf -- "$bundle_dir"
  trap - EXIT
done
