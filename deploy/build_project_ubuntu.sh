#!/usr/bin/env bash
# Build script to be executed inside the Ubuntu builder image.

set -euo pipefail

WORKSPACE="${WORKSPACE:-/workspace}"
OUTPUT_DIR="${OUTPUT_DIR:-${WORKSPACE}/build-out}"
NPM_REGISTRY_VALUE="${NPM_REGISTRY:-https://registry.npmmirror.com}"
GOPROXY_VALUE="${GOPROXY:-https://goproxy.cn,direct}"
GOSUMDB_VALUE="${GOSUMDB:-sum.golang.google.cn}"
VERSION_VALUE="${VERSION:-}"
COMMIT_VALUE="${COMMIT:-docker}"
DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

export NPM_CONFIG_REGISTRY="${NPM_REGISTRY_VALUE}"
export npm_config_registry="${NPM_REGISTRY_VALUE}"
export GOPROXY="${GOPROXY_VALUE}"
export GOSUMDB="${GOSUMDB_VALUE}"
export PATH="/usr/local/go/bin:/usr/local/node/bin:${PATH}"

cd "${WORKSPACE}"
mkdir -p "${OUTPUT_DIR}"

if [ -n "${PRE_BUILD_HOOK:-}" ]; then
    eval "${PRE_BUILD_HOOK}"
fi

cd frontend
pnpm config set registry "${NPM_REGISTRY_VALUE}"
pnpm install --frozen-lockfile
pnpm run build

cd "${WORKSPACE}/backend"
go mod download

if [ -z "${VERSION_VALUE}" ]; then
    VERSION_VALUE="$(tr -d '\r\n' < ./cmd/server/VERSION)"
fi

CGO_ENABLED=0 GOOS=linux go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT_VALUE} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o "${OUTPUT_DIR}/sub2api" \
    ./cmd/server

rm -rf "${OUTPUT_DIR}/resources"
mkdir -p "${OUTPUT_DIR}/resources"
cp -R ./resources/. "${OUTPUT_DIR}/resources/"

if [ -n "${COVERAGE_COMMAND:-}" ]; then
    eval "${COVERAGE_COMMAND}"
fi

if [ -n "${POST_BUILD_HOOK:-}" ]; then
    eval "${POST_BUILD_HOOK}"
fi
