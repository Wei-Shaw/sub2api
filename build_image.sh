#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

IMAGE_NAME="${IMAGE_NAME:-sub2api:local}"
DOCKERFILE="${DOCKERFILE:-$ROOT_DIR/Dockerfile}"
CONTEXT_DIR="${CONTEXT_DIR:-$ROOT_DIR}"

NPM_CONFIG_REGISTRY="${NPM_CONFIG_REGISTRY:-https://mirrors.tuna.tsinghua.edu.cn/npm/}"
ALPINE_MIRROR="${ALPINE_MIRROR:-https://mirrors.tuna.tsinghua.edu.cn/alpine}"
GOPROXY="${GOPROXY:-https://mirrors.tuna.tsinghua.edu.cn/goproxy/,direct}"
GOSUMDB="${GOSUMDB:-off}"

# Proxy config: set PROXY_URL to your proxy (e.g. http://127.0.0.1:7890).
PROXY_URL="${PROXY_URL:-}"
NO_PROXY="${NO_PROXY:-localhost,127.0.0.1,::1}"
HTTP_PROXY="${HTTP_PROXY:-$PROXY_URL}"
HTTPS_PROXY="${HTTPS_PROXY:-$PROXY_URL}"

VERSION="${VERSION:-local}"
COMMIT="${COMMIT:-local}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

BUILD_ARGS=(
  --build-arg VERSION="$VERSION"
  --build-arg COMMIT="$COMMIT"
  --build-arg DATE="$DATE"
  --build-arg NPM_CONFIG_REGISTRY="$NPM_CONFIG_REGISTRY"
  --build-arg ALPINE_MIRROR="$ALPINE_MIRROR"
  --build-arg GOPROXY="$GOPROXY"
  --build-arg GOSUMDB="$GOSUMDB"
)

if [[ -n "$HTTP_PROXY" || -n "$HTTPS_PROXY" || -n "$NO_PROXY" ]]; then
  BUILD_ARGS+=(
    --build-arg HTTP_PROXY="$HTTP_PROXY"
    --build-arg HTTPS_PROXY="$HTTPS_PROXY"
    --build-arg NO_PROXY="$NO_PROXY"
    --build-arg http_proxy="$HTTP_PROXY"
    --build-arg https_proxy="$HTTPS_PROXY"
    --build-arg no_proxy="$NO_PROXY"
  )
fi

DOCKER_BUILDKIT=1 docker build \
  -t "$IMAGE_NAME" \
  -f "$DOCKERFILE" \
  "${BUILD_ARGS[@]}" \
  "$CONTEXT_DIR"
