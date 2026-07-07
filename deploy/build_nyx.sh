#!/usr/bin/env bash
# Build script for Nyx-style CI: runs inside the builder image and writes
# release artifacts under $workdir/release/$App_name.

set -eu
(set -o pipefail) >/dev/null 2>&1 && set -o pipefail

if [ -z "$workdir" ]; then
    echo "workdir is required" >&2
    exit 1
fi

if [ -z "$Code_root" ]; then
    echo "Code_root is required" >&2
    exit 1
fi

if [ -z "$App_name" ]; then
    echo "App_name is required" >&2
    exit 1
fi

NPM_REGISTRY_VALUE="${NPM_REGISTRY:-https://registry.npmmirror.com}"
GOPROXY_VALUE="${GOPROXY:-https://goproxy.cn,direct}"
GOSUMDB_VALUE="${GOSUMDB:-sum.golang.google.cn}"
PNPM_VERSION_VALUE="${PNPM_VERSION:-9}"
VERSION_VALUE="${VERSION:-}"
COMMIT_VALUE="${COMMIT:-${GIT_COMMIT:-nyx}}"
DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

export NPM_CONFIG_REGISTRY="${NPM_REGISTRY_VALUE}"
export npm_config_registry="${NPM_REGISTRY_VALUE}"
export GOPROXY="${GOPROXY_VALUE}"
export GOSUMDB="${GOSUMDB_VALUE}"

if [ ! -d "$Code_root" ]; then
    echo "Code_root does not exist: $Code_root" >&2
    exit 1
fi

if [ ! -f "$Code_root/frontend/package.json" ] || [ ! -f "$Code_root/backend/go.mod" ]; then
    echo "Invalid Code_root: $Code_root" >&2
    echo "Expected files: frontend/package.json and backend/go.mod" >&2
    exit 1
fi

RELEASE_DIR="$workdir/release/$App_name"

env
pwd
echo "Resolved workdir=$workdir"
echo "Resolved code_root=$Code_root"
echo "Resolved app_name=$App_name"

rm -rf "$RELEASE_DIR"
mkdir -p "$RELEASE_DIR"
cp -rf "$Code_root"/. "$RELEASE_DIR"

if [ -n "${PRE_BUILD_HOOK:-}" ]; then
    eval "$PRE_BUILD_HOOK"
fi

cd "$RELEASE_DIR/frontend"

if ! command -v node >/dev/null 2>&1; then
    echo "node is required in the builder image" >&2
    exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required in the builder image" >&2
    exit 1
fi

if ! command -v pnpm >/dev/null 2>&1; then
    echo "pnpm is required in the builder image" >&2
    exit 1
fi

pnpm config set registry "$NPM_REGISTRY_VALUE"
pnpm install --frozen-lockfile
pnpm run build

cd "$RELEASE_DIR/backend"
go version
go mod download

if [ -z "$VERSION_VALUE" ]; then
    VERSION_VALUE="$(tr -d '\r\n' < ./cmd/server/VERSION)"
fi

mkdir -p "$RELEASE_DIR/bin"

CGO_ENABLED=0 GOOS=linux go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=$VERSION_VALUE -X main.Commit=$COMMIT_VALUE -X main.Date=$DATE_VALUE -X main.BuildType=release" \
    -trimpath \
    -o "$RELEASE_DIR/bin/sub2api" \
    ./cmd/server

chmod +x "$RELEASE_DIR/bin/sub2api"

if [ -n "${COVERAGE_COMMAND:-}" ]; then
    eval "$COVERAGE_COMMAND"
fi

if [ -n "${POST_BUILD_HOOK:-}" ]; then
    eval "$POST_BUILD_HOOK"
fi
