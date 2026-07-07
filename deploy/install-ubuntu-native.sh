#!/usr/bin/env bash
# =============================================================================
# Sub2API Ubuntu Native All-in-One Installer (No Systemd)
# =============================================================================
# For repeated deployment on ECS. Manages PG + Redis + App lifecycle via
# start.sh / stop.sh scripts (no systemd registration).
#
# Usage:
#   # Download binary from GitHub Release (default)
#   sudo bash install-ubuntu-native.sh
#
#   # Build from source (requires Go + Node)
#   sudo BUILD_FROM_SOURCE=1 bash install-ubuntu-native.sh
#
#   # Specify custom repo
#   sudo REPO=PigeonYYM/sub2api bash install-ubuntu-native.sh
# =============================================================================

set -euo pipefail

# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------
REPO="${REPO:-PigeonYYM/sub2api}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/sub2api}"
SRC_DIR="${SRC_DIR:-/opt/sub2api-src}"
DATA_DIR="${DATA_DIR:-/var/lib/sub2api}"
CONFIG_DIR="${CONFIG_DIR:-/etc/sub2api}"
LOG_DIR="${LOG_DIR:-/var/log/sub2api}"
RUNTIME_DIR="${RUNTIME_DIR:-/var/lib/sub2api/runtime}"

SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_PORT="${SERVER_PORT:-8080}"
SERVER_MODE="${SERVER_MODE:-release}"
RUN_MODE="${RUN_MODE:-standard}"
TZ_VALUE="${TZ:-Asia/Shanghai}"

POSTGRES_DATA_DIR="${DATA_DIR}/postgres"
POSTGRES_RUN_DIR="${RUNTIME_DIR}/postgres"
POSTGRES_LOG_FILE="${LOG_DIR}/postgres.log"
REDIS_DATA_DIR="${DATA_DIR}/redis"
REDIS_RUN_DIR="${RUNTIME_DIR}/redis"
REDIS_CONF_FILE="${RUNTIME_DIR}/redis.conf"
REDIS_LOG_FILE="${LOG_DIR}/redis.log"
RUNTIME_ENV_FILE="${CONFIG_DIR}/runtime.env"

DATABASE_HOST="127.0.0.1"
DATABASE_PORT="5432"
DATABASE_USER="sub2api"
DATABASE_DBNAME="sub2api"
DATABASE_SSLMODE="disable"
REDIS_HOST="127.0.0.1"
REDIS_PORT="6379"
REDIS_DB="0"
ADMIN_EMAIL="admin@sub2api.local"
JWT_EXPIRE_HOUR="24"

PG_BIN_DIR=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

print_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[OK]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_error()   { echo -e "${RED}[ERR]${NC} $1"; }

random_hex() { openssl rand -hex "${1:-32}"; }
command_exists() { command -v "$1" >/dev/null 2>&1; }

get_latest_version() {
    curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
}

# -----------------------------------------------------------------------------
# Pre-flight
# -----------------------------------------------------------------------------
check_root() {
    if [ "$(id -u)" != "0" ]; then
        print_error "请使用 root 权限运行 (sudo)"
        exit 1
    fi
}

check_os() {
    if [ ! -f /etc/os-release ]; then
        print_error "无法检测操作系统"
        exit 1
    fi
    . /etc/os-release
    if [ "$ID" != "ubuntu" ] && [ "$ID" != "debian" ]; then
        print_error "仅支持 Ubuntu/Debian，检测到: $ID"
        exit 1
    fi
    local v
    v=$(echo "$VERSION_ID" | cut -d. -f1)
    if [ "$v" -lt 20 ]; then
        print_error "需要 Ubuntu 20.04+ 或 Debian 11+"
        exit 1
    fi
    print_info "系统: $PRETTY_NAME"
}

check_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64)  ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        *) print_error "不支持的架构: $arch"; exit 1 ;;
    esac
    print_info "架构: $ARCH"
}

