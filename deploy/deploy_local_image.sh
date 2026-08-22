#!/usr/bin/env bash
# Replace only the Sub2API application container with the local image.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${ENV_FILE:-${SCRIPT_DIR}/.env}"
COMPOSE_FILES=(
    --file "${SCRIPT_DIR}/docker-compose.standalone.yml"
    --file "${SCRIPT_DIR}/docker-compose.sub2api-network.yml"
    --file "${SCRIPT_DIR}/docker-compose.local-image.yml"
)

if [[ ! -f "${ENV_FILE}" ]]; then
    echo "Missing environment file: ${ENV_FILE}" >&2
    exit 1
fi

if ! docker image inspect sub2api:latest >/dev/null 2>&1; then
    echo "Local image sub2api:latest does not exist. Run ./build_image.sh first." >&2
    exit 1
fi

if ! docker network inspect docker_app-network >/dev/null 2>&1; then
    echo "Docker network docker_app-network does not exist." >&2
    exit 1
fi

docker compose \
    --env-file "${ENV_FILE}" \
    "${COMPOSE_FILES[@]}" \
    up --detach --no-deps --force-recreate sub2api

for _ in {1..60}; do
    health="$(docker inspect sub2api --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}')"
    if [[ "${health}" == "healthy" ]]; then
        docker exec sub2api wget -q -T 5 -O - http://127.0.0.1:8080/health
        echo
        echo "Sub2API is healthy: http://127.0.0.1:${SERVER_PORT:-8080}"
        exit 0
    fi
    if [[ "${health}" == "unhealthy" || "${health}" == "exited" || "${health}" == "dead" ]]; then
        docker logs --tail 100 sub2api >&2
        exit 1
    fi
    sleep 2
done

echo "Timed out waiting for Sub2API health check." >&2
docker logs --tail 100 sub2api >&2
exit 1
