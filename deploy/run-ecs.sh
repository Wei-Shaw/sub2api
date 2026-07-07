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

log() { echo "[run-ecs] $*"; }

random_hex() { openssl rand -hex "${1:-32}"; }

# 1. 检查二进制
if [ ! -x "${BINARY}" ]; then
    echo "ERROR: ${BINARY} not found or not executable" >&2
    echo "Build locally and upload first:" >&2
    echo "  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o sub2api ./cmd/server" >&2
    exit 1
fi

# 2. 安装系统依赖
install_deps() {
    if ! command -v psql >/dev/null 2>&1; then
        log "Installing PostgreSQL..."
        apt-get update -qq
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql postgresql-client redis-server gosu openssl
    fi
    if ! command -v redis-server >/dev/null 2>&1; then
        apt-get install -y -qq redis-server
    fi
    if ! command -v gosu >/dev/null 2>&1; then
        apt-get install -y -qq gosu
    fi
}

# 3. 发现 PG
discover_pg() {
    PG_VER=$(find /usr/lib/postgresql -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1 | xargs basename)
    PG_BIN="/usr/lib/postgresql/${PG_VER}/bin"
    log "PostgreSQL ${PG_VER}"
}

# 4. 创建目录
setup_dirs() {
    mkdir -p "${PG_DATA}" "${REDIS_DATA}" "${RUNTIME_DIR}"
    touch "${RUNTIME_DIR}/postgres.log"
    chown -R postgres:postgres "${PG_DATA}" "${RUNTIME_DIR}"
    chown -R redis:redis "${REDIS_DATA}"
    chmod 700 "${PG_DATA}"
    chmod 755 "${RUNTIME_DIR}"
}

# 5. 初始化 PostgreSQL
init_postgres() {
    [ -f "${PG_DATA}/PG_VERSION" ] && return
    log "Initializing PostgreSQL..."
    gosu postgres "${PG_BIN}/initdb" \
        -D "${PG_DATA}" \
        --auth-local=trust \
        --auth-host=scram-sha-256 \
        --encoding=UTF8 \
        --locale=C.UTF-8
}

# 6. 启动 PostgreSQL
start_postgres() {
    log "Starting PostgreSQL..."
    gosu postgres "${PG_BIN}/pg_ctl" \
        -D "${PG_DATA}" \
        -l "${RUNTIME_DIR}/postgres.log" \
        -w start \
        -o "-c listen_addresses=127.0.0.1 -c port=5432 -c unix_socket_directories=${RUNTIME_DIR}"
}

# 7. 创建数据库和用户
ensure_db() {
    local db_exists
    db_exists=$(gosu postgres psql -tA -h "${RUNTIME_DIR}" -p 5432 postgres -c "SELECT 1 FROM pg_database WHERE datname = 'sub2api'" 2>/dev/null || true)
    [ "${db_exists}" = "1" ] && return

    local db_pass
    db_pass=$(grep '^DATABASE_PASSWORD=' "${ENV_FILE}" 2>/dev/null | cut -d= -f2-) || true
    [ -z "${db_pass:-}" ] && db_pass=$(random_hex 16)

    log "Creating database..."
    gosu postgres psql -h "${RUNTIME_DIR}" -p 5432 postgres -v ON_ERROR_STOP=1 <<SQL
CREATE USER sub2api WITH PASSWORD '${db_pass}';
CREATE DATABASE sub2api OWNER sub2api;
SQL

    sed -i "s|^DATABASE_PASSWORD=.*|DATABASE_PASSWORD=${db_pass}|" "${ENV_FILE}" 2>/dev/null || true
}

# 8. 启动 Redis
start_redis() {
    [ -f "${RUNTIME_DIR}/redis.pid" ] && kill -0 "$(cat "${RUNTIME_DIR}/redis.pid")" 2>/dev/null && return

    local redis_pass
    redis_pass=$(grep '^REDIS_PASSWORD=' "${ENV_FILE}" 2>/dev/null | cut -d= -f2-) || true
    [ -z "${redis_pass:-}" ] && redis_pass=$(random_hex 16)

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
        REDISCLI_AUTH="${redis_pass}" redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1 && break
        sleep 1
    done

    sed -i "s|^REDIS_PASSWORD=.*|REDIS_PASSWORD=${redis_pass}|" "${ENV_FILE}" 2>/dev/null || true
}

# 9. 生成环境变量
generate_env() {
    [ -f "${ENV_FILE}" ] && return

    log "Generating .env..."
    local db_pass=$(random_hex 16)
    local redis_pass=$(random_hex 16)
    local jwt=$(random_hex 32)
    local totp=$(random_hex 32)
    local admin_pass=$(random_hex 8)

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
ADMIN_EMAIL=ecs-sub2api@privatemail.local
ADMIN_PASSWORD=zizfo1-fitwom-vicReb
JWT_SECRET=${jwt}
TOTP_ENCRYPTION_KEY=${totp}
TZ=Asia/Shanghai
EOF

    chmod 600 "${ENV_FILE}"
    log "================================"
    log "Admin password: ${admin_pass}"
    log "================================"
}

# 10. 启动应用
start_app() {
    log "Starting Sub2API..."
    set -a && source "${ENV_FILE}" && set +a
    cd "${WORK_DIR}"
    "${BINARY}" &
    APP_PID="$!"
    wait "${APP_PID}"
}

# 清理
cleanup() {
    [ "${_CLEANED_UP}" -eq 1 ] && return
    _CLEANED_UP=1
    log "Shutting down..."

    [ -n "${APP_PID:-}" ] && kill -0 "${APP_PID}" 2>/dev/null && kill -TERM "${APP_PID}" 2>/dev/null && wait "${APP_PID}" 2>/dev/null || true

    local redis_pass
    redis_pass=$(grep '^REDIS_PASSWORD=' "${ENV_FILE}" 2>/dev/null | cut -d= -f2-)
    [ -f "${RUNTIME_DIR}/redis.pid" ] && REDISCLI_AUTH="${redis_pass:-}" redis-cli -h 127.0.0.1 -p 6379 shutdown 2>/dev/null || true

    [ -n "${PG_BIN:-}" ] && [ -f "${PG_DATA}/postmaster.pid" ] && gosu postgres "${PG_BIN}/pg_ctl" -D "${PG_DATA}" -m fast -w stop 2>/dev/null || true

    log "Stopped"
}

main() {
    [ "$(id -u)" != "0" ] && { echo "Must run as root" >&2; exit 1; }

    install_deps
    discover_pg
    setup_dirs
    init_postgres
    start_postgres
    generate_env
    ensure_db
    start_redis

    trap cleanup EXIT INT TERM
    start_app
}

main "$@"