# -----------------------------------------------------------------------------
# Install dependencies
# -----------------------------------------------------------------------------
install_deps() {
    print_info "安装系统依赖..."
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        ca-certificates curl gosu openssl postgresql postgresql-client \
        redis-server tzdata wget jq git

    ln -sf "/usr/share/zoneinfo/${TZ_VALUE}" /etc/localtime
    echo "${TZ_VALUE}" > /etc/timezone

    # Disable system services (we manage them manually)
    systemctl disable --now postgresql 2>/dev/null || true
    systemctl disable --now redis-server 2>/dev/null || true
    print_success "系统依赖安装完成"
}

# -----------------------------------------------------------------------------
# Install build tools (Go + Node) if building from source
# -----------------------------------------------------------------------------
install_build_tools() {
    print_info "安装编译工具..."

    if ! command_exists go; then
        print_info "安装 Go..."
        local go_version="1.25.7"
        local go_tar="go${go_version}.linux-${ARCH}.tar.gz"
        curl -sSL "https://go.dev/dl/${go_tar}" -o "/tmp/${go_tar}"
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "/tmp/${go_tar}"
        rm -f "/tmp/${go_tar}"
        ln -sf /usr/local/go/bin/go /usr/local/bin/go
        ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
        print_success "Go $(go version | awk '{print $3}') 安装完成"
    fi

    if ! command_exists node; then
        print_info "安装 Node.js..."
        curl -fsSL https://deb.nodesource.com/setup_20.x | bash - >/dev/null 2>&1
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nodejs
        print_success "Node $(node --version) 安装完成"
    fi

    if ! command_exists pnpm; then
        print_info "安装 pnpm..."
        npm install -g pnpm >/dev/null 2>&1
        print_success "pnpm $(pnpm --version) 安装完成"
    fi
}

# -----------------------------------------------------------------------------
# Setup directories
# -----------------------------------------------------------------------------
setup_dirs() {
    print_info "创建目录结构..."
    mkdir -p "$DEPLOY_DIR" "$SRC_DIR" "$DATA_DIR" "$CONFIG_DIR" "$LOG_DIR" "$RUNTIME_DIR"
    mkdir -p "$POSTGRES_DATA_DIR" "$POSTGRES_RUN_DIR" "$REDIS_DATA_DIR" "$REDIS_RUN_DIR"
    touch "$POSTGRES_LOG_FILE" "$REDIS_LOG_FILE"

    if ! id -u sub2api >/dev/null 2>&1; then
        useradd --system --home-dir "$DEPLOY_DIR" --shell /bin/false sub2api
    fi
    if ! id -u postgres >/dev/null 2>&1; then
        useradd --system --home-dir /var/lib/postgresql --shell /bin/bash postgres
    fi
    if ! id -u redis >/dev/null 2>&1; then
        useradd --system --home-dir /var/lib/redis --shell /usr/sbin/nologin redis
    fi

    chown -R sub2api:sub2api "$DEPLOY_DIR" "$DATA_DIR" "$CONFIG_DIR"
    chown -R postgres:postgres "$POSTGRES_DATA_DIR" "$POSTGRES_RUN_DIR" "$POSTGRES_LOG_FILE"
    chown -R redis:redis "$REDIS_DATA_DIR" "$REDIS_RUN_DIR" "$REDIS_CONF_FILE" "$REDIS_LOG_FILE"
    chmod 0700 "$POSTGRES_DATA_DIR" "$POSTGRES_RUN_DIR"
    chmod 0750 "$REDIS_DATA_DIR" "$REDIS_RUN_DIR"
    chmod 0755 "$LOG_DIR"
    print_success "目录创建完成"
}

# -----------------------------------------------------------------------------
# Deploy binary (from Release or build from source)
# -----------------------------------------------------------------------------
deploy_binary() {
    if [ "${BUILD_FROM_SOURCE:-0}" = "1" ]; then
        deploy_from_source
    else
        deploy_from_release
    fi
}

