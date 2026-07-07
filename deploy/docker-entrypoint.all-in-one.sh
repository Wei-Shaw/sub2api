#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APP_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)}"
APP_DATA_DIR="${APP_DATA_DIR:-${APP_ROOT}/data}"
APP_BINARY="${APP_BINARY:-${APP_ROOT}/sub2api}"
DATA_DIR="${DATA_DIR:-${APP_DATA_DIR}}"

RUNTIME_ENV_FILE="${RUNTIME_ENV_FILE:-${APP_DATA_DIR}/runtime.env}"
RUNTIME_STATE_DIR="${RUNTIME_STATE_DIR:-${APP_DATA_DIR}/runtime}"
POSTGRES_DATA_DIR="${POSTGRES_DATA_DIR:-${APP_DATA_DIR}/postgres}"
POSTGRES_RUN_DIR="${POSTGRES_RUN_DIR:-${RUNTIME_STATE_DIR}/postgres}"
POSTGRES_LOG_FILE="${POSTGRES_LOG_FILE:-${POSTGRES_RUN_DIR}/postgres.log}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-${APP_DATA_DIR}/redis}"
REDIS_RUN_DIR="${REDIS_RUN_DIR:-${RUNTIME_STATE_DIR}/redis}"
REDIS_CONF_FILE="${REDIS_CONF_FILE:-${REDIS_RUN_DIR}/redis.conf}"
REDIS_LOG_FILE="${REDIS_LOG_FILE:-${REDIS_RUN_DIR}/redis.log}"

POSTGRES_HOST_BIND="${POSTGRES_HOST_BIND:-127.0.0.1}"
REDIS_HOST_BIND="${REDIS_HOST_BIND:-127.0.0.1}"

AUTO_SETUP="${AUTO_SETUP:-true}"
SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_PORT="${SERVER_PORT:-8080}"
SERVER_MODE="${SERVER_MODE:-release}"
RUN_MODE="${RUN_MODE:-standard}"
TZ="${TZ:-Asia/Shanghai}"

DATABASE_HOST="${DATABASE_HOST:-127.0.0.1}"
DATABASE_PORT="${DATABASE_PORT:-5432}"
DATABASE_USER="${DATABASE_USER:-sub2api}"
DATABASE_PASSWORD="${DATABASE_PASSWORD:-}"
DATABASE_DBNAME="${DATABASE_DBNAME:-sub2api}"
DATABASE_SSLMODE="${DATABASE_SSLMODE:-disable}"
DATABASE_MAX_OPEN_CONNS="${DATABASE_MAX_OPEN_CONNS:-50}"
DATABASE_MAX_IDLE_CONNS="${DATABASE_MAX_IDLE_CONNS:-10}"
DATABASE_CONN_MAX_LIFETIME_MINUTES="${DATABASE_CONN_MAX_LIFETIME_MINUTES:-30}"
DATABASE_CONN_MAX_IDLE_TIME_MINUTES="${DATABASE_CONN_MAX_IDLE_TIME_MINUTES:-5}"

REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
REDIS_DB="${REDIS_DB:-0}"
REDIS_POOL_SIZE="${REDIS_POOL_SIZE:-1024}"
REDIS_MIN_IDLE_CONNS="${REDIS_MIN_IDLE_CONNS:-10}"
REDIS_ENABLE_TLS="${REDIS_ENABLE_TLS:-false}"

ADMIN_EMAIL="${ADMIN_EMAIL:-admin@sub2api.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
JWT_SECRET="${JWT_SECRET:-}"
JWT_EXPIRE_HOUR="${JWT_EXPIRE_HOUR:-24}"
TOTP_ENCRYPTION_KEY="${TOTP_ENCRYPTION_KEY:-}"

GEMINI_OAUTH_CLIENT_ID="${GEMINI_OAUTH_CLIENT_ID:-}"
GEMINI_OAUTH_CLIENT_SECRET="${GEMINI_OAUTH_CLIENT_SECRET:-}"
GEMINI_OAUTH_SCOPES="${GEMINI_OAUTH_SCOPES:-}"
GEMINI_QUOTA_POLICY="${GEMINI_QUOTA_POLICY:-}"
GEMINI_CLI_OAUTH_CLIENT_SECRET="${GEMINI_CLI_OAUTH_CLIENT_SECRET:-}"
ANTIGRAVITY_OAUTH_CLIENT_SECRET="${ANTIGRAVITY_OAUTH_CLIENT_SECRET:-}"
SECURITY_URL_ALLOWLIST_ENABLED="${SECURITY_URL_ALLOWLIST_ENABLED:-false}"
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP="${SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP:-false}"
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS="${SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS:-false}"
SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS="${SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS:-}"
UPDATE_PROXY_URL="${UPDATE_PROXY_URL:-}"

