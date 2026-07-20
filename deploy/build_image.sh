#!/usr/bin/env bash
# Build the frontend and backend into a local Docker image.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
IMAGE_NAME="${IMAGE_NAME:-sub2api:latest}"
VERSION="${VERSION:-}"
PLATFORM="${PLATFORM:-}"

BUILD_ARGS=(
    --tag "${IMAGE_NAME}"
    --build-arg "GOPROXY=${GOPROXY:-https://goproxy.cn,direct}"
    --build-arg "GOSUMDB=${GOSUMDB:-sum.golang.google.cn}"
    --file "${REPO_ROOT}/Dockerfile"
)

if [[ -n "${VERSION}" ]]; then
    BUILD_ARGS+=(--build-arg "VERSION=${VERSION}")
fi

if [[ -n "${PLATFORM}" ]]; then
    BUILD_ARGS+=(--platform "${PLATFORM}")
fi

echo "Building ${IMAGE_NAME} from ${REPO_ROOT}"
docker build "${BUILD_ARGS[@]}" "${REPO_ROOT}"

docker image inspect "${IMAGE_NAME}" \
    --format 'Built {{.RepoTags}} ({{.Architecture}}, {{.Size}} bytes, {{.Created}})'