deploy_from_release() {
    print_info "从 GitHub Release 下载二进制..."
    local version
    version=$(get_latest_version)
    if [ -z "$version" ]; then
        print_error "无法获取 Release 版本。可能该仓库没有 Release。"
        print_info "尝试从源码编译: sudo BUILD_FROM_SOURCE=1 bash $0"
        exit 1
    fi
    print_info "最新版本: $version"

    local version_num=${version#v}
    local archive="sub2api_${version_num}_linux_${ARCH}.tar.gz"
    local url="https://github.com/${REPO}/releases/download/${version}/${archive}"
    local tmpdir
    tmpdir=$(mktemp -d)

    print_info "下载: $archive"
    if ! curl -sSL -o "$tmpdir/$archive" "$url"; then
        print_error "下载失败"
        rm -rf "$tmpdir"
        exit 1
    fi

    tar -xzf "$tmpdir/$archive" -C "$tmpdir"
    cp "$tmpdir/sub2api" "$DEPLOY_DIR/sub2api"
    chmod +x "$DEPLOY_DIR/sub2api"

    if [ -d "$tmpdir/resources" ]; then
        rm -rf "$DEPLOY_DIR/resources"
        cp -R "$tmpdir/resources" "$DEPLOY_DIR/resources"
        chown -R sub2api:sub2api "$DEPLOY_DIR/resources"
    fi

    rm -rf "$tmpdir"
    print_success "二进制部署完成: $DEPLOY_DIR/sub2api"
}

deploy_from_source() {
    print_info "从源码编译..."
    install_build_tools

    if [ -d "${SRC_DIR}/.git" ]; then
        print_info "更新源码..."
        cd "$SRC_DIR"
        git pull --ff-only
    else
        print_info "克隆仓库: https://github.com/${REPO}.git"
        rm -rf "$SRC_DIR"
        git clone "https://github.com/${REPO}.git" "$SRC_DIR"
        cd "$SRC_DIR"
    fi

    # Build frontend
    print_info "编译前端..."
    cd "$SRC_DIR/frontend"
    pnpm config set registry "https://registry.npmmirror.com"
    pnpm install --frozen-lockfile
    pnpm run build

    # Build backend
    print_info "编译后端..."
    cd "$SRC_DIR/backend"
    go mod download

    local version_value
    version_value=$(tr -d '\r\n' < ./cmd/server/VERSION 2>/dev/null || echo "dev")

    CGO_ENABLED=0 GOOS=linux go build \
        -tags embed \
        -ldflags="-s -w -X main.Version=${version_value} -X main.Commit=src-$(git rev-parse --short HEAD) -X main.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.BuildType=release" \
        -trimpath \
        -o "$DEPLOY_DIR/sub2api" \
        ./cmd/server

    chmod +x "$DEPLOY_DIR/sub2api"

    # Copy resources
    rm -rf "$DEPLOY_DIR/resources"
    mkdir -p "$DEPLOY_DIR/resources"
    cp -R "$SRC_DIR/backend/resources/." "$DEPLOY_DIR/resources/"
    chown -R sub2api:sub2api "$DEPLOY_DIR/resources"

    print_success "源码编译完成"
}

# -----------------------------------------------------------------------------
# Generate secrets
# -----------------------------------------------------------------------------
generate_secrets() {
    if [ -f "$RUNTIME_ENV_FILE" ] && [ "${FORCE_REGENERATE:-0}" != "1" ]; then
        print_info "runtime.env 已存在，保留现有配置"
        return
    fi

    print_info "生成安全密钥..."
    local db_pass redis_pass jwt_secret totp_key admin_pass
    db_pass=$(random_hex 24)
    redis_pass=$(random_hex 24)
    jwt_secret=$(random_hex 32)
    totp_key=$(random_hex 32)
    admin_pass=$(random_hex 12)

    umask 077
    cat > "$RUNTIME_ENV_FILE" <<EOF
AUTO_SETUP=true
DATA_DIR=${DATA_DIR}
TZ=${TZ_VALUE}
SERVER_HOST=${SERVER_HOST}
SERVER_PORT=${SERVER_PORT}
SERVER_MODE=${SERVER_MODE}
RUN_MODE=${RUN_MODE}
DATABASE_HOST=${DATABASE_HOST}
DATABASE_PORT=${DATABASE_PORT}
DATABASE_USER=${DATABASE_USER}
DATABASE_PASSWORD=${db_pass}
DATABASE_DBNAME=${DATABASE_DBNAME}
DATABASE_SSLMODE=${DATABASE_SSLMODE}
REDIS_HOST=${REDIS_HOST}
REDIS_PORT=${REDIS_PORT}
REDIS_PASSWORD=${redis_pass}
REDIS_DB=${REDIS_DB}
ADMIN_EMAIL=${ADMIN_EMAIL}
ADMIN_PASSWORD=${admin_pass}
JWT_SECRET=${jwt_secret}
JWT_EXPIRE_HOUR=${JWT_EXPIRE_HOUR}
TOTP_ENCRYPTION_KEY=${totp_key}
EOF
    chmod 0600 "$RUNTIME_ENV_FILE"
    chown sub2api:sub2api "$RUNTIME_ENV_FILE"

    print_success "密钥已生成"
    echo ""
    echo -e "${CYAN}========== 凭证（请保存） ==========${NC}"
    echo "  Admin 密码:  ${YELLOW}${admin_pass}${NC}"
    echo "  数据库密码:  ${YELLOW}${db_pass}${NC}"
    echo "  Redis 密码:  ${YELLOW}${redis_pass}${NC}"
    echo ""
    echo -e "${YELLOW}已保存到: ${RUNTIME_ENV_FILE}${NC}"
    echo ""
}

# -----------------------------------------------------------------------------
# Discover PG binaries
# -----------------------------------------------------------------------------
discover_pg() {
    PG_BIN_DIR=$(find /usr/lib/postgresql -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1)
    if [ -z "$PG_BIN_DIR" ]; then
        print_error "未找到 PostgreSQL"
        exit 1
    fi
    PG_BIN_DIR="${PG_BIN_DIR}/bin"
    print_info "PostgreSQL: $PG_BIN_DIR"
}

# -----------------------------------------------------------------------------
# Pre-init database
# -----------------------------------------------------------------------------
init_postgres() {
    if [ -f "${POSTGRES_DATA_DIR}/PG_VERSION" ]; then
        print_info "PostgreSQL 数据目录已存在"
        return
    fi
    print_info "初始化 PostgreSQL..."
    gosu postgres "${PG_BIN_DIR}/initdb" \
        -D "${POSTGRES_DATA_DIR}" \
        --username=postgres \
        --auth-local=trust \
        --auth-host=scram-sha-256 \
        --encoding=UTF8 \
        --locale=C.UTF-8 >/dev/null
    print_success "PostgreSQL 初始化完成"
}

# -----------------------------------------------------------------------------
# Create start.sh (foreground, Ctrl+C to stop)
# -----------------------------------------------------------------------------
create_start_script() {
    print_info "创建 start.sh..."
    cat > "${DEPLOY_DIR}/start.sh" <<'WRAPPER'
#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APP_ROOT:-/opt/sub2api}"
APP_DATA_DIR="${APP_DATA_DIR:-/var/lib/sub2api}"
APP_BINARY="${APP_BINARY:-${APP_ROOT}/sub2api}"
RUNTIME_ENV_FILE="${RUNTIME_ENV_FILE:-/etc/sub2api/runtime.env}"
DATA_DIR="${APP_DATA_DIR}"
RUNTIME_STATE_DIR="${RUNTIME_STATE_DIR:-${APP_DATA_DIR}/runtime}"

POSTGRES_DATA_DIR="${POSTGRES_DATA_DIR:-${APP_DATA_DIR}/postgres}"
POSTGRES_RUN_DIR="${POSTGRES_RUN_DIR:-${RUNTIME_STATE_DIR}/postgres}"
POSTGRES_LOG_FILE="${POSTGRES_LOG_FILE:-/var/log/sub2api/postgres.log}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-${APP_DATA_DIR}/redis}"
REDIS_RUN_DIR="${REDIS_RUN_DIR:-${RUNTIME_STATE_DIR}/redis}"
REDIS_CONF_FILE="${REDIS_CONF_FILE:-${RUNTIME_STATE_DIR}/redis.conf}"
REDIS_LOG_FILE="${REDIS_LOG_FILE:-/var/log/sub2api/redis.log}"
POSTGRES_HOST_BIND="${POSTGRES_HOST_BIND:-127.0.0.1}"
REDIS_HOST_BIND="${REDIS_HOST_BIND:-127.0.0.1}"
PG_BIN_DIR="${PG_BIN_DIR:-}"

APP_PID=""
_CLEANED_UP=0

log() { printf '[sub2api] %s\n' "$*"; }

random_hex() { openssl rand -hex "${1:-32}"; }

validate_pg_identifier() {
    local value="$1" label="$2"
    if [[ ! "${value}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
        printf 'Invalid %s: %s\n' "${label}" "${value}" >&2; exit 1
    fi
}

load_runtime_env() {
    if [ -f "${RUNTIME_ENV_FILE}" ]; then
        set -a && . "${RUNTIME_ENV_FILE}" && set +a
    fi
}

ensure_runtime_env_values() {
    DATABASE_HOST="127.0.0.1"
    REDIS_HOST="127.0.0.1"
    REDIS_ENABLE_TLS="false"
    if [ -z "${DATABASE_PASSWORD:-}" ]; then DATABASE_PASSWORD="$(random_hex 24)"; fi
    if [ -z "${REDIS_PASSWORD:-}" ]; then REDIS_PASSWORD="$(random_hex 24)"; fi
    if [ -z "${JWT_SECRET:-}" ]; then JWT_SECRET="$(random_hex 32)"; fi
    if [ -z "${TOTP_ENCRYPTION_KEY:-}" ]; then TOTP_ENCRYPTION_KEY="$(random_hex 32)"; fi
    if [ -z "${ADMIN_PASSWORD:-}" ]; then ADMIN_PASSWORD="$(random_hex 12)"; fi
}

write_runtime_env() {
    local tmp="${RUNTIME_ENV_FILE}.tmp"
    umask 077; : > "$tmp"
    while IFS= read -r name; do
        printf '%s=%q\n' "$name" "${!name}" >> "$tmp"
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
REDIS_HOST
REDIS_PORT
REDIS_PASSWORD
REDIS_DB
ADMIN_EMAIL
ADMIN_PASSWORD
JWT_SECRET
JWT_EXPIRE_HOUR
TOTP_ENCRYPTION_KEY
EOF
    mv "$tmp" "$RUNTIME_ENV_FILE"
    chmod 0600 "$RUNTIME_ENV_FILE"
}

prepare_dirs() {
    mkdir -p "${APP_DATA_DIR}" "${DATA_DIR}" "${RUNTIME_STATE_DIR}" \
        "${POSTGRES_DATA_DIR}" "${POSTGRES_RUN_DIR}" \
        "${REDIS_DATA_DIR}" "${REDIS_RUN_DIR}"
    touch "${POSTGRES_LOG_FILE}" "${REDIS_LOG_FILE}"
    chown sub2api:sub2api "${APP_DATA_DIR}" "${DATA_DIR}" "${RUNTIME_STATE_DIR}"
    chown -R postgres:postgres "${POSTGRES_DATA_DIR}" "${POSTGRES_RUN_DIR}" "${POSTGRES_LOG_FILE}"
    chown -R redis:redis "${REDIS_DATA_DIR}" "${REDIS_RUN_DIR}" "${REDIS_LOG_FILE}"
    chmod 0700 "${POSTGRES_DATA_DIR}" "${POSTGRES_RUN_DIR}"
    chmod 0750 "${REDIS_DATA_DIR}" "${REDIS_RUN_DIR}"
}

discover_pg_bin_dir() {
    if [ -n "${PG_BIN_DIR}" ] && [ -x "${PG_BIN_DIR}/pg_ctl" ]; then return; fi
    PG_BIN_DIR=$(find /usr/lib/postgresql -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1)
    if [ -z "${PG_BIN_DIR}" ]; then echo "PostgreSQL not found" >&2; exit 1; fi
    PG_BIN_DIR="${PG_BIN_DIR}/bin"
}

init_postgres() {
    [ -f "${POSTGRES_DATA_DIR}/PG_VERSION" ] && return
    log "Initializing PostgreSQL"
    gosu postgres "${PG_BIN_DIR}/initdb" \
        -D "${POSTGRES_DATA_DIR}" --username=postgres \
        --auth-local=trust --auth-host=scram-sha-256 \
        --encoding=UTF8 --locale=C.UTF-8 >/dev/null
}

start_postgres() {
    log "Starting PostgreSQL on ${POSTGRES_HOST_BIND}:${DATABASE_PORT:-5432}"
    gosu postgres "${PG_BIN_DIR}/pg_ctl" \
        -D "${POSTGRES_DATA_DIR}" -l "${POSTGRES_LOG_FILE}" -w start \
        -o "-c listen_addresses=${POSTGRES_HOST_BIND} -c port=${DATABASE_PORT:-5432} -c unix_socket_directories=${POSTGRES_RUN_DIR}" >/dev/null
}

ensure_postgres_db() {
    local escaped_password db_exists
    validate_pg_identifier "${DATABASE_USER:-sub2api}" "DATABASE_USER"
    validate_pg_identifier "${DATABASE_DBNAME:-sub2api}" "DATABASE_DBNAME"
    escaped_password="${DATABASE_PASSWORD//\'/\'\'}"
    gosu postgres psql -v ON_ERROR_STOP=1 -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT:-5432}" postgres <<SQL >/dev/null
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
    db_exists="$(gosu postgres psql -tA -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT:-5432}" postgres -c "SELECT 1 FROM pg_database WHERE datname = '${DATABASE_DBNAME}'")"
    if [ "${db_exists}" != "1" ]; then
        gosu postgres createdb -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT:-5432}" -O "${DATABASE_USER}" "${DATABASE_DBNAME}"
    else
        gosu postgres psql -v ON_ERROR_STOP=1 -h "${POSTGRES_RUN_DIR}" -p "${DATABASE_PORT:-5432}" postgres -c "ALTER DATABASE ${DATABASE_DBNAME} OWNER TO ${DATABASE_USER}" >/dev/null
    fi
}

write_redis_config() {
    cat > "${REDIS_CONF_FILE}" <<EOF
bind ${REDIS_HOST_BIND}
protected-mode yes
port ${REDIS_PORT:-6379}
dir ${REDIS_DATA_DIR}
pidfile ${REDIS_RUN_DIR}/redis.pid
daemonize yes
appendonly yes
appendfilename appendonly.aof
save 60 1
logfile ${REDIS_LOG_FILE}
EOF
    if [ -n "${REDIS_PASSWORD:-}" ]; then
        printf 'requirepass %s\n' "${REDIS_PASSWORD}" >> "${REDIS_CONF_FILE}"
    fi
    chown redis:redis "${REDIS_CONF_FILE}"
    chmod 0600 "${REDIS_CONF_FILE}"
}

start_redis() {
    local attempt
    log "Starting Redis on ${REDIS_HOST_BIND}:${REDIS_PORT:-6379}"
    gosu redis redis-server "${REDIS_CONF_FILE}"
    for attempt in $(seq 1 30); do
        if REDISCLI_AUTH="${REDIS_PASSWORD:-}" redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" ping >/dev/null 2>&1; then
            return
        fi
        sleep 1
    done
    echo "Redis failed to start" >&2; exit 1
}

stop_redis() {
    [ -f "${REDIS_RUN_DIR}/redis.pid" ] || return
    REDISCLI_AUTH="${REDIS_PASSWORD:-}" redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" shutdown >/dev/null 2>&1 || true
}

stop_postgres() {
    if [ -z "${PG_BIN_DIR}" ] || [ ! -f "${POSTGRES_DATA_DIR}/postmaster.pid" ]; then return; fi
    gosu postgres "${PG_BIN_DIR}/pg_ctl" -D "${POSTGRES_DATA_DIR}" -m fast -w stop >/dev/null 2>&1 || true
}

cleanup() {
    [ "${_CLEANED_UP}" -eq 1 ] && return
    _CLEANED_UP=1
    log "Shutting down..."
    if [ -n "${APP_PID}" ] && kill -0 "${APP_PID}" 2>/dev/null; then
        kill -TERM "${APP_PID}" 2>/dev/null || true
        wait "${APP_PID}" 2>/dev/null || true
    fi
    stop_redis
    stop_postgres
    log "Stopped"
}

start_app() {
    export AUTO_SETUP DATA_DIR TZ SERVER_HOST SERVER_PORT SERVER_MODE RUN_MODE
    export DATABASE_HOST DATABASE_PORT DATABASE_USER DATABASE_PASSWORD DATABASE_DBNAME DATABASE_SSLMODE
    export REDIS_HOST REDIS_PORT REDIS_PASSWORD REDIS_DB
    export ADMIN_EMAIL ADMIN_PASSWORD JWT_SECRET JWT_EXPIRE_HOUR TOTP_ENCRYPTION_KEY

    log "Starting Sub2API..."
    gosu sub2api "${APP_BINARY}" &
    APP_PID="$!"
    wait "${APP_PID}"
}

# Main
if [ "$(id -u)" != "0" ]; then
    echo "Must run as root to manage PostgreSQL and Redis" >&2
    exit 1
fi

load_runtime_env
ensure_runtime_env_values
prepare_dirs
write_runtime_env
discover_pg_bin_dir
init_postgres
start_postgres
ensure_postgres_db
write_redis_config
start_redis

trap cleanup EXIT INT TERM

start_app || true
cleanup
WRAPPER

    chmod +x "${DEPLOY_DIR}/start.sh"
    print_success "start.sh 创建完成"
}

