#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SUB2API_ENV_FILE:-${SCRIPT_DIR}/.env}"

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
LOCK_DIR="${TMPDIR:-/tmp}/sub2api-apple-container.lock"
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
}

warn() {
    printf '[WARN] %s\n' "$*" >&2
}

die() {
    printf '[ERROR] %s\n' "$*" >&2
    exit 1
}

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
}

cleanup() {
    local exit_code=$?

    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
    if [[ "${LOCK_ACQUIRED}" == true && -d "${LOCK_DIR}" ]]; then
        rm -f "${LOCK_DIR}/pid"
        rmdir "${LOCK_DIR}" 2>/dev/null || true
    fi

    exit "${exit_code}"
}

acquire_lock() {
    if ! mkdir "${LOCK_DIR}" 2>/dev/null; then
        local owner_pid=""
        if [[ -f "${LOCK_DIR}/pid" ]]; then
            owner_pid="$(<"${LOCK_DIR}/pid")"
        fi
        if [[ "${owner_pid}" =~ ^[0-9]+$ ]] && ! kill -0 "${owner_pid}" 2>/dev/null; then
            rm -rf "${LOCK_DIR}"
            mkdir "${LOCK_DIR}" || die "Failed to reclaim stale operation lock."
        else
            die "Another Sub2API Apple container operation is already running."
        fi
    fi
    printf '%s\n' "$$" >"${LOCK_DIR}/pid"
    LOCK_ACQUIRED=true
    trap cleanup EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    trap 'exit 129' HUP
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || die "Required command not found: $1"
}

require_container_version() {
    local version_output major minor

    require_command container
    require_command plutil
    version_output="$(container --version)"
    if [[ ! "${version_output}" =~ ([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
        die "Unable to parse Apple container version: ${version_output}"
    fi

    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    if (( major < 1 || (major == 1 && minor < 1) )); then
        die "Apple container 1.1.0 or newer is required; found ${version_output}."
    fi
}

system_is_running() {
    container system status >/dev/null 2>&1
}

start_system() {
    if ! system_is_running; then
        info "Starting Apple container services..."
        container system start --enable-kernel-install
    fi
}

list_resource_ids() {
    case "$1" in
        container) container list --all --quiet ;;
        network) container network list --quiet ;;
        volume) container volume list --quiet ;;
        *) die "Unknown resource type: $1" ;;
    esac
}

resource_exists() {
    local resource_type=$1
    local resource_name=$2
    local output line

    if ! output="$(list_resource_ids "${resource_type}")"; then
        die "Failed to list Apple container ${resource_type} resources."
    fi

    while IFS= read -r line; do
        if [[ "${line}" == "${resource_name}" ]]; then
            return 0
        fi
    done <<<"${output}"

    return 1
}

inspect_resource() {
    case "$1" in
        container) container inspect "$2" ;;
        network) container network inspect "$2" ;;
        volume) container volume inspect "$2" ;;
        *) die "Unknown resource type: $1" ;;
    esac
}

assert_resource_owned() {
    local resource_type=$1
    local resource_name=$2
    local inspection compact

    inspection="$(inspect_resource "${resource_type}" "${resource_name}" | \
        plutil -extract 0.configuration.labels json -o - -)" || \
        die "Failed to inspect ${resource_type} ${resource_name}."
    compact="$(printf '%s' "${inspection}" | tr -d '[:space:]')"
    if [[ "${compact}" != *"\"${STACK_LABEL_KEY}\":\"${STACK_LABEL_VALUE}\""* ]]; then
        die "Refusing to manage existing ${resource_type} '${resource_name}' because it is not owned by this stack."
    fi
}

