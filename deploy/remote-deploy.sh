#!/usr/bin/env bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/sub2api}"
VERSION="${1:?usage: remote-deploy.sh <image-tag>}"
COMPOSE_FILE="$APP_DIR/docker-compose.yml"
REGISTRY_ENV_FILE="${REGISTRY_ENV_FILE:-$APP_DIR/registry.env}"
SKIP_PULL="${SKIP_PULL:-0}"

ensure_app_env() {
  [[ -f "$APP_DIR/app.env" ]] && return 0
  command -v openssl >/dev/null 2>&1 || {
    echo "missing $APP_DIR/app.env and openssl is unavailable for first-run secret generation" >&2
    exit 1
  }

  local postgres_password jwt_secret totp_encryption_key
  postgres_password="$(openssl rand -hex 32)"
  jwt_secret="$(openssl rand -hex 32)"
  totp_encryption_key="$(openssl rand -hex 32)"
  umask 077
  cat > "$APP_DIR/app.env" <<EOF
BIND_HOST=0.0.0.0
SERVER_PORT=8080
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=$postgres_password
POSTGRES_DB=sub2api
POSTGRES_MAX_CONNECTIONS=200
POSTGRES_SHARED_BUFFERS=128MB
POSTGRES_EFFECTIVE_CACHE_SIZE=512MB
POSTGRES_MAINTENANCE_WORK_MEM=64MB
DATABASE_MAX_OPEN_CONNS=50
DATABASE_MAX_IDLE_CONNS=10
REDIS_POOL_SIZE=256
REDIS_MIN_IDLE_CONNS=10
JWT_SECRET=$jwt_secret
TOTP_ENCRYPTION_KEY=$totp_encryption_key
TZ=Asia/Shanghai
EOF
  chmod 600 "$APP_DIR/app.env"
  echo "created $APP_DIR/app.env with generated persistent secrets"
}

ensure_app_env
[[ -f "$COMPOSE_FILE" ]] || { echo "missing $COMPOSE_FILE" >&2; exit 1; }

if [[ "$SKIP_PULL" != "1" ]]; then
  [[ -f "$REGISTRY_ENV_FILE" ]] || { echo "missing $REGISTRY_ENV_FILE" >&2; exit 1; }
  set -a
  # shellcheck disable=SC1090
  . "$REGISTRY_ENV_FILE"
  set +a
  : "${GHCR_USERNAME:?GHCR_USERNAME is required}"
  : "${GHCR_TOKEN:?GHCR_TOKEN is required}"
fi
: "${GHCR_IMAGE_REPOSITORY:?GHCR_IMAGE_REPOSITORY is required}"

COMPOSE=(docker compose --project-name sub2api --env-file "$APP_DIR/app.env" -f "$COMPOSE_FILE")
IMAGE="ghcr.io/${GHCR_IMAGE_REPOSITORY}:${VERSION}"
PREVIOUS_IMAGE="$(docker inspect --format '{{.Config.Image}}' sub2api 2>/dev/null || true)"

wait_for_health() {
  local expected_image="$1"
  for _ in $(seq 1 45); do
    local app_status postgres_status redis_status app_image
    app_status="$(docker inspect --format '{{.State.Health.Status}}' sub2api 2>/dev/null || true)"
    postgres_status="$(docker inspect --format '{{.State.Health.Status}}' sub2api-postgres 2>/dev/null || true)"
    redis_status="$(docker inspect --format '{{.State.Health.Status}}' sub2api-redis 2>/dev/null || true)"
    app_image="$(docker inspect --format '{{.Config.Image}}' sub2api 2>/dev/null || true)"
    if [[ "$app_status" == healthy && "$postgres_status" == healthy && "$redis_status" == healthy && "$app_image" == "$expected_image" ]]; then
      return 0
    fi
    sleep 2
  done
  return 1
}

print_diagnostics() {
  docker inspect --format '{{.Name}} status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}} image={{.Config.Image}}' \
    sub2api sub2api-postgres sub2api-redis 2>&1 || true
  docker compose --project-name sub2api --env-file "$APP_DIR/app.env" -f "$COMPOSE_FILE" logs --tail=80 sub2api postgres redis 2>&1 || true
}

cleanup_old_images() {
  local keep_image="${1:-}"
  local rollback_image="${2:-}"
  while IFS= read -r image_ref; do
    [[ -z "$image_ref" || "$image_ref" == *':<none>' ]] && continue
    [[ "$image_ref" == "$keep_image" || "$image_ref" == "$rollback_image" ]] && continue
    docker image rm "$image_ref" >/dev/null 2>&1 || true
  done < <(docker images --format '{{.Repository}}:{{.Tag}}' "ghcr.io/${GHCR_IMAGE_REPOSITORY}")
}

recover_previous_image() {
  if [[ -n "$PREVIOUS_IMAGE" && "$PREVIOUS_IMAGE" != "$IMAGE" ]]; then
    echo "recovering existing image: $PREVIOUS_IMAGE" >&2
    SUB2API_IMAGE="$PREVIOUS_IMAGE" "${COMPOSE[@]}" up -d --remove-orphans
    if wait_for_health "$PREVIOUS_IMAGE"; then
      echo "existing image recovered; deployment remains failed" >&2
    else
      echo "existing image recovery failed" >&2
    fi
  else
    echo "no previous Sub2API image available for recovery" >&2
  fi
}

if [[ "$SKIP_PULL" != "1" ]]; then
  if ! {
    printf '%s' "$GHCR_TOKEN" | docker login ghcr.io --username "$GHCR_USERNAME" --password-stdin >/dev/null
    trap 'docker logout ghcr.io >/dev/null 2>&1 || true' EXIT
    SUB2API_IMAGE="$IMAGE" "${COMPOSE[@]}" pull sub2api postgres redis
  }; then
    echo "image pull failed: $IMAGE" >&2
    print_diagnostics
    recover_previous_image
    exit 1
  fi
fi

SUB2API_IMAGE="$IMAGE" "${COMPOSE[@]}" up -d --remove-orphans
if ! wait_for_health "$IMAGE"; then
  echo "deployment failed health checks: $IMAGE" >&2
  print_diagnostics
  if [[ -n "$PREVIOUS_IMAGE" && "$PREVIOUS_IMAGE" != "$IMAGE" ]]; then
    echo "rolling back to $PREVIOUS_IMAGE" >&2
    SUB2API_IMAGE="$PREVIOUS_IMAGE" "${COMPOSE[@]}" up -d --remove-orphans
    if wait_for_health "$PREVIOUS_IMAGE"; then
      echo "rollback succeeded: $PREVIOUS_IMAGE" >&2
      cleanup_old_images "$PREVIOUS_IMAGE"
    else
      echo "rollback failed: $PREVIOUS_IMAGE" >&2
    fi
  fi
  exit 1
fi

cleanup_old_images "$IMAGE" "$PREVIOUS_IMAGE"
echo "deployed $IMAGE"