# -----------------------------------------------------------------------------
# Create stop.sh
# -----------------------------------------------------------------------------
create_stop_script() {
    print_info "创建 stop.sh..."
    cat > "${DEPLOY_DIR}/stop.sh" <<'STOPPER'
#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APP_ROOT:-/opt/sub2api}"
APP_BINARY="${APP_BINARY:-${APP_ROOT}/sub2api}"
RUNTIME_ENV_FILE="${RUNTIME_ENV_FILE:-/etc/sub2api/runtime.env}"
POSTGRES_DATA_DIR="${POSTGRES_DATA_DIR:-/var/lib/sub2api/postgres}"
REDIS_RUN_DIR="${REDIS_RUN_DIR:-${RUNTIME_STATE_DIR}/redis}"
REDIS_CONF_FILE="${REDIS_CONF_FILE:-${RUNTIME_STATE_DIR}/redis.conf}"

PG_BIN_DIR="${PG_BIN_DIR:-}"
REDIS_PASSWORD=""

load_env() {
    if [ -f "${RUNTIME_ENV_FILE}" ]; then
        set -a && . "${RUNTIME_ENV_FILE}" && set +a
    fi
}

discover_pg_bin_dir() {
    if [ -n "${PG_BIN_DIR}" ] && [ -x "${PG_BIN_DIR}/pg_ctl" ]; then return; fi
    PG_BIN_DIR=$(find /usr/lib/postgresql -mindepth 1 -maxdepth 1 -type d | sort -V | tail -n 1)
    [ -z "${PG_BIN_DIR}" ] && return
    PG_BIN_DIR="${PG_BIN_DIR}/bin"
}

stop_app() {
    local pid
    pid=$(pgrep -f "^${APP_BINARY}" || true)
    if [ -n "${pid}" ]; then
        echo "Stopping Sub2API (PID: ${pid})..."
        kill -TERM "${pid}" 2>/dev/null || true
        for _ in $(seq 1 30); do
            if ! kill -0 "${pid}" 2>/dev/null; then break; fi
            sleep 1
        done
        if kill -0 "${pid}" 2>/dev/null; then
            echo "Force killing Sub2API..."
            kill -KILL "${pid}" 2>/dev/null || true
        fi
        echo "Sub2API stopped"
    else
        echo "Sub2API not running"
    fi
}

stop_redis() {
    if [ ! -f "${REDIS_RUN_DIR}/redis.pid" ]; then
        echo "Redis not running"
        return
    fi
    if [ -f "${RUNTIME_ENV_FILE}" ]; then
        REDIS_PASSWORD="${REDIS_PASSWORD:-}"
    fi
    echo "Stopping Redis..."
    REDISCLI_AUTH="${REDIS_PASSWORD}" redis-cli -h 127.0.0.1 -p 6379 shutdown >/dev/null 2>&1 || true
    echo "Redis stopped"
}

stop_postgres() {
    if [ -z "${PG_BIN_DIR}" ] || [ ! -f "${POSTGRES_DATA_DIR}/postmaster.pid" ]; then
        echo "PostgreSQL not running"
        return
    fi
    echo "Stopping PostgreSQL..."
    gosu postgres "${PG_BIN_DIR}/pg_ctl" -D "${POSTGRES_DATA_DIR}" -m fast -w stop >/dev/null 2>&1 || true
    echo "PostgreSQL stopped"
}

load_env
discover_pg_bin_dir
stop_app
stop_redis
stop_postgres
STOPPER

    chmod +x "${DEPLOY_DIR}/stop.sh"
    print_success "stop.sh 创建完成"
}