PG_BIN_DIR=""
APP_PID=""
_CLEANED_UP=0

log() {
    printf '[entrypoint] %s\n' "$*"
}

random_hex() {
    openssl rand -hex "${1:-32}"
}

validate_pg_identifier() {
    local value="$1"
    local label="$2"
    if [[ ! "${value}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
        printf 'Invalid %s: %s\n' "${label}" "${value}" >&2
        exit 1
    fi
}

load_runtime_env() {
    if [ -f "${RUNTIME_ENV_FILE}" ]; then
        # shellcheck disable=SC1090
        set -a && . "${RUNTIME_ENV_FILE}" && set +a
    fi
}

ensure_runtime_env_values() {
    DATABASE_HOST="127.0.0.1"
    REDIS_HOST="127.0.0.1"
    REDIS_ENABLE_TLS="false"

    if [ -z "${DATABASE_PASSWORD}" ]; then
        DATABASE_PASSWORD="$(random_hex 24)"
    fi
    if [ -z "${REDIS_PASSWORD}" ]; then
        REDIS_PASSWORD="$(random_hex 24)"
    fi
    if [ -z "${JWT_SECRET}" ]; then
        JWT_SECRET="$(random_hex 32)"
    fi
    if [ -z "${TOTP_ENCRYPTION_KEY}" ]; then
        TOTP_ENCRYPTION_KEY="$(random_hex 32)"
    fi
    if [ -z "${ADMIN_PASSWORD}" ]; then
        ADMIN_PASSWORD="$(random_hex 12)"
    fi
}

write_runtime_env() {
    local tmp_file
    tmp_file="${RUNTIME_ENV_FILE}.tmp"

    umask 077
    : > "${tmp_file}"

    while IFS= read -r name; do
        printf '%s=%q\n' "${name}" "${!name}" >> "${tmp_file}"
    done <<'EOF'
AUTO_SETUP
DATA_DIR
TZ
SERVER_HOST
SERVER_PORT
SERVER_MODE
RUN_MODE
DATABASE_HOST
DATABASE_PORT
DATABASE_USER
DATABASE_PASSWORD
DATABASE_DBNAME
DATABASE_SSLMODE
DATABASE_MAX_OPEN_CONNS
DATABASE_MAX_IDLE_CONNS
DATABASE_CONN_MAX_LIFETIME_MINUTES
DATABASE_CONN_MAX_IDLE_TIME_MINUTES
REDIS_HOST
REDIS_PORT
REDIS_PASSWORD
REDIS_DB
REDIS_POOL_SIZE
REDIS_MIN_IDLE_CONNS
REDIS_ENABLE_TLS
ADMIN_EMAIL
ADMIN_PASSWORD
JWT_SECRET
JWT_EXPIRE_HOUR
TOTP_ENCRYPTION_KEY
GEMINI_OAUTH_CLIENT_ID
GEMINI_OAUTH_CLIENT_SECRET
GEMINI_OAUTH_SCOPES
GEMINI_QUOTA_POLICY
GEMINI_CLI_OAUTH_CLIENT_SECRET
ANTIGRAVITY_OAUTH_CLIENT_SECRET
SECURITY_URL_ALLOWLIST_ENABLED
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS
SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS
UPDATE_PROXY_URL
EOF

    mv "${tmp_file}" "${RUNTIME_ENV_FILE}"
    chmod 0600 "${RUNTIME_ENV_FILE}"
}

prepare_directories() {
    local app_owned_path

    mkdir -p \
        "${APP_DATA_DIR}" \
        "${DATA_DIR}" \
        "${RUNTIME_STATE_DIR}" \
        "${POSTGRES_DATA_DIR}" \
        "${POSTGRES_RUN_DIR}" \
        "${REDIS_DATA_DIR}" \
        "${REDIS_RUN_DIR}"

    touch "${POSTGRES_LOG_FILE}" "${REDIS_LOG_FILE}"

    chown sub2api:sub2api "${APP_DATA_DIR}" "${DATA_DIR}" "${RUNTIME_STATE_DIR}"
    if [ -e "${APP_BINARY}" ]; then
        chown sub2api:sub2api "${APP_BINARY}"
    fi
    for app_owned_path in "${DATA_DIR}/config.yaml" "${DATA_DIR}/.installed"; do
        if [ -e "${app_owned_path}" ]; then
            chown sub2api:sub2api "${app_owned_path}"
        fi
    done
    chown -R postgres:postgres "${POSTGRES_DATA_DIR}" "${POSTGRES_RUN_DIR}" "${POSTGRES_LOG_FILE}"
    chown -R redis:redis "${REDIS_DATA_DIR}" "${REDIS_RUN_DIR}" "${REDIS_LOG_FILE}"

    chmod 0700 "${POSTGRES_DATA_DIR}" "${POSTGRES_RUN_DIR}"
    chmod 0750 "${REDIS_DATA_DIR}" "${REDIS_RUN_DIR}"
}

discover_pg_bin_dir() {
    local initdb_path

    initdb_path="$(find /usr/lib/postgresql -type f -path '*/bin/initdb' | sort -V | tail -n 1)"
    if [ -n "${initdb_path}" ]; then
        PG_BIN_DIR="$(dirname "${initdb_path}")"
        return
    fi

    if command -v initdb >/dev/null 2>&1; then
        PG_BIN_DIR="$(dirname "$(command -v initdb)")"
        return
    fi

    if [ -z "${PG_BIN_DIR}" ]; then
        echo "PostgreSQL binaries not found" >&2
        exit 1
    fi
}

initialize_postgres() {
    if [ ! -f "${POSTGRES_DATA_DIR}/PG_VERSION" ]; then
        log "Initializing PostgreSQL data directory"
        gosu postgres "${PG_BIN_DIR}/initdb" \
            -D "${POSTGRES_DATA_DIR}" \
            --username=postgres \
            --auth-local=trust \
            --auth-host=scram-sha-256 \
            --encoding=UTF8 \
            --locale=C.UTF-8 >/dev/null
    fi
}

start_postgres() {
    log "Starting PostgreSQL on ${POSTGRES_HOST_BIND}:${DATABASE_PORT}"
    gosu postgres "${PG_BIN_DIR}/pg_ctl" \
        -D "${POSTGRES_DATA_DIR}" \
        -l "${POSTGRES_LOG_FILE}" \
        -w start \
        -o "-c listen_addresses=${POSTGRES_HOST_BIND} -c port=${DATABASE_PORT} -c unix_socket_directories=${POSTGRES_RUN_DIR}" >/dev/null
}

ensure_postgres_db() {
    local escaped_password
    local db_exists

    validate_pg_identifier "${DATABASE_USER}" "DATABASE_USER"
    validate_pg_identifier "${DATABASE_DBNAME}" "DATABASE_DBNAME"
    escaped_password="${DATABASE_PASSWORD//\'/\'\'}"

    gosu postgres psql \
        -v ON_ERROR_STOP=1 \
        -h "${POSTGRES_RUN_DIR}" \
        -p "${DATABASE_PORT}" \
        postgres <<SQL >/dev/null
DO \$\$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${DATABASE_USER}') THEN
        CREATE ROLE ${DATABASE_USER} LOGIN SUPERUSER PASSWORD '${escaped_password}';
    ELSE
        ALTER ROLE ${DATABASE_USER} WITH LOGIN SUPERUSER PASSWORD '${escaped_password}';
    END IF;
END
\$\$;
SQL

    db_exists="$(gosu postgres psql -tA -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT}" postgres -c "SELECT 1 FROM pg_database WHERE datname = '${DATABASE_DBNAME}'")"
    if [ "${db_exists}" != "1" ]; then
        gosu postgres createdb -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT}" -O "${DATABASE_USER}" "${DATABASE_DBNAME}"
    else
        gosu postgres psql -v ON_ERROR_STOP=1 -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT}" postgres -c "ALTER DATABASE ${DATABASE_DBNAME} OWNER TO ${DATABASE_USER}" >/dev/null
    fi
}

