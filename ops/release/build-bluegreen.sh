#!/usr/bin/env bash
# Execute on the Docker host. Only a verified Git archive is a build input.
set -euo pipefail
[[ $# == 3 ]] || { echo 'Usage: build-bluegreen.sh archive.tar.gz archive.tar.gz.json sub2api:buaga-custom-v<version>[-r<number>]' >&2; exit 2; }
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
archive="$1"
manifest="$2"
image_tag="$3"
[[ "$image_tag" =~ ^sub2api:buaga-custom-v[0-9]+\.[0-9]+\.[0-9]+(-r[0-9]+)?$ ]] || { echo 'Image tag must be sub2api:buaga-custom-v<major>.<minor>.<patch>[-r<number>]' >&2; exit 1; }
[[ -z "${BUILD_REPO_ROOT:-}" && -z "${BUILD_FRONTEND_SOURCE:-}" && -z "${BUILD_DOCKERFILE:-}" ]] || { echo 'Mutable source or Dockerfile overrides are forbidden' >&2; exit 1; }
stage="$(mktemp -d /tmp/buaga-git-build.XXXXXX)"
python3 "$script_dir/verify_package.py" "$archive" "$manifest" --extract "$stage/source"
commit="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source_git_commit"])' "$manifest")"
version="$(tr -d '[:space:]' < "$stage/source/backend/cmd/server/VERSION")"
[[ -n "${BUAGA_BASE_IMAGE:-}" ]] || { echo 'BUAGA_BASE_IMAGE must name the exact current custom runtime image' >&2; exit 1; }
actual_image="$(docker inspect --type=image --format '{{.Id}}' "$BUAGA_BASE_IMAGE")"
expected_image="$(python3 -c 'import json; print(json.load(open("/home/serveradmin/releases/buaga-current.json"))["image_id"])')"
[[ "$actual_image" == "$expected_image" ]] || { echo 'Runtime base differs from current release' >&2; exit 1; }
[[ "${DRY_RUN:-0}" == 1 ]] && { echo "Verified inputs; no build or traffic change. Source: $stage/source"; exit 0; }
base_tag="sub2api:verified-base-${actual_image:7:16}"
docker tag "$actual_image" "$base_tag"
[[ "$(docker inspect --type=image --format '{{.Id}}' "$base_tag")" == "$expected_image" ]] || exit 1
DOCKER_BUILDKIT=0 docker build --memory=6g --cpu-period=100000 --cpu-quota=200000 --pull=false --label "buaga.source.commit=$commit" --label 'buaga.custom=true' \
  --build-arg "BUAGA_BASE_IMAGE=$base_tag" --build-arg "VERSION=$version" --build-arg "COMMIT=$commit" \
  -f "$script_dir/Dockerfile" -t "$image_tag" "$stage/source"
echo 'Candidate image built. Pilot tests, current-image comparison, custom asset checks and graceful traffic switching are still required.'
