#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]REDACTED")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIRREDACTED/.." && pwd)"

docker build -t sub2api:latest \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    -f "${REPO_ROOTREDACTED/Dockerfile" \
    "${REPO_ROOTREDACTED"
