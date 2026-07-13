#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if ! command -v go >/dev/null 2>&1; then
  echo "go 未安装，无法运行 e2e 测试（需要 Go 1.26.2+）" >&2
  exit 1
fi

cd "$PROJECT_ROOT"

if [ "${1:-}" = "--local" ]; then
  shift
fi

GOFLAGS_VALUE="${GOFLAGS:-}"
if [[ " $GOFLAGS_VALUE " != *" -p="* ]]; then
  GOFLAGS_VALUE="${GOFLAGS_VALUE:+$GOFLAGS_VALUE }-p=1"
fi

GOMAXPROCS_VALUE="${GOMAXPROCS:-2}"

GOFLAGS="$GOFLAGS_VALUE" GOMAXPROCS="$GOMAXPROCS_VALUE" \
  go test -tags=e2e -v -timeout=300s ./internal/integration/... "$@"
