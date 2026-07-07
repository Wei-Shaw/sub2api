#!/usr/bin/env bash
# Build the Ubuntu-based builder image.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IMAGE_NAME="${IMAGE_NAME:-sub2api-builder:ubuntu}"
UBUNTU_IMAGE="${UBUNTU_IMAGE:-hub.bilibili.co/nyx-base/ubuntu:22.04}"
APT_MIRROR="${APT_MIRROR:-mirrors.ustc.edu.cn}"
GO_VERSION="${GO_VERSION:-1.26.2}"
GO_DOWNLOAD_URL="${GO_DOWNLOAD_URL:-http://shjd-boss.bilibili.co/nyx-nas/hezhizhen/go${GO_VERSION}.linux-amd64.tar.gz}"
NODE_VERSION="${NODE_VERSION:-22.0.0}"
NODE_DOWNLOAD_URL="${NODE_DOWNLOAD_URL:-https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz}"
NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"
PNPM_VERSION="${PNPM_VERSION:-9}"
DOWNLOAD_TIMEOUT="${DOWNLOAD_TIMEOUT:-30}"
DOWNLOAD_TRIES="${DOWNLOAD_TRIES:-2}"

docker build -t "${IMAGE_NAME}" \
    --build-arg UBUNTU_IMAGE="${UBUNTU_IMAGE}" \
    --build-arg APT_MIRROR="${APT_MIRROR}" \
    --build-arg GO_VERSION="${GO_VERSION}" \
    --build-arg GO_DOWNLOAD_URL="${GO_DOWNLOAD_URL}" \
    --build-arg NODE_VERSION="${NODE_VERSION}" \
    --build-arg NODE_DOWNLOAD_URL="${NODE_DOWNLOAD_URL}" \
    --build-arg NPM_REGISTRY="${NPM_REGISTRY}" \
    --build-arg PNPM_VERSION="${PNPM_VERSION}" \
    --build-arg DOWNLOAD_TIMEOUT="${DOWNLOAD_TIMEOUT}" \
    --build-arg DOWNLOAD_TRIES="${DOWNLOAD_TRIES}" \
    -f "${REPO_ROOT}/Dockerfile.builder.ubuntu" \
    "${REPO_ROOT}"
