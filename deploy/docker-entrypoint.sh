#!/bin/sh
set -e

# Fix data directory permissions when running as root.
# Docker named volumes / host bind-mounts may be owned by root,
# preventing the non-root sub2api user from writing files.
if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    # Use || true to avoid failure on read-only mounted files (e.g. config.yaml:ro)
    chown -R sub2api:sub2api /app/data 2>/dev/null || true
    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    exec su-exec sub2api "$0" "$@"
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- /app/sub2api "$@"
fi

# Railway / PaaS URL parsing: decompose composite URLs into
# individual env vars when they aren't already set.
# All operations are POSIX sh compatible (Alpine ash).

_url_decode() {
    printf '%s' "$1" | sed 's/%\([0-9A-Fa-f][0-9A-Fa-f]\)/\\x\1/g' | xargs -0 printf '%b' 2>/dev/null || printf '%s' "$1"
}

# postgresql://user:password@host:5432/dbname?sslmode=require
if [ -n "$DATABASE_URL" ] && [ -z "$DATABASE_HOST" ]; then
    _db_rest="$DATABASE_URL"
    _db_rest="${_db_rest#postgresql://}"
    _db_rest="${_db_rest#postgres://}"

    # Split at first / to separate authority from path+query
    _db_authority=$(printf '%s' "$_db_rest" | cut -d'/' -f1)
    _db_pathquery=$(printf '%s' "$_db_rest" | cut -d'/' -f2-)

    _db_path=$(printf '%s' "$_db_pathquery" | cut -d'?' -f1)
    _db_query=$(printf '%s' "$_db_pathquery" | cut -d'?' -f2-)

    # Split at LAST @ to separate user:pass from host:port
    if printf '%s' "$_db_authority" | grep -q '@'; then
        _db_userpass=$(printf '%s' "$_db_authority" | sed 's/@[^@]*$//')
        _db_hostport=$(printf '%s' "$_db_authority" | sed 's/.*@//')
    else
        _db_userpass=""
        _db_hostport="$_db_authority"
    fi

    _db_user=$(printf '%s' "$_db_userpass" | cut -d':' -f1)
    _db_pass=$(printf '%s' "$_db_userpass" | cut -s -d':' -f2)

    _db_host=$(printf '%s' "$_db_hostport" | cut -d':' -f1)
    _db_port=$(printf '%s' "$_db_hostport" | cut -s -d':' -f2)

    _db_name="$_db_path"
    _db_sslmode=$(printf '%s' "$_db_query" | sed -n 's/.*sslmode=\([^&]*\).*/\1/p')

    export DATABASE_HOST="$_db_host"
    export DATABASE_PORT="${_db_port:-5432}"
    export DATABASE_USER="${_db_user:-postgres}"
    export DATABASE_PASSWORD="$(_url_decode "$_db_pass")"
    export DATABASE_DBNAME="${_db_name:-sub2api}"
    [ -n "$_db_sslmode" ] && export DATABASE_SSLMODE="$_db_sslmode"
fi

# redis://[:password@]host:6379[/db] or rediss://default:password@host:6379
if [ -n "$REDIS_URL" ] && [ -z "$REDIS_HOST" ]; then
    _redis_rest="$REDIS_URL"

    # rediss:// scheme means TLS enabled
    case "$_redis_rest" in
        rediss://*)
            export REDIS_ENABLE_TLS="true"
            _redis_rest="${_redis_rest#rediss://}"
            ;;
        redis://*)
            _redis_rest="${_redis_rest#redis://}"
            ;;
    esac

    _redis_authority=$(printf '%s' "$_redis_rest" | cut -d'/' -f1)
    _redis_path=$(printf '%s' "$_redis_rest" | cut -d'/' -f2-)

    if printf '%s' "$_redis_authority" | grep -q '@'; then
        _redis_userpass=$(printf '%s' "$_redis_authority" | sed 's/@[^@]*$//')
        _redis_hostport=$(printf '%s' "$_redis_authority" | sed 's/.*@//')
    else
        _redis_userpass=""
        _redis_hostport="$_redis_authority"
    fi

    _redis_user=$(printf '%s' "$_redis_userpass" | cut -d':' -f1)
    _redis_pass=$(printf '%s' "$_redis_userpass" | cut -s -d':' -f2)

    _redis_host=$(printf '%s' "$_redis_hostport" | cut -d':' -f1)
    _redis_port=$(printf '%s' "$_redis_hostport" | cut -s -d':' -f2)

    _redis_db="$_redis_path"

    export REDIS_HOST="$_redis_host"
    export REDIS_PORT="${_redis_port:-6379}"
    export REDIS_PASSWORD="$(_url_decode "$_redis_pass")"
    [ -n "$_redis_db" ] && export REDIS_DB="$_redis_db"
fi

exec "$@"