write_redis_config() {
    cat > "${REDIS_CONF_FILE}" <<EOF
bind ${REDIS_HOST_BIND}
protected-mode yes
port ${REDIS_PORT}
dir ${REDIS_DATA_DIR}
pidfile ${REDIS_RUN_DIR}/redis.pid
daemonize yes
appendonly yes
appendfilename appendonly.aof
save 60 1
logfile ${REDIS_LOG_FILE}
EOF

    if [ -n "${REDIS_PASSWORD}" ]; then
        printf 'requirepass %s\n' "${REDIS_PASSWORD}" >> "${REDIS_CONF_FILE}"
    fi

    chown redis:redis "${REDIS_CONF_FILE}"
    chmod 0600 "${REDIS_CONF_FILE}"
}

start_redis() {
    local attempt

    log "Starting Redis on ${REDIS_HOST_BIND}:${REDIS_PORT}"
    gosu redis redis-server "${REDIS_CONF_FILE}"

    for attempt in $(seq 1 30); do
        if REDISCLI_AUTH="${REDIS_PASSWORD}" redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" ping >/dev/null 2>&1; then
            return
        fi
        sleep 1
    done

    echo "Redis failed to start within 30 seconds" >&2
    exit 1
}

stop_redis() {
    if [ ! -f "${REDIS_RUN_DIR}/redis.pid" ]; then
        return
    fi
    REDISCLI_AUTH="${REDIS_PASSWORD}" redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" shutdown >/dev/null 2>&1 || true
}

