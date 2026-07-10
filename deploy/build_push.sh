#!/usr/bin/env bash
# 交叉构建 sub2api 镜像并推送到私有仓库。
# 默认 arm64(Mac) -> linux/amd64 服务器：前端/后端在本机原生构建，仅产出 amd64 二进制，
# 避免 QEMU 全模拟（构建从 ~40min 降到 ~10min）。
#
# 用法：
#   bash deploy/build_push.sh                 # 用下面的默认值
#   IMAGE=docker-registry.xinsulv.com/ns/sub2api TAG=v0.1.139 bash deploy/build_push.sh
#
# 前置：已 docker login 到目标 registry。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IMAGE="${IMAGE:-docker-registry.xinsulv.com/sub2api}"
TAG="${TAG:-latest}"
PLATFORM="${PLATFORM:-linux/amd64}"
BUILDER="${BUILDER:-desktop-linux}"

# 用 Docker Desktop 自带的 docker 驱动 builder：它共享 daemon 的镜像加速器
# (registry-mirrors) 与 insecure-registries 配置，能拉基础镜像、能推私有 insecure 仓库。
# 独立的 docker-container builder 不继承这些配置，会拉镜像超时/推送失败。
# 代价：docker 驱动不支持 --push，故先 --load 到本地再 docker push。
echo "==> 构建 ${IMAGE}:${TAG} (${PLATFORM})"
docker buildx build \
  --builder "${BUILDER}" \
  --platform "${PLATFORM}" \
  -t "${IMAGE}:${TAG}" \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  --build-arg GOSUMDB=sum.golang.google.cn \
  --load \
  -f "${REPO_ROOT}/Dockerfile" \
  "${REPO_ROOT}"

echo "==> 推送 ${IMAGE}:${TAG}"
docker push "${IMAGE}:${TAG}"

echo "✅ 已推送 ${IMAGE}:${TAG} (${PLATFORM})"
echo "   服务器更新：docker compose pull sub2api && docker compose up -d sub2api"
