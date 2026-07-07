#!/usr/bin/env bash
# Build and/or run the Ubuntu-based runtime image from artifacts in build-out/.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

ACTION="${ACTION:-all}" # build | run | all
IMAGE_NAME="${IMAGE_NAME:-sub2api:ubuntu}"
UBUNTU_IMAGE="${UBUNTU_IMAGE:-hub.bilibili.co/nyx-base/ubuntu:22.04}"
APT_MIRROR="${APT_MIRROR:-mirrors.ustc.edu.cn}"
CONTAINER_NAME="${CONTAINER_NAME:-sub2api}"
APP_PORT="${APP_PORT:-8080}"
DATA_VOLUME="${DATA_VOLUME:-sub2api_data}"
TZ_VALUE="${TZ:-Asia/Shanghai}"
ENV_FILE="${ENV_FILE:-}"
NETWORK_NAME="${NETWORK_NAME:-}"
FORCE_RECREATE="${FORCE_RECREATE:-0}"
POSTGRES_HOST_PORT="${POSTGRES_HOST_PORT:-}"
REDIS_HOST_PORT="${REDIS_HOST_PORT:-}"

ensure_artifacts() {
    if [ ! -x "${REPO_ROOT}/build-out/sub2api" ]; then
        echo "Missing build artifact: ${REPO_ROOT}/build-out/sub2api" >&2
        echo "Run deploy/run_build_in_builder_ubuntu.sh first." >&2
        exit 1
    fi
}

build_image() {
    ensure_artifacts
    docker build -t "${IMAGE_NAME}" \
        --build-arg UBUNTU_IMAGE="${UBUNTU_IMAGE}" \
        --build-arg APT_MIRROR="${APT_MIRROR}" \
        -f "${REPO_ROOT}/Dockerfile.runtime.ubuntu" \
        "${REPO_ROOT}"
}

