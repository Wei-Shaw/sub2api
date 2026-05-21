#!/bin/bash
# =============================================================================
# Sub2API LDAP Docker Bootstrap Script
# =============================================================================
# Run this in an empty target directory or an existing sub2api repo root.
# It will:
#   - download/extract the latest fork source snapshot when needed
#   - generate deploy/.env with secure defaults
#   - build the LDAP image locally
#   - start docker-compose.local.yml
# =============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TARGET_ROOT="$(pwd)"
REPO_ROOT=""
DEPLOY_DIR=""
COMPOSE_FILE="docker-compose.local.yml"
REPO_SNAPSHOT_URL="https://codeload.github.com/big-dimple/sub2api/tar.gz/refs/heads/main"

JWT_SECRET_VALUE=""
TOTP_ENCRYPTION_KEY_VALUE=""
POSTGRES_PASSWORD_VALUE=""
ADMIN_PASSWORD_VALUE=""
GENERATED_ENV=0

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

download_to_file() {
    local url="$1"
    local dest="$2"

    if command_exists curl; then
        curl -fsSL --retry 3 --connect-timeout 10 "$url" -o "$dest"
        return 0
    fi

    if command_exists wget; then
        wget -q --tries=3 --timeout=10 "$url" -O "$dest"
        return 0
    fi

    print_error "Neither curl nor wget is installed."
    exit 1
}

generate_secret() {
    openssl rand -hex 32
}

generate_admin_password() {
    openssl rand -base64 24 | tr -d '/+=' | cut -c1-20
}

read_env() {
    local env_file="$1"
    local key="$2"
    local default_value="$3"
    local value=""

    if [[ -f "$env_file" ]]; then
        value="$(grep -E "^${key}=" "$env_file" | tail -n 1 | cut -d'=' -f2- || true)"
        value="${value%$'\r'}"
        value="${value%\"}"
        value="${value#\"}"
        value="${value%\'}"
        value="${value#\'}"
    fi

    if [[ -z "$value" ]]; then
        echo "$default_value"
    else
        echo "$value"
    fi
}

ensure_repo_root() {
    if [[ -f "${TARGET_ROOT}/Dockerfile" && -f "${TARGET_ROOT}/deploy/${COMPOSE_FILE}" ]]; then
        REPO_ROOT="${TARGET_ROOT}"
        DEPLOY_DIR="${REPO_ROOT}/deploy"
        print_info "Using existing repository at ${REPO_ROOT}"
        return
    fi

    local existing
    existing="$(find "${TARGET_ROOT}" -mindepth 1 -maxdepth 1 ! -name '.git' ! -name 'docker-deploy.sh' -print -quit 2>/dev/null || true)"
    if [[ -n "$existing" ]]; then
        print_error "Run this bootstrap in an empty directory or an existing sub2api repo root."
        exit 1
    fi

    local tmpdir archive
    tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/sub2api-bootstrap.XXXXXX")"
    archive="${tmpdir}/sub2api-main.tar.gz"

    print_info "Downloading source snapshot from ${REPO_SNAPSHOT_URL} ..."
    download_to_file "${REPO_SNAPSHOT_URL}" "${archive}"

    print_info "Extracting source snapshot ..."
    tar -xzf "${archive}" -C "${TARGET_ROOT}" --strip-components=1
    rm -rf "${tmpdir}"

    REPO_ROOT="${TARGET_ROOT}"
    DEPLOY_DIR="${REPO_ROOT}/deploy"
    print_success "Source prepared in ${REPO_ROOT}"
}

init_directory_permissions() {
    cd "${DEPLOY_DIR}"
    chmod 775 data postgres_data redis_data 2>/dev/null || true

    if [[ "$(id -u)" -eq 0 ]]; then
        chown -R 1000:1000 data || true
        chown -R 70:70 postgres_data || true
        chown -R 999:1000 redis_data || true
        print_success "Initialized data directory ownership for container users"
    else
        print_warning "Running as non-root; skipped chown. If startup fails, fix directory ownership manually."
    fi
}

