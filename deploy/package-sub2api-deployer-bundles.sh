#!/usr/bin/env bash

set -euo pipefail
umask 022

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
DIST_DIR=${1:-"$REPO_ROOT/deployer-dist"}
mkdir -p -- "$DIST_DIR"
DIST_DIR=$(cd -- "$DIST_DIR" && pwd -P)

VERSION=$(tr -d '[:space:]' < "$REPO_ROOT/backend/cmd/server/VERSION")
COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD)
[[ "$VERSION" =~ ^[0-9][0-9A-Za-z.-]{0,63}$ ]] || { echo "invalid VERSION: $VERSION" >&2; exit 1; }
[[ "$COMMIT" =~ ^[0-9a-f]{40,64}$ ]] || { echo "invalid commit: $COMMIT" >&2; exit 1; }

declare -a ARCHIVES=()
stage=""
cleanup() {
  if [[ -n "$stage" ]]; then
    rm -rf -- "$stage"
  fi
}
trap cleanup EXIT

for arch in amd64 arm64; do
  binary="sub2api-deployer-linux-$arch"
  [[ -x "$DIST_DIR/$binary" ]] || { echo "missing deployer binary: $DIST_DIR/$binary" >&2; exit 1; }

  stage=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-deployer-bundle.XXXXXX")
  bundle="sub2api-deployer-linux-$arch"
  root="$stage/$bundle"
  mkdir -p -- "$root/deploy"
  install -m 0755 "$DIST_DIR/$binary" "$root/$binary"
  install -m 0755 "$REPO_ROOT/deploy/install-sub2api-deployer.sh" "$root/install-sub2api-deployer.sh"
  install -m 0644 "$REPO_ROOT/deploy/DEPLOYER_BUNDLE_README.md" "$root/README.md"
  for asset in compose.deployer.yml sub2api-deployer.service sub2api-deployer-upgrade.service sub2api-deployer-upgrade.sh sub2api-deployer-tmpfiles.conf sub2api-managed-upstream.conf; do
    install -m 0644 "$REPO_ROOT/deploy/$asset" "$root/deploy/$asset"
  done
  printf '{"schema":1,"version":"%s","commit":"%s","os":"linux","architecture":"%s"}\n' \
    "$VERSION" "$COMMIT" "$arch" > "$root/BUNDLE-MANIFEST.json"
  (
    cd -- "$root"
    sha256sum \
      BUNDLE-MANIFEST.json \
      README.md \
      install-sub2api-deployer.sh \
      "$binary" \
      deploy/compose.deployer.yml \
      deploy/sub2api-deployer.service \
      deploy/sub2api-deployer-upgrade.service \
      deploy/sub2api-deployer-upgrade.sh \
      deploy/sub2api-deployer-tmpfiles.conf \
      deploy/sub2api-managed-upstream.conf > MANIFEST.sha256
  )

  archive="$bundle.tar.gz"
  if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    epoch=${SOURCE_DATE_EPOCH:-$(git -C "$REPO_ROOT" show -s --format=%ct HEAD)}
    tar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner \
      -C "$stage" -czf "$DIST_DIR/$archive" "$bundle"
  else
    COPYFILE_DISABLE=1 tar -C "$stage" -czf "$DIST_DIR/$archive" "$bundle"
  fi
  rm -rf -- "$stage"
  stage=""
  ARCHIVES+=("$archive")
done

(
  cd -- "$DIST_DIR"
  sha256sum \
    sub2api-deployer-linux-amd64 \
    sub2api-deployer-linux-arm64 \
    "${ARCHIVES[@]}" > sub2api-deployer-checksums.txt
)