run_container() {
    local postgres_host_bind
    local redis_host_bind

    postgres_host_bind="${POSTGRES_HOST_BIND:-127.0.0.1}"
    redis_host_bind="${REDIS_HOST_BIND:-127.0.0.1}"

    if [ -n "${POSTGRES_HOST_PORT}" ] && [ -z "${POSTGRES_HOST_BIND:-}" ]; then
        postgres_host_bind="0.0.0.0"
    fi
    if [ -n "${REDIS_HOST_PORT}" ] && [ -z "${REDIS_HOST_BIND:-}" ]; then
        redis_host_bind="0.0.0.0"
    fi

    if docker ps -a --format '{{.Names}}' | grep -Fxq "${CONTAINER_NAME}"; then
        if [ "${FORCE_RECREATE}" = "1" ]; then
            docker rm -f "${CONTAINER_NAME}"
        else
            echo "Container ${CONTAINER_NAME} already exists. Set FORCE_RECREATE=1 to replace it." >&2
            exit 1
        fi
    fi

    docker volume create "${DATA_VOLUME}" >/dev/null

    env_args=(
        --env "AUTO_SETUP=${AUTO_SETUP:-true}"
        --env "SERVER_HOST=0.0.0.0"
        --env "SERVER_PORT=8080"
        --env "SERVER_MODE=${SERVER_MODE:-release}"
        --env "RUN_MODE=${RUN_MODE:-standard}"
        --env "POSTGRES_HOST_BIND=${postgres_host_bind}"
        --env "REDIS_HOST_BIND=${redis_host_bind}"
        --env "DATA_DIR=/app/data"
        --env "TZ=${TZ_VALUE}"
        --env "DATABASE_HOST=127.0.0.1"
        --env "DATABASE_PORT=${DATABASE_PORT:-5432}"
        --env "DATABASE_USER=${DATABASE_USER:-sub2api}"
        --env "DATABASE_PASSWORD=${DATABASE_PASSWORD:-}"
        --env "DATABASE_DBNAME=${DATABASE_DBNAME:-sub2api}"
        --env "DATABASE_SSLMODE=${DATABASE_SSLMODE:-disable}"
        --env "DATABASE_MAX_OPEN_CONNS=${DATABASE_MAX_OPEN_CONNS:-50}"
        --env "DATABASE_MAX_IDLE_CONNS=${DATABASE_MAX_IDLE_CONNS:-10}"
        --env "DATABASE_CONN_MAX_LIFETIME_MINUTES=${DATABASE_CONN_MAX_LIFETIME_MINUTES:-30}"
        --env "DATABASE_CONN_MAX_IDLE_TIME_MINUTES=${DATABASE_CONN_MAX_IDLE_TIME_MINUTES:-5}"
        --env "REDIS_HOST=127.0.0.1"
        --env "REDIS_PORT=${REDIS_PORT:-6379}"
        --env "REDIS_PASSWORD=${REDIS_PASSWORD:-}"
        --env "REDIS_DB=${REDIS_DB:-0}"
        --env "REDIS_POOL_SIZE=${REDIS_POOL_SIZE:-1024}"
        --env "REDIS_MIN_IDLE_CONNS=${REDIS_MIN_IDLE_CONNS:-10}"
        --env "REDIS_ENABLE_TLS=${REDIS_ENABLE_TLS:-false}"
        --env "ADMIN_EMAIL=${ADMIN_EMAIL:-admin@sub2api.local}"
        --env "ADMIN_PASSWORD=${ADMIN_PASSWORD:-}"
        --env "JWT_SECRET=${JWT_SECRET:-}"
        --env "JWT_EXPIRE_HOUR=${JWT_EXPIRE_HOUR:-24}"
        --env "TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY:-}"
        --env "GEMINI_OAUTH_CLIENT_ID=${GEMINI_OAUTH_CLIENT_ID:-}"
        --env "GEMINI_OAUTH_CLIENT_SECRET=${GEMINI_OAUTH_CLIENT_SECRET:-}"
        --env "GEMINI_OAUTH_SCOPES=${GEMINI_OAUTH_SCOPES:-}"
        --env "GEMINI_QUOTA_POLICY=${GEMINI_QUOTA_POLICY:-}"
        --env "GEMINI_CLI_OAUTH_CLIENT_SECRET=${GEMINI_CLI_OAUTH_CLIENT_SECRET:-}"
        --env "ANTIGRAVITY_OAUTH_CLIENT_SECRET=${ANTIGRAVITY_OAUTH_CLIENT_SECRET:-}"
        --env "SECURITY_URL_ALLOWLIST_ENABLED=${SECURITY_URL_ALLOWLIST_ENABLED:-false}"
        --env "SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=${SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP:-false}"
        --env "SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=${SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS:-false}"
        --env "SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS=${SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS:-}"
        --env "UPDATE_PROXY_URL=${UPDATE_PROXY_URL:-}"
    )

    if [ -n "${ENV_FILE}" ]; then
        env_args+=(--env-file "${ENV_FILE}")
    fi

    run_args=(
        -d
        --name "${CONTAINER_NAME}"
        --restart unless-stopped
        -p "${APP_PORT}:8080"
        -v "${DATA_VOLUME}:/app/data"
    )

    if [ -n "${POSTGRES_HOST_PORT}" ]; then
        run_args+=(-p "${POSTGRES_HOST_PORT}:${DATABASE_PORT:-5432}")
    fi

    if [ -n "${REDIS_HOST_PORT}" ]; then
        run_args+=(-p "${REDIS_HOST_PORT}:${REDIS_PORT:-6379}")
    fi

    if [ -n "${NETWORK_NAME}" ]; then
        run_args+=(--network "${NETWORK_NAME}")
    fi

    docker run "${run_args[@]}" "${env_args[@]}" "${IMAGE_NAME}"
}

case "${ACTION}" in
    build)
        build_image
        ;;
    run)
        run_container
        ;;
    all)
        build_image
        run_container
        ;;
    *)
        echo "Unsupported ACTION=${ACTION}. Use build, run, or all." >&2
        exit 1
        ;;
esac
