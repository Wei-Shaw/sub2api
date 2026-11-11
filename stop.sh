#!/bin/bash
# Sub2API 停止脚本

DEPLOY_DIR="$(cd "$(dirname "$0")/deploy" && pwd)"
COMPOSE_FILE="$DEPLOY_DIR/docker-compose.yml"
ENV_FILE="$DEPLOY_DIR/.env"

COMPOSE_CMD="docker compose"
docker compose version >/dev/null 2>&1 || COMPOSE_CMD="docker-compose"

echo "停止 Sub2API 服务..."
$COMPOSE_CMD -f "$COMPOSE_FILE" --env-file "$ENV_FILE" down

echo "已停止。"
