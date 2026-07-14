#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]REDACTED")" && pwd)"
ENV_FILE="${SUB2API_ENV_FILE:-${SCRIPT_DIRREDACTED/.envREDACTED"

STACK_LABEL_KEY="org.sub2api.stack"
STACK_LABEL_VALUE="apple-container"
NETWORK_NAME="sub2api-apple"
APP_CONTAINER="sub2api-apple"
POSTGRES_CONTAINER="sub2api-apple-postgres"
REDIS_CONTAINER="sub2api-apple-redis"
APP_VOLUME="sub2api-apple-data"
POSTGRES_VOLUME="sub2api-apple-postgres-data"
REDIS_VOLUME="sub2api-apple-redis-data"
PLATFORM="linux/arm64"

TEMP_DIR=""
LOCK_DIR="${TMPDIR:-/tmpREDACTED/sub2api-apple-container.lock"
LOCK_ACQUIRED=false

APP_IMAGE=""
POSTGRES_IMAGE=""
REDIS_IMAGE=""
BIND_HOST=""
HOST_PORT=""
ACCESS_HOST=""
POSTGRES_USER=""
POSTGRES_PASSWORD=""
POSTGRES_DB=""
REDIS_PASSWORD=""
TZ_VALUE=""
POSTGRES_ADDRESS=""
REDIS_ADDRESS=""
APP_ENV_FILE=""
POSTGRES_ENV_FILE=""
POSTGRES_PROBE_ENV_FILE=""
REDIS_ENV_FILE=""

