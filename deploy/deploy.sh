#!/usr/bin/env bash
set -euo pipefail

# 部署前按需修改这里。REMOTE_DIR 可为绝对路径或相对远端用户主目录的路径。
SSH_HOST="gcp"
SSH_PORT=22
APP_PORT=8083
BIND_HOST="127.0.0.1"
REMOTE_DIR="my-sub2api"
PROJECT_NAME="my-sub2api"
LOCAL_IMAGE="sub2api:local"

repo_root=$(cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

# SSH 会将参数拼成远端 shell 命令，配置仅接受普通名称和路径。
[[ "$SSH_HOST" =~ ^[a-zA-Z0-9_@.-]+$ && "$SSH_HOST" != -* ]] || { echo 'Invalid SSH_HOST' >&2; exit 1; }
for port in "$SSH_PORT" "$APP_PORT"; do
    [[ "$port" =~ ^[0-9]{1,5}$ ]] && (( 10#$port > 0 && 10#$port < 65536 )) || { echo 'Invalid port' >&2; exit 1; }
done
[[ "$BIND_HOST" =~ ^[0-9a-fA-F.:]+$ ]] || { echo 'Invalid BIND_HOST' >&2; exit 1; }
[[ "$REMOTE_DIR" =~ ^[a-zA-Z0-9_./-]+$ && "$REMOTE_DIR" != *..* && "$REMOTE_DIR" != -* && "$REMOTE_DIR" != / && "$REMOTE_DIR" != . ]] || { echo 'Invalid REMOTE_DIR' >&2; exit 1; }
[[ "$PROJECT_NAME" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || { echo 'Invalid PROJECT_NAME' >&2; exit 1; }

docker image inspect "$LOCAL_IMAGE" >/dev/null || { echo 'Run make docker-build first' >&2; exit 1; }
ssh -p "$SSH_PORT" "$SSH_HOST" 'set -e; docker compose version >/dev/null; command -v openssl >/dev/null'

remote_image="$PROJECT_NAME:$(date -u +%Y%m%d%H%M%S)"
docker tag "$LOCAL_IMAGE" "$remote_image"
docker save "$remote_image" | gzip | ssh -p "$SSH_PORT" "$SSH_HOST" 'gzip -dc | docker load'
ssh -p "$SSH_PORT" "$SSH_HOST" "mkdir -p -- $REMOTE_DIR"
scp -P "$SSH_PORT" deploy/docker-compose.local.yml "$SSH_HOST:$REMOTE_DIR/docker-compose.yml"

ssh -p "$SSH_PORT" "$SSH_HOST" bash -s -- "$REMOTE_DIR" "$remote_image" "$PROJECT_NAME" "$APP_PORT" "$BIND_HOST" <<'REMOTE'
set -euo pipefail
remote_dir=$1
[[ "$remote_dir" = /* ]] || remote_dir="$HOME/$remote_dir"
cd "$remote_dir"
image=$2
project=$3
port=$4
bind_host=$5

env_tmp=$(mktemp .env.XXXXXX)
trap 'rm -f "$env_tmp"' EXIT
if [ -e .env ]; then
    sed -E '/^(SERVER_PORT|BIND_HOST)=/d' .env > "$env_tmp"
else
    postgres_password=$(openssl rand -hex 24)
    jwt_secret=$(openssl rand -hex 32)
    totp_key=$(openssl rand -hex 32)
    admin_id=$(openssl rand -hex 6)
    admin_password=$(openssl rand -hex 24)
    {
        printf 'POSTGRES_PASSWORD=%s\n' "$postgres_password"
        printf 'JWT_SECRET=%s\n' "$jwt_secret"
        printf 'TOTP_ENCRYPTION_KEY=%s\n' "$totp_key"
        printf 'ADMIN_EMAIL=admin-%s@example.test\n' "$admin_id"
        printf 'ADMIN_PASSWORD=%s\n' "$admin_password"
    } > "$env_tmp"
fi
printf 'SERVER_PORT=%s\nBIND_HOST=%s\n' "$port" "$bind_host" >> "$env_tmp"
mv "$env_tmp" .env

# 用覆盖文件选择本地镜像和独立容器名，不改上游部署配置。
cat > docker-compose.override.yml <<YAML
services:
  sub2api:
    image: $image
    container_name: $project
    healthcheck:
      test: ["CMD", "wget", "-Y", "off", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8080/health"]
  postgres:
    container_name: $project-postgres
  redis:
    container_name: $project-redis
YAML
docker compose -p "$project" --env-file .env -f docker-compose.yml -f docker-compose.override.yml up -d --wait
REMOTE

printf 'Deployed %s to %s:%s (app port %s)\n' "$remote_image" "$SSH_HOST" "$REMOTE_DIR" "$APP_PORT"
