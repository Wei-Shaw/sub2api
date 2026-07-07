#!/usr/bin/env bash
set -euo pipefail

WORK_DIR="/root/workspace/sub2api"
DATA_DIR="/var/lib/sub2api"
PG_DATA="${DATA_DIR}/postgres"
REDIS_DATA="${DATA_DIR}/redis"
RUNTIME_DIR="${DATA_DIR}/runtime"
ENV_FILE="${WORK_DIR}/.env"
BINARY="${WORK_DIR}/sub2api"

PG_VER=""
PG_BIN=""
APP_PID=""
_CLEANED_UP=0

log() { echo "[start-ecs] $*"; }

random_hex() { openssl rand -hex "${1:-32}"; }

install_deps() {
    if ! command -v psql >/dev/null 2>&1; then
        log "Installing PostgreSQL..."
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql postgresql-client redis-server gosu openssl git curl
    fi
    if ! command -v redis-server >/dev/null 2>&1; then
        log "Installing Redis..."
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq redis-server
    fi
    if ! command -v gosu >/dev/null 2>&1; then
        apt-get install -y -qq gosu
    fi
}

discover_pg() {
    PG_VER=$(find /usr/lib/postgresql -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1 | xargs basename)
    PG_BIN="/usr/lib/postgresql/${PG_VER}/bin"
    log "PostgreSQL ${PG_VER} => ${PG_BIN}"
}

setup_dirs() {
    mkdir -p "${PG_DATA}" "${REDIS_DATA}" "${RUNTIME_DIR}"
    touch "${RUNTIME_DIR}/postgres.log"
    chown -R postgres:postgres "${PG_DATA}"
    chown -R postgres:postgres "${RUNTIME_DIR}"
    chown -R redis:redis "${REDIS_DATA}"
    chmod 700 "${PG_DATA}"
    chmod 755 "${RUNTIME_DIR}"
}

init_postgres() {
    if [ -f "${PG_DATA}/PG_VERSION" ]; then
        log "PostgreSQL already initialized"
        return
    fi
    log "Initializing PostgreSQL..."
    gosu postgres "${PG_BIN}/initdb" \
        -D "${PG_DATA}" \
        --auth-local=trust \
        --auth-host=scram-sha-256 \
        --encoding=UTF8 \
        --locale=C.UTF-8
}

start_postgres() {
    log "Starting PostgreSQL..."
    gosu postgres "${PG_BIN}/pg_ctl" \
        -D "${PG_DATA}" \
        -l "${RUNTIME_DIR}/postgres.log" \
        -w start \
        -o "-c listen_addresses=127.0.0.1 -c port=5432 -c unix_socket_directories=${RUNTIME_DIR}"
}

ensure_db() {
    local db_exists
    db_exists=$(gosu postgres psql -tA -h "${RUNTIME_DIR}" -p 5432 postgres -c "SELECT 1 FROM pg_database WHERE datname = 'sub2api'" 2>/dev/null || true)
    if [ "${db_exists}" = "1" ]; then
        log "Database sub2api already exists"
        return
    fi

    local db_pass
    if [ -f "${ENV_FILE}" ]; then
        db_pass=$(grep '^DATABASE_PASSWORD=' "${ENV_FILE}" | cut -d= -f2-)
    fi
    if [ -z "${db_pass:-}" ]; then
        db_pass=$(random_hex 16)
    fi

    log "Creating database and user..."
    gosu postgres psql -h "${RUNTIME_DIR}" -p 5432 postgres -v ON_ERROR_STOP=1 <<SQL
CREATE USER sub2api WITH PASSWORD '${db_pass}';
CREATE DATABASE sub2api OWNER sub2api;
SQL

    if [ -f "${ENV_FILE}" ]; then
        sed -i "s|^DATABASE_PASSWORD=.*|DATABASE_PASSWORD=${db_pass}|" "${ENV_FILE}"
    fi
}

