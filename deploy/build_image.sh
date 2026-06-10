#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Image tag — keep in sync with the compose default (AIBRIDGE_IMAGE / aibridge:latest).
# Override by exporting AIBRIDGE_IMAGE before running this script.
IMAGE="${AIBRIDGE_IMAGE:-aibridge:latest}"

docker build -t "${IMAGE}" \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    -f "${REPO_ROOT}/Dockerfile" \
    "${REPO_ROOT}"
