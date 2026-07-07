#!/bin/sh
set -e

APP_ROOT="${APP_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)}"
APP_DATA_DIR="${APP_DATA_DIR:-${APP_ROOT}/data}"
APP_BINARY="${APP_BINARY:-${APP_ROOT}/sub2api}"

# Fix data directory permissions when running as root.
# Docker named volumes / host bind-mounts may be owned by root,
# preventing the non-root sub2api user from writing files.
if [ "$(id -u)" = "0" ]; then
    mkdir -p "${APP_DATA_DIR}"
    # Use || true to avoid failure on read-only mounted files (e.g. config.yaml:ro)
    chown -R sub2api:sub2api "${APP_DATA_DIR}" 2>/dev/null || true
    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    if command -v su-exec >/dev/null 2>&1; then
        exec su-exec sub2api "$0" "$@"
    fi
    if command -v gosu >/dev/null 2>&1; then
        exec gosu sub2api "$0" "$@"
    fi
    echo "Neither su-exec nor gosu is available" >&2
    exit 1
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- "${APP_BINARY}" "$@"
fi

exec "$@"