start_redis() {
    if [ -f "${RUNTIME_DIR}/redis.pid" ] && kill -0 "$(cat "${RUNTIME_DIR}/redis.pid")" 2>/dev/null; then
        log "Redis already running"
        return
    fi

    local redis_pass
    if [ -f "${ENV_FILE}" ]; then
        redis_pass=$(grep '^REDIS_PASSWORD=' "${ENV_FILE}" | cut -d= -f2-)
    fi
    if [ -z "${redis_pass:-}" ]; then
        redis_pass=$(random_hex 16)
    fi

    cat > "${RUNTIME_DIR}/redis.conf" <<EOF
bind 127.0.0.1
port 6379
dir ${REDIS_DATA}
pidfile ${RUNTIME_DIR}/redis.pid
daemonize yes
appendonly yes
requirepass ${redis_pass}
EOF

    log "Starting Redis..."
    redis-server "${RUNTIME_DIR}/redis.conf"

    local attempt
    for attempt in $(seq 1 30); do
        if REDISCLI_AUTH="${redis_pass}" redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1; then
            log "Redis ready"
            break
        fi
        sleep 1
    done

    if [ -f "${ENV_FILE}" ]; then
        sed -i "s|^REDIS_PASSWORD=.*|REDIS_PASSWORD=${redis_pass}|" "${ENV_FILE}"
    fi
}

generate_env() {
    if [ -f "${ENV_FILE}" ]; then
        log ".env already exists, keeping it"
        return
    fi

    log "Generating .env..."
    local db_pass redis_pass jwt totp admin_pass
    db_pass=$(random_hex 16)
    redis_pass=$(random_hex 16)
    jwt=$(random_hex 32)
    totp=$(random_hex 32)
    admin_pass=$(random_hex 8)

    cat > "${ENV_FILE}" <<EOF
AUTO_SETUP=true
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_MODE=release
DATABASE_HOST=127.0.0.1
DATABASE_PORT=5432
DATABASE_USER=sub2api
DATABASE_PASSWORD=${db_pass}
DATABASE_DBNAME=sub2api
DATABASE_SSLMODE=disable
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=${redis_pass}
REDIS_DB=0
ADMIN_EMAIL=admin@sub2api.local
ADMIN_PASSWORD=${admin_pass}
JWT_SECRET=${jwt}
TOTP_ENCRYPTION_KEY=${totp}
TZ=Asia/Shanghai
EOF

    chmod 600 "${ENV_FILE}"
    log "================================"
    log "Admin password: ${admin_pass}"
    log "================================"
}

build_binary() {
    if [ -x "${BINARY}" ]; then
        log "Binary already exists: ${BINARY}"
        return
    fi

    log "Building binary..."
    cd "${WORK_DIR}/backend"
    go mod download
    CGO_ENABLED=0 go build -o "${BINARY}" ./cmd/server
    log "Binary built: ${BINARY}"
}

start_app() {
    log "Starting Sub2API..."
    set -a && source "${ENV_FILE}" && set +a
    cd "${WORK_DIR}"
    "${BINARY}" &
    APP_PID="$!"
    wait "${APP_PID}"
}

cleanup() {
    [ "${_CLEANED_UP}" -eq 1 ] && return
    _CLEANED_UP=1
    log "Shutting down..."

    if [ -n "${APP_PID:-}" ] && kill -0 "${APP_PID}" 2>/dev/null; then
        kill -TERM "${APP_PID}" 2>/dev/null || true
        wait "${APP_PID}" 2>/dev/null || true
    fi

    if [ -f "${RUNTIME_DIR}/redis.pid" ]; then
        local redis_pass
        redis_pass=$(grep '^REDIS_PASSWORD=' "${ENV_FILE}" 2>/dev/null | cut -d= -f2-)
        REDISCLI_AUTH="${redis_pass:-}" redis-cli -h 127.0.0.1 -p 6379 shutdown 2>/dev/null || true
    fi

    if [ -n "${PG_BIN:-}" ] && [ -f "${PG_DATA}/postmaster.pid" ]; then
        gosu postgres "${PG_BIN}/pg_ctl" -D "${PG_DATA}" -m fast -w stop 2>/dev/null || true
    fi

    log "Stopped"
}

main() {
    if [ "$(id -u)" != "0" ]; then
        echo "Must run as root" >&2
        exit 1
    fi

    log "WORK_DIR: ${WORK_DIR}"

    install_deps
    discover_pg
    setup_dirs
    init_postgres
    start_postgres
    generate_env
    ensure_db
    start_redis
    build_binary

    trap cleanup EXIT INT TERM
    start_app
}

main "$@"
