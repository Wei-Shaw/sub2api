#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${SUB2API_PACKAGE_ROOT:-${ROOT_DIR}/../.codex-tmp}"
VERSION="$(tr -d '\r\n[:space:]' < "${ROOT_DIR}/backend/cmd/server/VERSION")"
COMMIT_SHA="$(git -C "${ROOT_DIR}" rev-parse --short=8 HEAD)"
BUILD_DATE_UTC="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
STAMP="$(date +%Y%m%d-%H%M%S)"
PACKAGE_BASENAME="sub2api-localdeploy-${VERSION}-${COMMIT_SHA}-linux-amd64-${STAMP}"
PACKAGE_DIR="${OUT_ROOT}/${PACKAGE_BASENAME}"
PACKAGE_TAR="${PACKAGE_DIR}.tar.gz"

mkdir -p "${PACKAGE_DIR}"

if [ "${SKIP_FRONTEND_BUILD:-0}" != "1" ]; then
  pnpm --dir "${ROOT_DIR}/frontend" install --frozen-lockfile
  pnpm --dir "${ROOT_DIR}/frontend" run build
fi

(
  cd "${ROOT_DIR}/backend"
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
    -tags embed \
    -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT_SHA} -X main.Date=${BUILD_DATE_UTC} -X main.BuildType=release" \
    -o "${PACKAGE_DIR}/sub2api" \
    ./cmd/server
)

if tar --help 2>/dev/null | grep -q -- '--no-xattrs'; then
  tar --no-xattrs -czf "${PACKAGE_TAR}" -C "${PACKAGE_DIR}" sub2api
else
  tar -czf "${PACKAGE_TAR}" -C "${PACKAGE_DIR}" sub2api
fi

ls -lh "${PACKAGE_DIR}/sub2api" "${PACKAGE_TAR}"
printf '%s\n' "${PACKAGE_TAR}"