stop_postgres() {
    if [ -z "${PG_BIN_DIR}" ] || [ ! -f "${POSTGRES_DATA_DIR}/postmaster.pid" ]; then
        return
    fi
    gosu postgres "${PG_BIN_DIR}/pg_ctl" -D "${POSTGRES_DATA_DIR}" -m fast -w stop >/dev/null 2>&1 || true
}

cleanup() {
    if [ "${_CLEANED_UP}" -eq 1 ]; then
        return
    fi
    _CLEANED_UP=1
    stop_redis
    stop_postgres
}

forward_signal() {
    if [ -n "${APP_PID}" ] && kill -0 "${APP_PID}" 2>/dev/null; then
        kill -TERM "${APP_PID}" 2>/dev/null || true
    fi
}

start_app() {
    export AUTO_SETUP
    export DATA_DIR
    export TZ
    export SERVER_HOST
    export SERVER_PORT
    export SERVER_MODE
    export RUN_MODE
    export DATABASE_HOST
    export DATABASE_PORT
    export DATABASE_USER
    export DATABASE_PASSWORD
    export DATABASE_DBNAME
    export DATABASE_SSLMODE
    export DATABASE_MAX_OPEN_CONNS
    export DATABASE_MAX_IDLE_CONNS
    export DATABASE_CONN_MAX_LIFETIME_MINUTES
    export DATABASE_CONN_MAX_IDLE_TIME_MINUTES
    export REDIS_HOST
    export REDIS_PORT
    export REDIS_PASSWORD
    export REDIS_DB
    export REDIS_POOL_SIZE
    export REDIS_MIN_IDLE_CONNS
    export REDIS_ENABLE_TLS
    export ADMIN_EMAIL
    export ADMIN_PASSWORD
    export JWT_SECRET
    export JWT_EXPIRE_HOUR
    export TOTP_ENCRYPTION_KEY
    export GEMINI_OAUTH_CLIENT_ID
    export GEMINI_OAUTH_CLIENT_SECRET
    export GEMINI_OAUTH_SCOPES
    export GEMINI_QUOTA_POLICY
    export GEMINI_CLI_OAUTH_CLIENT_SECRET
    export ANTIGRAVITY_OAUTH_CLIENT_SECRET
    export SECURITY_URL_ALLOWLIST_ENABLED
    export SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP
    export SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS
    export SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS
    export UPDATE_PROXY_URL

    log "Runtime configuration persisted to ${RUNTIME_ENV_FILE}"
    log "Admin bootstrap credentials: ${APP_DATA_DIR}/runtime.env"
    gosu sub2api "$@" &
    APP_PID="$!"
    wait "${APP_PID}"
}

main() {
    local app_status

    if [ "$#" -eq 0 ]; then
        set -- "${APP_BINARY}"
    elif [ "${1#-}" != "$1" ]; then
        set -- "${APP_BINARY}" "$@"
    fi

    if [ "$1" != "${APP_BINARY}" ] && [ "$(basename -- "$1")" != "$(basename -- "${APP_BINARY}")" ]; then
        exec "$@"
    fi

    if [ "$(id -u)" != "0" ]; then
        echo "This entrypoint must run as root to manage PostgreSQL and Redis" >&2
        exit 1
    fi

    load_runtime_env
    ensure_runtime_env_values
    prepare_directories
    write_runtime_env
    discover_pg_bin_dir
    initialize_postgres
    start_postgres
    ensure_postgres_db
    write_redis_config
    start_redis

    trap forward_signal TERM INT
    trap cleanup EXIT

    start_app "$@" || app_status=$?
    app_status="${app_status:-0}"
    cleanup
    exit "${app_status}"
}

main "$@"