# -----------------------------------------------------------------------------
# Show completion
# -----------------------------------------------------------------------------
show_done() {
    local ip
    ip=$(curl -sL http://icanhazip.com 2>/dev/null || hostname -I | awk '{print $1}')
    echo ""
    echo -e "${GREEN}=============================================="
    echo "  Sub2API 部署完成！"
    echo "==============================================${NC}"
    echo ""
    echo "  启动:  sudo ${DEPLOY_DIR}/start.sh"
    echo "  停止:  sudo ${DEPLOY_DIR}/stop.sh"
    echo ""
    echo "  启动后访问: http://${ip}:${SERVER_PORT}"
    echo ""
    echo "  目录结构:"
    echo "    二进制:  ${DEPLOY_DIR}/sub2api"
    echo "    启动脚本: ${DEPLOY_DIR}/start.sh"
    echo "    数据目录: ${DATA_DIR}"
    echo "    配置文件: ${RUNTIME_ENV_FILE}"
    if [ "${BUILD_FROM_SOURCE:-0}" = "1" ]; then
        echo "    源码目录: ${SRC_DIR}"
    fi
    echo ""
    echo "  反复部署:"
    if [ "${BUILD_FROM_SOURCE:-0}" = "1" ]; then
        echo "    cd ${SRC_DIR} && git pull"
        echo "    sudo bash $0"
    else
        echo "    sudo bash $0"
    fi
    echo ""
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------
main() {
    echo ""
    echo -e "${CYAN}=============================================="
    echo "  Sub2API Ubuntu 原生部署 (无 Systemd)"
    echo "  仓库: ${REPO}"
    echo "==============================================${NC}"
    echo ""

    check_root
    check_os
    check_arch
    install_deps
    setup_dirs
    deploy_binary
    generate_secrets
    discover_pg
    init_postgres
    create_start_script
    create_stop_script
    show_done
}

main "$@"
