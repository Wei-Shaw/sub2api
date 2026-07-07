#!/usr/bin/env bash
# Run the project build script inside the Ubuntu builder image.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BUILDER_IMAGE="${BUILDER_IMAGE:-sub2api-builder:ubuntu}"
WORKSPACE="${WORKSPACE:-/workspace}"

docker run --rm \
    -v "${REPO_ROOT}:${WORKSPACE}" \
    -w "${WORKSPACE}" \
    -e WORKSPACE="${WORKSPACE}" \
    -e OUTPUT_DIR="${OUTPUT_DIR:-${WORKSPACE}/build-out}" \
    -e NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}" \
    -e GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" \
    -e GOSUMDB="${GOSUMDB:-sum.golang.google.cn}" \
    -e VERSION="${VERSION:-}" \
    -e COMMIT="${COMMIT:-docker}" \
    -e DATE="${DATE:-}" \
    -e PRE_BUILD_HOOK="${PRE_BUILD_HOOK:-}" \
    -e COVERAGE_COMMAND="${COVERAGE_COMMAND:-}" \
    -e POST_BUILD_HOOK="${POST_BUILD_HOOK:-}" \
    "${BUILDER_IMAGE}" \
    bash "${WORKSPACE}/deploy/build_project_ubuntu.sh"
