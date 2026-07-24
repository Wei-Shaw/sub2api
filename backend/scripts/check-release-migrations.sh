#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPOSITORY_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
CHECKER_BIN=${SUB2API_MIGRATION_CHECKER_BIN:-}

if [[ -n "$CHECKER_BIN" ]]; then
  if [[ ! -x "$CHECKER_BIN" ]]; then
    echo "SUB2API_MIGRATION_CHECKER_BIN is not executable: $CHECKER_BIN" >&2
    exit 2
  fi
  exec "$CHECKER_BIN" "$@"
fi

cd -- "$REPOSITORY_ROOT/backend"
exec go run ./cmd/check-release-migrations "$@"