preflight_stack_ownership() {
    local resource_name

    for resource_name in "${APP_CONTAINER}" "${REDIS_CONTAINER}" "${POSTGRES_CONTAINER}"; do
        if resource_exists container "${resource_name}"; then
            assert_resource_owned container "${resource_name}"
        fi
    done
    if resource_exists network "${NETWORK_NAME}"; then
        assert_resource_owned network "${NETWORK_NAME}"
    fi
    for resource_name in "${APP_VOLUME}" "${REDIS_VOLUME}" "${POSTGRES_VOLUME}"; do
        if resource_exists volume "${resource_name}"; then
            assert_resource_owned volume "${resource_name}"
        fi
    done
}

ensure_network() {
    if resource_exists network "${NETWORK_NAME}"; then
        assert_resource_owned network "${NETWORK_NAME}"
        return
    fi

    info "Creating network ${NETWORK_NAME}..."
    container network create \
        --label "${STACK_LABEL_KEY}=${STACK_LABEL_VALUE}" \
        "${NETWORK_NAME}" >/dev/null
}

ensure_volume() {
    local volume_name=$1

    if resource_exists volume "${volume_name}"; then
        assert_resource_owned volume "${volume_name}"
        return
    fi

    info "Creating volume ${volume_name}..."
    container volume create \
        --label "${STACK_LABEL_KEY}=${STACK_LABEL_VALUE}" \
        "${volume_name}" >/dev/null
}

ensure_image_available() {
    local image=$1

    if container image inspect "${image}" >/dev/null 2>&1; then
        return
    fi
    info "Pulling ${image}..."
    container image pull --platform "${PLATFORM}" "${image}"
}

container_is_running() {
    local container_name=$1
    local output line

    output="$(container list --quiet)" || die "Failed to list running Apple containers."
    while IFS= read -r line; do
        if [[ "${line}" == "${container_name}" ]]; then
            return 0
        fi
    done <<<"${output}"

    return 1
}

ensure_system() {
    require_container_version
    require_command curl
    start_system
}