info() {
    printf '[INFO] %s\n' "$*"
REDACTED

warn() {
    printf '[WARN] %s\n' "$*" >&2
REDACTED

die() {
    printf '[ERROR] %s\n' "$*" >&2
    exit 1
REDACTED

usage() {
    cat <<'EOF'
Usage: ./apple-container.sh <command> [options]

Commands:
  init                  Create .env and generate required secrets
  up [--recreate]       Create and start the complete Sub2API stack
  down                  Stop the stack and preserve all data
  restart               Restart the stack in dependency order
  status                Show container and workload health
  logs <service> [-f]   Show logs for app, postgres, or redis
  pull                  Pull all stack images for linux/arm64
  destroy [options]     Delete stack containers and network

Destroy options:
  --volumes             Also delete all persistent data volumes
  --yes                 Skip the confirmation prompt

Environment:
  SUB2API_ENV_FILE      Path to the deployment env file (default: deploy/.env)
EOF
REDACTED

cleanup() {
    local exit_code=$?

    if [[ -n "${TEMP_DIRREDACTED" && -d "${TEMP_DIRREDACTED" ]]; then
        rm -rf "${TEMP_DIRREDACTED"
    fi
    if [[ "${LOCK_ACQUIREDREDACTED" == true && -d "${LOCK_DIRREDACTED" ]]; then
        rm -f "${LOCK_DIRREDACTED/pid"
        rmdir "${LOCK_DIRREDACTED" 2>/dev/null || true
    fi

    exit "${exit_codeREDACTED"
REDACTED

acquire_lock() {
    if ! mkdir "${LOCK_DIRREDACTED" 2>/dev/null; then
        local owner_pid=""
        if [[ -f "${LOCK_DIRREDACTED/pid" ]]; then
            owner_pid="$(<"${LOCK_DIRREDACTED/pid")"
        fi
        if [[ "${owner_pidREDACTED" =~ ^[0-9]+$ ]] && ! kill -0 "${owner_pidREDACTED" 2>/dev/null; then
            rm -rf "${LOCK_DIRREDACTED"
            mkdir "${LOCK_DIRREDACTED" || die "Failed to reclaim stale operation lock."
        else
            die "Another Sub2API Apple container operation is already running."
        fi
    fi
    printf '%s\n' "$$" >"${LOCK_DIRREDACTED/pid"
    LOCK_ACQUIRED=true
    trap cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    trap 'exit 129' HUP
REDACTED

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
REDACTED

require_container_version() {
    local version_output major minor

    require_command container
    require_command plutil
    version_output="$(container --version)"
    if [[ ! "${version_outputREDACTED" =~ ([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
        die "Unable to parse Apple container version: ${version_outputREDACTED"
    fi

    major="${BASH_REMATCH[1]REDACTED"
    minor="${BASH_REMATCH[2]REDACTED"
    if (( major < 1 || (major == 1 && minor < 1) )); then
        die "Apple container 1.1.0 or newer is required; found ${version_outputREDACTED."
    fi
REDACTED

system_is_running() {
    container system status >/dev/null 2>&1
REDACTED

start_system() {
    if ! system_is_running; then
        info "Starting Apple container services..."
        container system start --enable-kernel-install
    fi
REDACTED

list_resource_ids() {
    case "$1" in
        container) container list --all --quiet ;;
        network) container network list --quiet ;;
        volume) container volume list --quiet ;;
        *) die "Unknown resource type: $1" ;;
    esac
REDACTED

resource_exists() {
    local resource_type=$1
    local resource_name=$2
    local output line

    if ! output="$(list_resource_ids "${resource_typeREDACTED")"; then
        die "Failed to list Apple container ${resource_typeREDACTED resources."
    fi

    while IFS= read -r line; do
        if [[ "${lineREDACTED" == "${resource_nameREDACTED" ]]; then
            return 0
        fi
    done <<<"${outputREDACTED"

    return 1
REDACTED

inspect_resource() {
    case "$1" in
        container) container inspect "$2" ;;
        network) container network inspect "$2" ;;
        volume) container volume inspect "$2" ;;
        *) die "Unknown resource type: $1" ;;
    esac
REDACTED

assert_resource_owned() {
    local resource_type=$1
    local resource_name=$2
    local inspection compact

    inspection="$(inspect_resource "${resource_typeREDACTED" "${resource_nameREDACTED" | \
        plutil -extract 0.configuration.labels json -o - -)" || \
        die "Failed to inspect ${resource_typeREDACTED ${resource_nameREDACTED."
    compact="$(printf '%s' "${inspectionREDACTED" | tr -d '[:space:]')"
    if [[ "${compactREDACTED" != *"\"${STACK_LABEL_KEYREDACTED\":\"${STACK_LABEL_VALUEREDACTED\""* ]]; then
        die "Refusing to manage existing ${resource_typeREDACTED '${resource_nameREDACTED' because it is not owned by this stack."
    fi
REDACTED

preflight_stack_ownership() {
    local resource_name

    for resource_name in "${APP_CONTAINERREDACTED" "${REDIS_CONTAINERREDACTED" "${POSTGRES_CONTAINERREDACTED"; do
        if resource_exists container "${resource_nameREDACTED"; then
            assert_resource_owned container "${resource_nameREDACTED"
        fi
    done
    if resource_exists network "${NETWORK_NAMEREDACTED"; then
        assert_resource_owned network "${NETWORK_NAMEREDACTED"
    fi
    for resource_name in "${APP_VOLUMEREDACTED" "${REDIS_VOLUMEREDACTED" "${POSTGRES_VOLUMEREDACTED"; do
        if resource_exists volume "${resource_nameREDACTED"; then
            assert_resource_owned volume "${resource_nameREDACTED"
        fi
    done
REDACTED

ensure_network() {
    if resource_exists network "${NETWORK_NAMEREDACTED"; then
        assert_resource_owned network "${NETWORK_NAMEREDACTED"
        return
    fi

    info "Creating network ${NETWORK_NAMEREDACTED..."
    container network create \
        --label "${STACK_LABEL_KEYREDACTED=${STACK_LABEL_VALUEREDACTED" \
        "${NETWORK_NAMEREDACTED" >/dev/null
REDACTED

ensure_volume() {
    local volume_name=$1

    if resource_exists volume "${volume_nameREDACTED"; then
        assert_resource_owned volume "${volume_nameREDACTED"
        return
    fi

    info "Creating volume ${volume_nameREDACTED..."
    container volume create \
        --label "${STACK_LABEL_KEYREDACTED=${STACK_LABEL_VALUEREDACTED" \
        "${volume_nameREDACTED" >/dev/null
REDACTED

ensure_image_available() {
    local image=$1

    if container image inspect "${imageREDACTED" >/dev/null 2>&1; then
        return
    fi
    info "Pulling ${imageREDACTED..."
    container image pull --platform "${PLATFORMREDACTED" "${imageREDACTED"
REDACTED

container_is_running() {
    local container_name=$1
    local output line

    output="$(container list --quiet)" || die "Failed to list running Apple containers."
    while IFS= read -r line; do
        if [[ "${lineREDACTED" == "${container_nameREDACTED" ]]; then
            return 0
        fi
    done <<<"${outputREDACTED"

    return 1
REDACTED

ensure_system() {
    require_container_version
    require_command curl
    start_system
REDACTED

container_ipv4_address() {
    local container_name=$1
    local address

    address="$(container inspect "${container_nameREDACTED" | \
        plutil -extract 0.status.networks.0.ipv4Address raw -o - -)" || \
        die "Unable to read the network address for ${container_nameREDACTED."
    address="${address%%/*REDACTED"
    [[ "${addressREDACTED" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || \
        die "Apple container returned an invalid IPv4 address for ${container_nameREDACTED: ${addressREDACTED"
    printf '%s\n' "${addressREDACTED"
REDACTED

read_env_value() {
    local key=$1
    local fallback=${2-REDACTED

    awk -v wanted="${keyREDACTED" -v fallback="${fallbackREDACTED" '
        BEGIN { found = 0 REDACTED
        /^[[:space:]]*#/ || /^[[:space:]]*$/ { next REDACTED
        {
            separator = index($0, "=")
            if (separator == 0) { next REDACTED
            key = substr($0, 1, separator - 1)
            if (key == wanted) {
                value = substr($0, separator + 1)
                sub(/\r$/, "", value)
                found = 1
            REDACTED
        REDACTED
        END {
            if (found) { print value REDACTED
            else { print fallback REDACTED
        REDACTED
    ' "${ENV_FILEREDACTED"
REDACTED

replace_env_value() {
    local key=$1
    local value=$2
    local target_file=${3:-${ENV_FILEREDACTEDREDACTED
    local temp_file="${target_fileREDACTED.tmp.$$"

    awk -v wanted="${keyREDACTED" -v replacement="${valueREDACTED" '
        BEGIN { replaced = 0 REDACTED
        {
            separator = index($0, "=")
            key = separator == 0 ? "" : substr($0, 1, separator - 1)
            if (key == wanted) {
                if (!replaced) { print wanted "=" replacement REDACTED
                replaced = 1
                next
            REDACTED
            print
        REDACTED
        END {
            if (!replaced) { print wanted "=" replacement REDACTED
        REDACTED
    ' "${target_fileREDACTED" >"${temp_fileREDACTED"
    chmod 600 "${temp_fileREDACTED"
    mv "${temp_fileREDACTED" "${target_fileREDACTED"
REDACTED

generate_secret() {
    openssl rand -hex 32
REDACTED

cmd_init() {
    local env_dir temp_file postgres_secret jwt_secret totp_secret

    require_command openssl

    if [[ -e "${ENV_FILEREDACTED" ]]; then
        die "Environment file already exists: ${ENV_FILEREDACTED"
    fi

    postgres_secret="$(generate_secret)" || die "Failed to generate PostgreSQL password."
    jwt_secret="$(generate_secret)" || die "Failed to generate JWT secret."
    totp_secret="$(generate_secret)" || die "Failed to generate TOTP encryption key."
    [[ -n "${postgres_secretREDACTED" && -n "${jwt_secretREDACTED" && -n "${totp_secretREDACTED" ]] || \
        die "Secret generation returned an empty value."

    env_dir="$(dirname "${ENV_FILEREDACTED")"
    temp_file="${ENV_FILEREDACTED.init.tmp.$$"
    mkdir -p "${env_dirREDACTED"
    cp "${SCRIPT_DIRREDACTED/.env.example" "${temp_fileREDACTED"
    chmod 600 "${temp_fileREDACTED"
    replace_env_value POSTGRES_PASSWORD "${postgres_secretREDACTED" "${temp_fileREDACTED"
    replace_env_value JWT_SECRET "${jwt_secretREDACTED" "${temp_fileREDACTED"
    replace_env_value TOTP_ENCRYPTION_KEY "${totp_secretREDACTED" "${temp_fileREDACTED"
    mv "${temp_fileREDACTED" "${ENV_FILEREDACTED"

    info "Created ${ENV_FILEREDACTED with generated secrets."
    info "Review the file, then run: SUB2API_ENV_FILE='${ENV_FILEREDACTED' ${SCRIPT_DIRREDACTED/apple-container.sh up"
REDACTED

validate_port() {
    local port=$1
    local decimal_port

    [[ "${portREDACTED" =~ ^[0-9]+$ ]] || die "SERVER_PORT must be numeric: ${portREDACTED"
    decimal_port=$((10#${portREDACTED))
    (( decimal_port >= 1025 && decimal_port <= 65535 )) || \
        die "SERVER_PORT must be between 1025 and 65535 for Apple container port forwarding."
REDACTED

validate_ipv4_address() {
    local address=$1
    local first second third fourth extra octet

    IFS=. read -r first second third fourth extra <<<"${addressREDACTED"
    [[ -n "${firstREDACTED" && -n "${secondREDACTED" && -n "${thirdREDACTED" && -n "${fourthREDACTED" && -z "${extraREDACTED" ]] || \
        die "BIND_HOST must be a valid IPv4 address: ${addressREDACTED"
    for octet in "${firstREDACTED" "${secondREDACTED" "${thirdREDACTED" "${fourthREDACTED"; do
        [[ "${octetREDACTED" =~ ^[0-9]+$ ]] || die "BIND_HOST must be a valid IPv4 address: ${addressREDACTED"
        (( 10#${octetREDACTED <= 255 )) || die "BIND_HOST must be a valid IPv4 address: ${addressREDACTED"
    done
REDACTED

validate_env_file_security() {
    local owner mode permissions

    [[ -f "${ENV_FILEREDACTED" ]] || die "Environment file not found: ${ENV_FILEREDACTED. Run '$0 init' first."
    owner="$(stat -f '%u' "${ENV_FILEREDACTED")" || die "Unable to read owner for ${ENV_FILEREDACTED."
    mode="$(stat -f '%Lp' "${ENV_FILEREDACTED")" || die "Unable to read permissions for ${ENV_FILEREDACTED."
    [[ "${ownerREDACTED" == "${EUIDREDACTED" ]] || die "Environment file must be owned by the current user: ${ENV_FILEREDACTED"
    [[ "${modeREDACTED" =~ ^[0-7]+$ ]] || die "Unable to parse permissions for ${ENV_FILEREDACTED: ${modeREDACTED"
    permissions=$((8#${modeREDACTED))
    (( (permissions & 077) == 0 )) || \
        die "Environment file must not be readable by group or others. Run: chmod 600 '${ENV_FILEREDACTED'"
REDACTED

prepare_environment() {
    validate_env_file_security

    APP_IMAGE="$(read_env_value APPLE_CONTAINER_SUB2API_IMAGE weishaw/sub2api:latest)"
    POSTGRES_IMAGE="$(read_env_value APPLE_CONTAINER_POSTGRES_IMAGE postgres:18-alpine)"
    REDIS_IMAGE="$(read_env_value APPLE_CONTAINER_REDIS_IMAGE redis:8-alpine)"
    BIND_HOST="$(read_env_value BIND_HOST 0.0.0.0)"
    HOST_PORT="$(read_env_value SERVER_PORT 8080)"
    POSTGRES_USER="$(read_env_value POSTGRES_USER sub2api)"
    POSTGRES_PASSWORD="$(read_env_value POSTGRES_PASSWORD)"
    POSTGRES_DB="$(read_env_value POSTGRES_DB sub2api)"
    REDIS_PASSWORD="$(read_env_value REDIS_PASSWORD)"
    TZ_VALUE="$(read_env_value TZ Asia/Shanghai)"

    [[ -n "${BIND_HOSTREDACTED" ]] || die "BIND_HOST must not be empty."
    validate_ipv4_address "${BIND_HOSTREDACTED"
    validate_port "${HOST_PORTREDACTED"
    if [[ "${BIND_HOSTREDACTED" == "0.0.0.0" ]]; then
        ACCESS_HOST="127.0.0.1"
    else
        ACCESS_HOST="${BIND_HOSTREDACTED"
    fi
    [[ -n "${POSTGRES_USERREDACTED" ]] || die "POSTGRES_USER must not be empty."
    [[ -n "${POSTGRES_DBREDACTED" ]] || die "POSTGRES_DB must not be empty."
    if [[ -z "${POSTGRES_PASSWORDREDACTED" || "${POSTGRES_PASSWORDREDACTED" == "change_this_secure_password" ]]; then
        die "Set a secure POSTGRES_PASSWORD in ${ENV_FILEREDACTED."
    fi

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmpREDACTED/sub2api-apple.XXXXXX")"
    APP_ENV_FILE="${TEMP_DIRREDACTED/app.env"
    POSTGRES_ENV_FILE="${TEMP_DIRREDACTED/postgres.env"
    POSTGRES_PROBE_ENV_FILE="${TEMP_DIRREDACTED/postgres-probe.env"
    REDIS_ENV_FILE="${TEMP_DIRREDACTED/redis.env"

    cat >"${POSTGRES_ENV_FILEREDACTED" <<EOF
POSTGRES_USER=${POSTGRES_USERREDACTED
POSTGRES_PASSWORD=${POSTGRES_PASSWORDREDACTED
POSTGRES_DB=${POSTGRES_DBREDACTED
TZ=${TZ_VALUEREDACTED
EOF

    cat >"${POSTGRES_PROBE_ENV_FILEREDACTED" <<EOF
PGPASSWORD=${POSTGRES_PASSWORDREDACTED
EOF

    cat >"${REDIS_ENV_FILEREDACTED" <<EOF
REDIS_PASSWORD=${REDIS_PASSWORDREDACTED
TZ=${TZ_VALUEREDACTED
EOF
    if [[ -n "${REDIS_PASSWORDREDACTED" ]]; then
        printf 'REDISCLI_AUTH=%s\n' "${REDIS_PASSWORDREDACTED" >>"${REDIS_ENV_FILEREDACTED"
    fi

    chmod 600 "${POSTGRES_ENV_FILEREDACTED" "${POSTGRES_PROBE_ENV_FILEREDACTED" "${REDIS_ENV_FILEREDACTED"
REDACTED

prepare_app_environment() {
    [[ -n "${POSTGRES_ADDRESSREDACTED" && -n "${REDIS_ADDRESSREDACTED" ]] || \
        die "Dependency network addresses are not available."

    cp "${ENV_FILEREDACTED" "${APP_ENV_FILEREDACTED"
    cat >>"${APP_ENV_FILEREDACTED" <<EOF

AUTO_SETUP=true
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
DATABASE_HOST=${POSTGRES_ADDRESSREDACTED
DATABASE_PORT=5432
DATABASE_USER=${POSTGRES_USERREDACTED
DATABASE_PASSWORD=${POSTGRES_PASSWORDREDACTED
DATABASE_DBNAME=${POSTGRES_DBREDACTED
DATABASE_SSLMODE=disable
REDIS_HOST=${REDIS_ADDRESSREDACTED
REDIS_PORT=6379
REDIS_PASSWORD=${REDIS_PASSWORDREDACTED
DATA_DIR=/app/storage/data
EOF
    chmod 600 "${APP_ENV_FILEREDACTED"
REDACTED

create_postgres_container() {
    info "Creating PostgreSQL container..."
    container create \
        --name "${POSTGRES_CONTAINERREDACTED" \
        --label "${STACK_LABEL_KEYREDACTED=${STACK_LABEL_VALUEREDACTED" \
        --network "${NETWORK_NAMEREDACTED" \
        --platform "${PLATFORMREDACTED" \
        --ulimit nofile=100000:100000 \
        --env-file "${POSTGRES_ENV_FILEREDACTED" \
        --volume "${POSTGRES_VOLUMEREDACTED:/var/lib/postgresql" \
        "${POSTGRES_IMAGEREDACTED" >/dev/null
REDACTED

create_redis_container() {
    info "Creating Redis container..."
    container create \
        --name "${REDIS_CONTAINERREDACTED" \
        --label "${STACK_LABEL_KEYREDACTED=${STACK_LABEL_VALUEREDACTED" \
        --network "${NETWORK_NAMEREDACTED" \
        --platform "${PLATFORMREDACTED" \
        --ulimit nofile=100000:100000 \
        --env-file "${REDIS_ENV_FILEREDACTED" \
        --volume "${REDIS_VOLUMEREDACTED:/var/lib/redis" \
        "${REDIS_IMAGEREDACTED" \
        sh -c 'set -e; mkdir -p /var/lib/redis/data; chown redis:redis /var/lib/redis/data; exec /usr/local/bin/docker-entrypoint.sh redis-server --dir /var/lib/redis/data --save 60 1 --appendonly yes --appendfsync everysec ${REDIS_PASSWORD:+--requirepass "$REDIS_PASSWORD"REDACTED' \
        >/dev/null
REDACTED

create_app_container() {
    info "Creating Sub2API container..."
    container create \
        --name "${APP_CONTAINERREDACTED" \
        --label "${STACK_LABEL_KEYREDACTED=${STACK_LABEL_VALUEREDACTED" \
        --network "${NETWORK_NAMEREDACTED" \
        --platform "${PLATFORMREDACTED" \
        --ulimit nofile=100000:100000 \
        --publish "${BIND_HOSTREDACTED:${HOST_PORTREDACTED:8080/tcp" \
        --env-file "${APP_ENV_FILEREDACTED" \
        --volume "${APP_VOLUMEREDACTED:/app/storage" \
        --entrypoint /bin/sh \
        "${APP_IMAGEREDACTED" \
        -c 'set -e; mkdir -p "$DATA_DIR"; chown -R sub2api:sub2api "$DATA_DIR"; exec su-exec sub2api /app/sub2api' \
        >/dev/null
REDACTED

ensure_container() {
    local container_name=$1
    local create_function=$2

    if resource_exists container "${container_nameREDACTED"; then
        assert_resource_owned container "${container_nameREDACTED"
        return
    fi

    "${create_functionREDACTED"
REDACTED

start_container_if_needed() {
    local container_name=$1

    if container_is_running "${container_nameREDACTED"; then
        return
    fi

    info "Starting ${container_nameREDACTED..."
    container start "${container_nameREDACTED" >/dev/null
REDACTED

stop_container_if_running() {
    local container_name=$1

    if ! resource_exists container "${container_nameREDACTED"; then
        return
    fi
    assert_resource_owned container "${container_nameREDACTED"
    if container_is_running "${container_nameREDACTED"; then
        info "Stopping ${container_nameREDACTED..."
        container stop --time 30 "${container_nameREDACTED" >/dev/null
    fi
REDACTED

delete_container_if_present() {
    local container_name=$1

    if ! resource_exists container "${container_nameREDACTED"; then
        return
    fi
    assert_resource_owned container "${container_nameREDACTED"
    if container_is_running "${container_nameREDACTED"; then
        container stop --time 30 "${container_nameREDACTED" >/dev/null
    fi
    info "Deleting ${container_nameREDACTED..."
    container delete "${container_nameREDACTED" >/dev/null
REDACTED

wait_for_probe() {
    local description=$1
    local attempts=$2
    shift 2

    local attempt
    for ((attempt = 1; attempt <= attempts; attempt++)); do
        if "$@" >/dev/null 2>&1; then
            info "${descriptionREDACTED is ready."
            return 0
        fi
        sleep 1
    done

    return 1
REDACTED

probe_postgres() {
    container exec --env-file "${POSTGRES_PROBE_ENV_FILEREDACTED" \
        "${POSTGRES_CONTAINERREDACTED" \
        psql -h 127.0.0.1 -U "${POSTGRES_USERREDACTED" -d "${POSTGRES_DBREDACTED" \
        -v ON_ERROR_STOP=1 -tAc 'SELECT 1'
REDACTED

probe_redis() {
    container exec --env-file "${REDIS_ENV_FILEREDACTED" \
        "${REDIS_CONTAINERREDACTED" \
        redis-cli ping
REDACTED

probe_app() {
    container exec "${APP_CONTAINERREDACTED" \
        wget -q -T 5 -O /dev/null http://localhost:8080/health
REDACTED

probe_host_app() {
    curl --fail --silent --show-error --max-time 5 \
        "http://${ACCESS_HOSTREDACTED:${HOST_PORTREDACTED/health"
REDACTED

show_failure_logs() {
    local container_name=$1

    warn "Last logs from ${container_nameREDACTED:"
    container logs -n 50 "${container_nameREDACTED" >&2 || true
REDACTED

start_dependencies() {
    start_container_if_needed "${POSTGRES_CONTAINERREDACTED"
    if ! wait_for_probe "PostgreSQL" 90 probe_postgres; then
        show_failure_logs "${POSTGRES_CONTAINERREDACTED"
        die "PostgreSQL did not become ready."
    fi

    start_container_if_needed "${REDIS_CONTAINERREDACTED"
    if ! wait_for_probe "Redis" 60 probe_redis; then
        show_failure_logs "${REDIS_CONTAINERREDACTED"
        die "Redis did not become ready."
    fi
REDACTED

start_app() {
    start_container_if_needed "${APP_CONTAINERREDACTED"
    if ! wait_for_probe "Sub2API" 180 probe_app; then
        show_failure_logs "${APP_CONTAINERREDACTED"
        die "Sub2API did not become ready."
    fi
    if ! wait_for_probe "Sub2API host port" 15 probe_host_app; then
        die "Host port forwarding failed. In System Settings > Privacy & Security > Local Network, allow container-runtime-linux; restart Apple container services; then run 'apple-container.sh up' again."
    fi
REDACTED

cmd_up() {
    local recreate=false

    if [[ $# -gt 1 || ($# -eq 1 && "${1-REDACTED" != "--recreate") ]]; then
        usage
        exit 2
    fi
    if [[ $# -eq 1 ]]; then
        recreate=true
    fi

    ensure_system
    prepare_environment
    preflight_stack_ownership
    ensure_network
    ensure_volume "${APP_VOLUMEREDACTED"
    ensure_volume "${POSTGRES_VOLUMEREDACTED"
    ensure_volume "${REDIS_VOLUMEREDACTED"
    ensure_image_available "${APP_IMAGEREDACTED"
    ensure_image_available "${POSTGRES_IMAGEREDACTED"
    ensure_image_available "${REDIS_IMAGEREDACTED"

    if [[ "${recreateREDACTED" == true ]]; then
        delete_container_if_present "${APP_CONTAINERREDACTED"
        delete_container_if_present "${REDIS_CONTAINERREDACTED"
        delete_container_if_present "${POSTGRES_CONTAINERREDACTED"
    fi

    ensure_container "${POSTGRES_CONTAINERREDACTED" create_postgres_container
    ensure_container "${REDIS_CONTAINERREDACTED" create_redis_container
    start_dependencies
    POSTGRES_ADDRESS="$(container_ipv4_address "${POSTGRES_CONTAINERREDACTED")"
    REDIS_ADDRESS="$(container_ipv4_address "${REDIS_CONTAINERREDACTED")"
    prepare_app_environment
    # The dependency IPs may change whenever their lightweight VMs restart.
    delete_container_if_present "${APP_CONTAINERREDACTED"
    create_app_container
    start_app

    info "Sub2API is available at http://${ACCESS_HOSTREDACTED:${HOST_PORTREDACTED"
REDACTED

cmd_down() {
    require_container_version
    if ! system_is_running; then
        info "Apple container services are already stopped."
        return
    fi
    preflight_stack_ownership
    stop_container_if_running "${APP_CONTAINERREDACTED"
    stop_container_if_running "${REDIS_CONTAINERREDACTED"
    stop_container_if_running "${POSTGRES_CONTAINERREDACTED"
    info "Sub2API stack stopped; persistent volumes were preserved."
REDACTED

cmd_restart() {
    cmd_down
    cmd_up
REDACTED

print_container_status() {
    local service=$1
    local container_name=$2

    if ! resource_exists container "${container_nameREDACTED"; then
        printf '%-12s %s\n' "${serviceREDACTED" "missing"
    elif container_is_running "${container_nameREDACTED"; then
        printf '%-12s %s\n' "${serviceREDACTED" "running"
    else
        printf '%-12s %s\n' "${serviceREDACTED" "stopped"
    fi
REDACTED

cmd_status() {
    local failed=0

    require_container_version
    if ! system_is_running; then
        printf '%-12s %s\n' "system" "stopped"
        return 1
    fi

    printf '%-12s %s\n' "system" "running"
    preflight_stack_ownership
    print_container_status app "${APP_CONTAINERREDACTED"
    print_container_status postgres "${POSTGRES_CONTAINERREDACTED"
    print_container_status redis "${REDIS_CONTAINERREDACTED"

    if [[ -f "${ENV_FILEREDACTED" ]]; then
        prepare_environment
        if container_is_running "${POSTGRES_CONTAINERREDACTED" && probe_postgres >/dev/null 2>&1; then
            printf '%-12s %s\n' "postgres" "healthy"
        else
            printf '%-12s %s\n' "postgres" "unhealthy"
            failed=1
        fi
        if container_is_running "${REDIS_CONTAINERREDACTED" && probe_redis >/dev/null 2>&1; then
            printf '%-12s %s\n' "redis" "healthy"
        else
            printf '%-12s %s\n' "redis" "unhealthy"
            failed=1
        fi
        if container_is_running "${APP_CONTAINERREDACTED" && probe_app >/dev/null 2>&1; then
            printf '%-12s %s\n' "app" "healthy"
        else
            printf '%-12s %s\n' "app" "unhealthy"
            failed=1
        fi
        if container_is_running "${APP_CONTAINERREDACTED" && probe_host_app >/dev/null 2>&1; then
            printf '%-12s %s\n' "host-port" "healthy"
        else
            printf '%-12s %s\n' "host-port" "unhealthy"
            failed=1
        fi
    else
        warn "Health probes require ${ENV_FILEREDACTED."
        failed=1
    fi

    return "${failedREDACTED"
REDACTED

cmd_logs() {
    local service=${1-REDACTED
    local follow=${2-REDACTED
    local container_name

    [[ $# -ge 1 && $# -le 2 ]] || { usage; exit 2; REDACTED
    if [[ -n "${followREDACTED" && "${followREDACTED" != "-f" && "${followREDACTED" != "--follow" ]]; then
        usage
        exit 2
    fi

    case "${serviceREDACTED" in
        app|sub2api) container_name="${APP_CONTAINERREDACTED" ;;
        postgres) container_name="${POSTGRES_CONTAINERREDACTED" ;;
        redis) container_name="${REDIS_CONTAINERREDACTED" ;;
        *) die "Unknown service '${serviceREDACTED'. Use app, postgres, or redis." ;;
    esac

    require_container_version
    system_is_running || die "Apple container services are not running."
    resource_exists container "${container_nameREDACTED" || die "Container not found: ${container_nameREDACTED"
    assert_resource_owned container "${container_nameREDACTED"
    if [[ -n "${followREDACTED" ]]; then
        container logs --follow "${container_nameREDACTED"
    else
        container logs "${container_nameREDACTED"
    fi
REDACTED

cmd_pull() {
    ensure_system
    prepare_environment
    info "Pulling ${APP_IMAGEREDACTED..."
    container image pull --platform "${PLATFORMREDACTED" "${APP_IMAGEREDACTED"
    info "Pulling ${POSTGRES_IMAGEREDACTED..."
    container image pull --platform "${PLATFORMREDACTED" "${POSTGRES_IMAGEREDACTED"
    info "Pulling ${REDIS_IMAGEREDACTED..."
    container image pull --platform "${PLATFORMREDACTED" "${REDIS_IMAGEREDACTED"
REDACTED

confirm_destroy() {
    local include_volumes=$1
    local answer

    if [[ "${include_volumesREDACTED" == true ]]; then
        printf 'Delete the Sub2API stack and all persistent data? [y/N] '
    else
        printf 'Delete the Sub2API containers and network, preserving volumes? [y/N] '
    fi
    read -r answer
    [[ "${answerREDACTED" == "y" || "${answerREDACTED" == "Y" ]]
REDACTED

delete_volume_if_present() {
    local volume_name=$1

    if resource_exists volume "${volume_nameREDACTED"; then
        assert_resource_owned volume "${volume_nameREDACTED"
        info "Deleting volume ${volume_nameREDACTED..."
        container volume delete "${volume_nameREDACTED" >/dev/null
    fi
REDACTED

cmd_destroy() {
    local include_volumes=false
    local assume_yes=false
    local argument

    for argument in "$@"; do
        case "${argumentREDACTED" in
            --volumes) include_volumes=true ;;
            --yes) assume_yes=true ;;
            *) usage; exit 2 ;;
        esac
    done

    require_container_version
    start_system
    preflight_stack_ownership
    if [[ "${assume_yesREDACTED" != true ]] && ! confirm_destroy "${include_volumesREDACTED"; then
        info "Cancelled."
        return
    fi

    delete_container_if_present "${APP_CONTAINERREDACTED"
    delete_container_if_present "${REDIS_CONTAINERREDACTED"
    delete_container_if_present "${POSTGRES_CONTAINERREDACTED"

    if resource_exists network "${NETWORK_NAMEREDACTED"; then
        assert_resource_owned network "${NETWORK_NAMEREDACTED"
        info "Deleting network ${NETWORK_NAMEREDACTED..."
        container network delete "${NETWORK_NAMEREDACTED" >/dev/null
    fi

    if [[ "${include_volumesREDACTED" == true ]]; then
        delete_volume_if_present "${APP_VOLUMEREDACTED"
        delete_volume_if_present "${REDIS_VOLUMEREDACTED"
        delete_volume_if_present "${POSTGRES_VOLUMEREDACTED"
        info "Sub2API stack and persistent data deleted."
    else
        info "Sub2API stack deleted; persistent volumes were preserved."
    fi
REDACTED

main() {
    local command=${1-REDACTED
    if [[ $# -gt 0 ]]; then
        shift
    fi

    case "${commandREDACTED" in
        init)
            [[ $# -eq 0 ]] || { usage; exit 2; REDACTED
            acquire_lock
            cmd_init
            ;;
        up)
            acquire_lock
            cmd_up "$@"
            ;;
        down)
            [[ $# -eq 0 ]] || { usage; exit 2; REDACTED
            acquire_lock
            cmd_down
            ;;
        restart)
            [[ $# -eq 0 ]] || { usage; exit 2; REDACTED
            acquire_lock
            cmd_restart
            ;;
        status)
            [[ $# -eq 0 ]] || { usage; exit 2; REDACTED
            trap cleanup EXIT
            cmd_status
            ;;
        logs)
            cmd_logs "$@"
            ;;
        pull)
            [[ $# -eq 0 ]] || { usage; exit 2; REDACTED
            acquire_lock
            cmd_pull
            ;;
        destroy)
            acquire_lock
            cmd_destroy "$@"
            ;;
        help|-h|--help)
            usage
            ;;
        *)
            usage
            exit 2
            ;;
    esac
REDACTED

main "$@"
