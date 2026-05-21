#!/usr/bin/env bash
# Deprecated compatibility wrapper.
# Public LDAP upgrade entrypoint is deploy/upgrade_main.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET_SCRIPT="${SCRIPT_DIR}/upgrade_main.sh"

if [[ ! -f "$TARGET_SCRIPT" ]]; then
    echo "ERROR: upgrade_main.sh not found next to upgrade_ldap_prod.sh. Download deploy/upgrade_main.sh and rerun." >&2
    exit 1
fi

echo "WARN: upgrade_ldap_prod.sh is deprecated. Redirecting to upgrade_main.sh." >&2
exec bash "$TARGET_SCRIPT" "$@"