container_ipv4_address() {
    local container_name=$1
    local address

    address="$(container inspect "${container_name}" | \
        plutil -extract 0.status.networks.0.ipv4Address raw -o - -)" || \
        die "Unable to read the network address for ${container_name}."
    address="${address%%/*}"
    [[ "${address}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || \
        die "Apple container returned an invalid IPv4 address for ${container_name}: ${address}"
    printf '%s\n' "${address}"
}

read_env_value() {
    local key=$1
    local fallback=${2-}

    awk -v wanted="${key}" -v fallback="${fallback}" '
        BEGIN { found = 0 }
        /^[[:space:]]*#/ || /^[[:space:]]*$/ { next }
        {
            separator = index($0, "=")
            if (separator == 0) { next }
            key = substr($0, 1, separator - 1)
            if (key == wanted) {
                value = substr($0, separator + 1)
                sub(/\r$/, "", value)
                found = 1
            }
        }
        END {
            if (found) { print value }
            else { print fallback }
        }
    ' "${ENV_FILE}"
}

replace_env_value() {
    local key=$1
    local value=$2
    local target_file=${3:-${ENV_FILE}}
    local temp_file="${target_file}.tmp.$$"

    awk -v wanted="${key}" -v replacement="${value}" '
        BEGIN { replaced = 0 }
        {
            separator = index($0, "=")
            key = separator == 0 ? "" : substr($0, 1, separator - 1)
            if (key == wanted) {
                if (!replaced) { print wanted "=" replacement }
                replaced = 1
                next
            }
            print
        }
        END {
            if (!replaced) { print wanted "=" replacement }
        }
    ' "${target_file}" >"${temp_file}"
    chmod 600 "${temp_file}"
    mv "${temp_file}" "${target_file}"
}

generate_secret() {
    openssl rand -hex 32
}

cmd_init() {
    local env_dir temp_file postgres_secret jwt_secret totp_secret

    require_command openssl

    if [[ -e "${ENV_FILE}" ]]; then
        die "Environment file already exists: ${ENV_FILE}"
    fi

    postgres_secret="$(generate_secret)" || die "Failed to generate PostgreSQL password."
    jwt_secret="$(generate_secret)" || die "Failed to generate JWT secret."
    totp_secret="$(generate_secret)" || die "Failed to generate TOTP encryption key."
    [[ -n "${postgres_secret}" && -n "${jwt_secret}" && -n "${totp_secret}" ]] || \
        die "Secret generation returned an empty value."

    env_dir="$(dirname "${ENV_FILE}")"
    temp_file="${ENV_FILE}.init.tmp.$$"
    mkdir -p "${env_dir}"
    cp "${SCRIPT_DIR}/.env.example" "${temp_file}"
    chmod 600 "${temp_file}"
    replace_env_value POSTGRES_PASSWORD "${postgres_secret}" "${temp_file}"
    replace_env_value JWT_SECRET "${jwt_secret}" "${temp_file}"
    replace_env_value TOTP_ENCRYPTION_KEY "${totp_secret}" "${temp_file}"
    mv "${temp_file}" "${ENV_FILE}"

    info "Created ${ENV_FILE} with generated secrets."
    info "Review the file, then run: SUB2API_ENV_FILE='${ENV_FILE}' ${SCRIPT_DIR}/apple-container.sh up"
}

validate_port() {
    local port=$1
    local decimal_port

    [[ "${port}" =~ ^[0-9]+$ ]] || die "SERVER_PORT must be numeric: ${port}"
    decimal_port=$((10#${port}))
    (( decimal_port >= 1025 && decimal_port <= 65535 )) || \
        die "SERVER_PORT must be between 1025 and 65535 for Apple container port forwarding."
}

validate_ipv4_address() {
    local address=$1
    local first second third fourth extra octet

    IFS=. read -r first second third fourth extra <<<"${address}"
    [[ -n "${first}" && -n "${second}" && -n "${third}" && -n "${fourth}" && -z "${extra}" ]] || \
        die "BIND_HOST must be a valid IPv4 address: ${address}"
    for octet in "${first}" "${second}" "${third}" "${fourth}"; do
        [[ "${octet}" =~ ^[0-9]+$ ]] || die "BIND_HOST must be a valid IPv4 address: ${address}"
        (( 10#${octet} <= 255 )) || die "BIND_HOST must be a valid IPv4 address: ${address}"
    done
}

validate_env_file_security() {
    local owner mode permissions

    [[ -f "${ENV_FILE}" ]] || die "Environment file not found: ${ENV_FILE}. Run '$0 init' first."
    owner="$(stat -f '%u' "${ENV_FILE}")" || die "Unable to read owner for ${ENV_FILE}."
    mode="$(stat -f '%Lp' "${ENV_FILE}")" || die "Unable to read permissions for ${ENV_FILE}."
    [[ "${owner}" == "${EUID}" ]] || die "Environment file must be owned by the current user: ${ENV_FILE}"
    [[ "${mode}" =~ ^[0-7]+$ ]] || die "Unable to parse permissions for ${ENV_FILE}: ${mode}"
    permissions=$((8#${mode}))
    (( (permissions & 077) == 0 )) || \
        die "Environment file must not be readable by group or others. Run: chmod 600 '${ENV_FILE}'"
}

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

    [[ -n "${BIND_HOST}" ]] || die "BIND_HOST must not be empty."
    validate_ipv4_address "${BIND_HOST}"
    validate_port "${HOST_PORT}"
    if [[ "${BIND_HOST}" == "0.0.0.0" ]]; then
        ACCESS_HOST="127.0.0.1"
    else
        ACCESS_HOST="${BIND_HOST}"
    fi
    [[ -n "${POSTGRES_USER}" ]] || die "POSTGRES_USER must not be empty."
    [[ -n "${POSTGRES_DB}" ]] || die "POSTGRES_DB must not be empty."
    if [[ -z "${POSTGRES_PASSWORD}" || "${POSTGRES_PASSWORD}" == "change_this_secure_password" ]]; then
        die "Set a secure POSTGRES_PASSWORD in ${ENV_FILE}."
    fi

    TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/sub2api-apple.XXXXXX")"
    APP_ENV_FILE="${TEMP_DIR}/app.env"
    POSTGRES_ENV_FILE="${TEMP_DIR}/postgres.env"
    POSTGRES_PROBE_ENV_FILE="${TEMP_DIR}/postgres-probe.env"
    REDIS_ENV_FILE="${TEMP_DIR}/redis.env"

    cat >"${POSTGRES_ENV_FILE}" <<EOF
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=${POSTGRES_DB}
TZ=${TZ_VALUE}
EOF

    cat >"${POSTGRES_PROBE_ENV_FILE}" <<EOF
PGPASSWORD=${POSTGRES_PASSWORD}
EOF

    cat >"${REDIS_ENV_FILE}" <<EOF
REDIS_PASSWORD=${REDIS_PASSWORD}
TZ=${TZ_VALUE}
EOF
    if [[ -n "${REDIS_PASSWORD}" ]]; then
        printf 'REDISCLI_AUTH=%s\n' "${REDIS_PASSWORD}" >>"${REDIS_ENV_FILE}"
    fi

    chmod 600 "${POSTGRES_ENV_FILE}" "${POSTGRES_PROBE_ENV_FILE}" "${REDIS_ENV_FILE}"
}

prepare_app_environment() {
    [[ -n "${POSTGRES_ADDRESS}" && -n "${REDIS_ADDRESS}" ]] || \
        die "Dependency network addresses are not available."

    cp "${ENV_FILE}" "${APP_ENV_FILE}"
    cat >>"${APP_ENV_FILE}" <<EOF

AUTO_SETUP=true
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
DATABASE_HOST=${POSTGRES_ADDRESS}
DATABASE_PORT=5432
DATABASE_USER=${POSTGRES_USER}
DATABASE_PASSWORD=${POSTGRES_PASSWORD}
DATABASE_DBNAME=${POSTGRES_DB}
DATABASE_SSLMODE=disable
REDIS_HOST=${REDIS_ADDRESS}
REDIS_PORT=6379
REDIS_PASSWORD=${REDIS_PASSWORD}
DATA_DIR=/app/storage/data
EOF
    chmod 600 "${APP_ENV_FILE}"
}

create_postgres_container() {
    info "Creating PostgreSQL container..."
    container create \
        --name "${POSTGRES_CONTAINER}" \
        --label "${STACK_LABEL_KEY}=${STACK_LABEL_VALUE}" \
        --network "${NETWORK_NAME}" \
        --platform "${PLATFORM}" \
        --ulimit nofile=100000:100000 \
        --env-file "${POSTGRES_ENV_FILE}" \
        --volume "${POSTGRES_VOLUME}:/var/lib/postgresql" \
        "${POSTGRES_IMAGE}" >/dev/null
}

create_redis_container() {
    info "Creating Redis container..."
    container create \
        --name "${REDIS_CONTAINER}" \
        --label "${STACK_LABEL_KEY}=${STACK_LABEL_VALUE}" \
        --network "${NETWORK_NAME}" \
        --platform "${PLATFORM}" \
        --ulimit nofile=100000:100000 \
        --env-file "${REDIS_ENV_FILE}" \
        --volume "${REDIS_VOLUME}:/var/lib/redis" \
        "${REDIS_IMAGE}" \
        sh -c 'set -e; mkdir -p /var/lib/redis/data; chown redis:redis /var/lib/redis/data; exec /usr/local/bin/docker-entrypoint.sh redis-server --dir /var/lib/redis/data --save 60 1 --appendonly yes --appendfsync everysec ${REDIS_PASSWORD:+--requirepass "$REDIS_PASSWORD"}' \
        >/dev/null
}

create_app_container() {
    info "Creating Sub2API container..."
    container create \
        --name "${APP_CONTAINER}" \
        --label "${STACK_LABEL_KEY}=${STACK_LABEL_VALUE}" \
        --network "${NETWORK_NAME}" \
        --platform "${PLATFORM}" \
        --ulimit nofile=100000:100000 \
        --publish "${BIND_HOST}:${HOST_PORT}:8080/tcp" \
        --env-file "${APP_ENV_FILE}" \
        --volume "${APP_VOLUME}:/app/storage" \
        --entrypoint /bin/sh \
        "${APP_IMAGE}" \
        -c 'set -e; mkdir -p "$DATA_DIR"; chown -R sub2api:sub2api "$DATA_DIR"; exec su-exec sub2api /app/sub2api' \
        >/dev/null
}

ensure_container() {
    local container_name=$1
    local create_function=$2

    if resource_exists container "${container_name}"; then
        assert_resource_owned container "${container_name}"
        return
    fi

    "${create_function}"
}

start_container_if_needed() {
    local container_name=$1

    if container_is_running "${container_name}"; then
        return
    fi

    info "Starting ${container_name}..."
    container start "${container_name}" >/dev/null
}

stop_container_if_running() {
    local container_name=$1

    if ! resource_exists container "${container_name}"; then
        return
    fi
    assert_resource_owned container "${container_name}"
    if container_is_running "${container_name}"; then
        info "Stopping ${container_name}..."
        container stop --time 30 "${container_name}" >/dev/null
    fi
}

delete_container_if_present() {
    local container_name=$1

    if ! resource_exists container "${container_name}"; then
        return
    fi
    assert_resource_owned container "${container_name}"
    if container_is_running "${container_name}"; then
        container stop --time 30 "${container_name}" >/dev/null
    fi
    info "Deleting ${container_name}..."
    container delete "${container_name}" >/dev/null
}

wait_for_probe() {
    local description=$1
    local attempts=$2
    shift 2

    local attempt
    for ((attempt = 1; attempt <= attempts; attempt++)); do
        if "$@" >/dev/null 2>&1; then
            info "${description} is ready."
            return 0
        fi
        sleep 1
    done

    return 1
}

probe_postgres() {
    container exec --env-file "${POSTGRES_PROBE_ENV_FILE}" \
        "${POSTGRES_CONTAINER}" \
        psql -h 127.0.0.1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" \
        -v ON_ERROR_STOP=1 -tAc 'SELECT 1'
}

probe_redis() {
    container exec --env-file "${REDIS_ENV_FILE}" \
        "${REDIS_CONTAINER}" \
        redis-cli ping
}

probe_app() {
    container exec "${APP_CONTAINER}" \
        wget -q -T 5 -O /dev/null http://localhost:8080/health
}

probe_host_app() {
    curl --fail --silent --show-error --max-time 5 \
        "http://${ACCESS_HOST}:${HOST_PORT}/health"
}

show_failure_logs() {
    local container_name=$1

    warn "Last logs from ${container_name}:"
    container logs -n 50 "${container_name}" >&2 || true
}

start_dependencies() {
    start_container_if_needed "${POSTGRES_CONTAINER}"
    if ! wait_for_probe "PostgreSQL" 90 probe_postgres; then
        show_failure_logs "${POSTGRES_CONTAINER}"
        die "PostgreSQL did not become ready."
    fi

    start_container_if_needed "${REDIS_CONTAINER}"
    if ! wait_for_probe "Redis" 60 probe_redis; then
        show_failure_logs "${REDIS_CONTAINER}"
        die "Redis did not become ready."
    fi
}

start_app() {
    start_container_if_needed "${APP_CONTAINER}"
    if ! wait_for_probe "Sub2API" 180 probe_app; then
        show_failure_logs "${APP_CONTAINER}"
        die "Sub2API did not become ready."
    fi
    if ! wait_for_probe "Sub2API host port" 15 probe_host_app; then
        die "Host port forwarding failed. In System Settings > Privacy & Security > Local Network, allow container-runtime-linux; restart Apple container services; then run 'apple-container.sh up' again."
    fi
}

cmd_up() {
    local recreate=false

    if [[ $# -gt 1 || ($# -eq 1 && "${1-}" != "--recreate") ]]; then
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
    ensure_volume "${APP_VOLUME}"
    ensure_volume "${POSTGRES_VOLUME}"
    ensure_volume "${REDIS_VOLUME}"
    ensure_image_available "${APP_IMAGE}"
    ensure_image_available "${POSTGRES_IMAGE}"
    ensure_image_available "${REDIS_IMAGE}"

    if [[ "${recreate}" == true ]]; then
        delete_container_if_present "${APP_CONTAINER}"
        delete_container_if_present "${REDIS_CONTAINER}"
        delete_container_if_present "${POSTGRES_CONTAINER}"
    fi

    ensure_container "${POSTGRES_CONTAINER}" create_postgres_container
    ensure_container "${REDIS_CONTAINER}" create_redis_container
    start_dependencies
    POSTGRES_ADDRESS="$(container_ipv4_address "${POSTGRES_CONTAINER}")"
    REDIS_ADDRESS="$(container_ipv4_address "${REDIS_CONTAINER}")"
    prepare_app_environment
    # The dependency IPs may change whenever their lightweight VMs restart.
    delete_container_if_present "${APP_CONTAINER}"
    create_app_container
    start_app

    info "Sub2API is available at http://${ACCESS_HOST}:${HOST_PORT}"
}

cmd_down() {
    require_container_version
    if ! system_is_running; then
        info "Apple container services are already stopped."
        return
    fi
    preflight_stack_ownership
    stop_container_if_running "${APP_CONTAINER}"
    stop_container_if_running "${REDIS_CONTAINER}"
    stop_container_if_running "${POSTGRES_CONTAINER}"
    info "Sub2API stack stopped; persistent volumes were preserved."
}

cmd_restart() {
    cmd_down
    cmd_up
}

print_container_status() {
    local service=$1
    local container_name=$2

    if ! resource_exists container "${container_name}"; then
        printf '%-12s %s\n' "${service}" "missing"
    elif container_is_running "${container_name}"; then
        printf '%-12s %s\n' "${service}" "running"
    else
        printf '%-12s %s\n' "${service}" "stopped"
    fi
}

cmd_status() {
    local failed=0

    require_container_version
    if ! system_is_running; then
        printf '%-12s %s\n' "system" "stopped"
        return 1
    fi

    printf '%-12s %s\n' "system" "running"
    preflight_stack_ownership
    print_container_status app "${APP_CONTAINER}"
    print_container_status postgres "${POSTGRES_CONTAINER}"
    print_container_status redis "${REDIS_CONTAINER}"

    if [[ -f "${ENV_FILE}" ]]; then
        prepare_environment
        if container_is_running "${POSTGRES_CONTAINER}" && probe_postgres >/dev/null 2>&1; then
            printf '%-12s %s\n' "postgres" "healthy"
        else
            printf '%-12s %s\n' "postgres" "unhealthy"
            failed=1
        fi
        if container_is_running "${REDIS_CONTAINER}" && probe_redis >/dev/null 2>&1; then
            printf '%-12s %s\n' "redis" "healthy"
        else
            printf '%-12s %s\n' "redis" "unhealthy"
            failed=1
        fi
        if container_is_running "${APP_CONTAINER}" && probe_app >/dev/null 2>&1; then
            printf '%-12s %s\n' "app" "healthy"
        else
            printf '%-12s %s\n' "app" "unhealthy"
            failed=1
        fi
        if container_is_running "${APP_CONTAINER}" && probe_host_app >/dev/null 2>&1; then
            printf '%-12s %s\n' "host-port" "healthy"
        else
            printf '%-12s %s\n' "host-port" "unhealthy"
            failed=1
        fi
    else
        warn "Health probes require ${ENV_FILE}."
        failed=1
    fi

    return "${failed}"
}

cmd_logs() {
    local service=${1-}
    local follow=${2-}
    local container_name

    [[ $# -ge 1 && $# -le 2 ]] || { usage; exit 2; }
    if [[ -n "${follow}" && "${follow}" != "-f" && "${follow}" != "--follow" ]]; then
        usage
        exit 2
    fi

    case "${service}" in
        app|sub2api) container_name="${APP_CONTAINER}" ;;
        postgres) container_name="${POSTGRES_CONTAINER}" ;;
        redis) container_name="${REDIS_CONTAINER}" ;;
        *) die "Unknown service '${service}'. Use app, postgres, or redis." ;;
    esac

    require_container_version
    system_is_running || die "Apple container services are not running."
    resource_exists container "${container_name}" || die "Container not found: ${container_name}"
    assert_resource_owned container "${container_name}"
    if [[ -n "${follow}" ]]; then
        container logs --follow "${container_name}"
    else
        container logs "${container_name}"
    fi
}

cmd_pull() {
    ensure_system
    prepare_environment
    info "Pulling ${APP_IMAGE}..."
    container image pull --platform "${PLATFORM}" "${APP_IMAGE}"
    info "Pulling ${POSTGRES_IMAGE}..."
    container image pull --platform "${PLATFORM}" "${POSTGRES_IMAGE}"
    info "Pulling ${REDIS_IMAGE}..."
    container image pull --platform "${PLATFORM}" "${REDIS_IMAGE}"
}

confirm_destroy() {
    local include_volumes=$1
    local answer

    if [[ "${include_volumes}" == true ]]; then
        printf 'Delete the Sub2API stack and all persistent data? [y/N] '
    else
        printf 'Delete the Sub2API containers and network, preserving volumes? [y/N] '
    fi
    read -r answer
    [[ "${answer}" == "y" || "${answer}" == "Y" ]]
}

delete_volume_if_present() {
    local volume_name=$1

    if resource_exists volume "${volume_name}"; then
        assert_resource_owned volume "${volume_name}"
        info "Deleting volume ${volume_name}..."
        container volume delete "${volume_name}" >/dev/null
    fi
}

cmd_destroy() {
    local include_volumes=false
    local assume_yes=false
    local argument

    for argument in "$@"; do
        case "${argument}" in
            --volumes) include_volumes=true ;;
            --yes) assume_yes=true ;;
            *) usage; exit 2 ;;
        esac
    done

    require_container_version
    start_system
    preflight_stack_ownership
    if [[ "${assume_yes}" != true ]] && ! confirm_destroy "${include_volumes}"; then
        info "Cancelled."
        return
    fi

    delete_container_if_present "${APP_CONTAINER}"
    delete_container_if_present "${REDIS_CONTAINER}"
    delete_container_if_present "${POSTGRES_CONTAINER}"

    if resource_exists network "${NETWORK_NAME}"; then
        assert_resource_owned network "${NETWORK_NAME}"
        info "Deleting network ${NETWORK_NAME}..."
        container network delete "${NETWORK_NAME}" >/dev/null
    fi

    if [[ "${include_volumes}" == true ]]; then
        delete_volume_if_present "${APP_VOLUME}"
        delete_volume_if_present "${REDIS_VOLUME}"
        delete_volume_if_present "${POSTGRES_VOLUME}"
        info "Sub2API stack and persistent data deleted."
    else
        info "Sub2API stack deleted; persistent volumes were preserved."
    fi
}

main() {
    local command=${1-}
    if [[ $# -gt 0 ]]; then
        shift
    fi

    case "${command}" in
        init)
            [[ $# -eq 0 ]] || { usage; exit 2; }
            acquire_lock
            cmd_init
            ;;
        up)
            acquire_lock
            cmd_up "$@"
            ;;
        down)
            [[ $# -eq 0 ]] || { usage; exit 2; }
            acquire_lock
            cmd_down
            ;;
        restart)
            [[ $# -eq 0 ]] || { usage; exit 2; }
            acquire_lock
            cmd_restart
            ;;
        status)
            [[ $# -eq 0 ]] || { usage; exit 2; }
            trap cleanup EXIT
            cmd_status
            ;;
        logs)
            cmd_logs "$@"
            ;;
        pull)
            [[ $# -eq 0 ]] || { usage; exit 2; }
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
}

main "$@"