prepare_env_file() {
    cd "${DEPLOY_DIR}"

    if [[ -f ".env" ]]; then
        print_info "Using existing deploy/.env"
        ADMIN_PASSWORD_VALUE="$(read_env .env ADMIN_PASSWORD "")"
        return
    fi

    [[ -f ".env.example" ]] || {
        print_error "Missing ${DEPLOY_DIR}/.env.example"
        exit 1
    }

    JWT_SECRET_VALUE="$(generate_secret)"
    TOTP_ENCRYPTION_KEY_VALUE="$(generate_secret)"
    POSTGRES_PASSWORD_VALUE="$(generate_secret)"
    ADMIN_PASSWORD_VALUE="$(generate_admin_password)"

    cp .env.example .env

    if sed --version >/dev/null 2>&1; then
        sed -i "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET_VALUE}/" .env
        sed -i "s/^TOTP_ENCRYPTION_KEY=.*/TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY_VALUE}/" .env
        sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${POSTGRES_PASSWORD_VALUE}/" .env
        sed -i "s/^ADMIN_PASSWORD=.*/ADMIN_PASSWORD=${ADMIN_PASSWORD_VALUE}/" .env
    else
        sed -i '' "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET_VALUE}/" .env
        sed -i '' "s/^TOTP_ENCRYPTION_KEY=.*/TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY_VALUE}/" .env
        sed -i '' "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${POSTGRES_PASSWORD_VALUE}/" .env
        sed -i '' "s/^ADMIN_PASSWORD=.*/ADMIN_PASSWORD=${ADMIN_PASSWORD_VALUE}/" .env
    fi

    if ! grep -q "^ADMIN_PASSWORD=" .env; then
        echo "ADMIN_PASSWORD=${ADMIN_PASSWORD_VALUE}" >> .env
    fi

    chmod 600 .env
    GENERATED_ENV=1
    print_success "Generated deploy/.env with secure defaults"
}

probe_health() {
    local url="$1"
    if command_exists curl; then
        curl -fsS --max-time 3 "$url" >/dev/null
        return $?
    fi
    if command_exists wget; then
        wget -qO- --timeout=3 "$url" >/dev/null
        return $?
    fi
    return 1
}

wait_for_health() {
    local env_file="${DEPLOY_DIR}/.env"
    local configured_port deadline

    configured_port="$(read_env "${env_file}" SERVER_PORT "8080")"
    deadline=$((SECONDS + 120))

    while (( SECONDS < deadline )); do
        if probe_health "http://127.0.0.1:${configured_port}/health"; then
            print_success "Service is healthy on port ${configured_port}"
            return 0
        fi
        sleep 3
    done

    return 1
}

build_and_start() {
    mkdir -p "${DEPLOY_DIR}/data" "${DEPLOY_DIR}/postgres_data" "${DEPLOY_DIR}/redis_data"
    init_directory_permissions

    print_info "Building LDAP image ..."
    docker build -t weishaw/sub2api:latest "${REPO_ROOT}"

    print_info "Starting containers ..."
    cd "${DEPLOY_DIR}"
    docker compose -f "${COMPOSE_FILE}" up -d
}

run_self_check_snapshot() {
    cd "${DEPLOY_DIR}"
    print_info "docker compose status:"
    docker compose -f "${COMPOSE_FILE}" ps || true

    if wait_for_health; then
        return 0
    fi

    print_warning "Health check did not pass in time. Recent logs:"
    docker compose -f "${COMPOSE_FILE}" logs --tail=120 sub2api || true
}

main() {
    echo ""
    echo "=========================================="
    echo "  Sub2API LDAP Bootstrap"
    echo "=========================================="
    echo ""

    command_exists openssl || {
        print_error "openssl is not installed."
        exit 1
    }
    command_exists tar || {
        print_error "tar is not installed."
        exit 1
    }
    command_exists docker || {
        print_error "docker is not installed."
        exit 1
    }

    ensure_repo_root
    prepare_env_file
    build_and_start
    run_self_check_snapshot

    echo ""
    echo "=========================================="
    echo "  Done"
    echo "=========================================="
    echo ""
    echo "Repo: ${REPO_ROOT}"
    echo "Deploy dir: ${DEPLOY_DIR}"
    echo "Update script: ${DEPLOY_DIR}/upgrade_main.sh"
    if [[ "${GENERATED_ENV}" -eq 1 ]]; then
        echo "Admin password: ${ADMIN_PASSWORD_VALUE}"
    else
        echo "Admin password: $(read_env "${DEPLOY_DIR}/.env" ADMIN_PASSWORD "<set in deploy/.env>")"
    fi
    echo ""
    echo "Upgrade:"
    echo "  cd ${DEPLOY_DIR} && curl -fsSLo upgrade_main.sh https://raw.githubusercontent.com/big-dimple/sub2api/main/deploy/upgrade_main.sh && bash upgrade_main.sh"
}

main "$@"
