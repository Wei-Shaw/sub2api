#!/usr/bin/env bash

set -euo pipefail

[[ $# -eq 3 ]] || { echo "usage: $0 <immutable-image> <candidate-dir> <config-json>" >&2; exit 2; }
IMMUTABLE_IMAGE=$1
CANDIDATE_DIR=$2
CONFIG_JSON=$3
[[ "$IMMUTABLE_IMAGE" =~ ^[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}$ ]] || {
  echo "invalid immutable image reference: $IMMUTABLE_IMAGE" >&2
  exit 1
}
[[ -f "$CANDIDATE_DIR/candidate.json" && -f "$CONFIG_JSON" ]] || {
  echo "candidate metadata or deployer test config is missing" >&2
  exit 1
}
CONFIG_JSON="$(cd -- "$(dirname -- "$CONFIG_JSON")" && pwd -P)/$(basename -- "$CONFIG_JSON")"

VERSION=$(jq -er '.version' "$CANDIDATE_DIR/candidate.json")
COMMIT=$(jq -er '.commit' "$CANDIDATE_DIR/candidate.json")
EXPECTED_MANIFEST_SHA=$(jq -er '.control_plane_manifest_sha256' "$CANDIDATE_DIR/candidate.json")
work_root=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-control-plane-image.XXXXXX")
cleanup() {
  local container
  for container in "${containers[@]:-}"; do
    [[ -n "$container" ]] && docker rm -f "$container" >/dev/null 2>&1 || true
  done
  rm -rf -- "$work_root"
}
declare -a containers=()
trap cleanup EXIT

docker buildx imagetools inspect "$IMMUTABLE_IMAGE" --format '{{json .}}' > "$work_root/index.json"
jq -e '
  [.manifest.manifests[]? | select(.platform.os == "linux") | .platform.architecture]
  | sort == ["amd64", "arm64"]
' "$work_root/index.json" >/dev/null || { echo "image does not contain exactly linux/amd64 and linux/arm64" >&2; exit 1; }

for arch in amd64 arm64; do
  docker pull --platform "linux/$arch" "$IMMUTABLE_IMAGE" >/dev/null
  container=$(docker create --platform "linux/$arch" --network none "$IMMUTABLE_IMAGE" /bin/true)
  containers+=("$container")
  docker inspect "$container" --format '{{json .Config.Labels}}' > "$work_root/$arch-labels.json"
  jq -e \
    --arg version "$VERSION" \
    --arg commit "$COMMIT" \
    --arg manifest_sha "$EXPECTED_MANIFEST_SHA" '
      .["org.opencontainers.image.version"] == $version
      and .["org.opencontainers.image.revision"] == $commit
      and .["io.tokensupply.sub2api.update-protocol"] == "2"
      and .["io.tokensupply.sub2api.control-plane-protocol"] == "1"
      and .["io.tokensupply.sub2api.control-plane-manifest-sha256"] == $manifest_sha
    ' "$work_root/$arch-labels.json" >/dev/null || {
      echo "control-plane OCI labels are invalid for $arch" >&2
      exit 1
    }
  extracted="$work_root/$arch"
  mkdir -p "$extracted"
  docker cp "$container:/opt/sub2api-control-plane/." "$extracted/"
  docker run --rm --platform "linux/$arch" --network none \
    --entrypoint /bin/sh "$IMMUTABLE_IMAGE" -eu -c '
      test "$(stat -c %u /opt/sub2api-control-plane/sub2api-deployer)" = 0
      test "$(stat -c %u /opt/sub2api-control-plane/CONTROL-PLANE-MANIFEST.json)" = 0
      test "$(stat -c %a /opt/sub2api-control-plane/sub2api-deployer)" = 755
      test "$(stat -c %a /opt/sub2api-control-plane/CONTROL-PLANE-MANIFEST.json)" = 644
    ' || {
      echo "control-plane payload ownership or modes are invalid in the image for $arch" >&2
      exit 1
    }

  [[ -x "$extracted/sub2api-deployer" && -f "$extracted/CONTROL-PLANE-MANIFEST.json" ]] || {
    echo "control-plane payload is incomplete for $arch" >&2
    exit 1
  }
  [[ "$(stat -c %a "$extracted/sub2api-deployer")" == 755 && \
      "$(stat -c %a "$extracted/CONTROL-PLANE-MANIFEST.json")" == 644 ]] || {
    echo "control-plane payload modes are invalid for $arch" >&2
    exit 1
  }
  file "$extracted/sub2api-deployer" | grep -Eq \
    "$( [[ "$arch" == amd64 ]] && printf 'x86-64|x86_64' || printf 'aarch64|ARM aarch64' )" || {
      echo "control-plane binary has the wrong architecture for $arch" >&2
      exit 1
    }

  cmp "$CANDIDATE_DIR/CONTROL-PLANE-MANIFEST.json" "$extracted/CONTROL-PLANE-MANIFEST.json"
  [[ "sha256:$(sha256sum "$extracted/CONTROL-PLANE-MANIFEST.json" | awk '{print $1}')" == "$EXPECTED_MANIFEST_SHA" ]] || {
    echo "embedded control-plane manifest digest mismatch for $arch" >&2
    exit 1
  }
  expected_binary_sha=$(jq -er --arg arch "$arch" '.deployer_assets["sub2api-deployer-linux-" + $arch]' "$CANDIDATE_DIR/candidate.json")
  [[ "sha256:$(sha256sum "$extracted/sub2api-deployer" | awk '{print $1}')" == "$expected_binary_sha" ]] || {
    echo "embedded deployer digest mismatch for $arch" >&2
    exit 1
  }

  version_output=$(docker run --rm --platform "linux/$arch" --network none \
    --entrypoint /opt/sub2api-control-plane/sub2api-deployer "$IMMUTABLE_IMAGE" -version)
  [[ "$version_output" == *"Sub2API Deployer $VERSION "* \
    && "$version_output" == *"commit: $COMMIT"* \
    && "$version_output" == *"type: release"* \
    && "$version_output" == *"arch: $arch"* ]] || {
    echo "embedded deployer build identity mismatch for $arch: $version_output" >&2
    exit 1
  }
  docker run --rm --platform "linux/$arch" --network none \
    -v "$CONFIG_JSON:/candidate-config.json:ro" \
    --entrypoint /opt/sub2api-control-plane/sub2api-deployer \
    "$IMMUTABLE_IMAGE" -check -config /candidate-config.json
  docker run --rm --platform "linux/$arch" --network none --user 1000:1000 \
    --entrypoint /bin/sh "$IMMUTABLE_IMAGE" -eu -c \
    'test ! -w /opt/sub2api-control-plane && ! touch /opt/sub2api-control-plane/write-probe'

  docker rm "$container" >/dev/null
  containers[${#containers[@]}-1]=""
done
