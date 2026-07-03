#!/bin/bash
# Sub2API 启动脚本
# 依赖：Docker + Docker Compose，配置文件位于 deploy/.env

set -e

DEPLOY_DIR="$(cd "$(dirname "$0")/deploy" && pwd)"
COMPOSE_FILE="$DEPLOY_DIR/docker-compose.yml"
ENV_FILE="$DEPLOY_DIR/.env"
ENV_EXAMPLE="$DEPLOY_DIR/.env.example"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
error()   { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 检查依赖
command -v docker >/dev/null 2>&1 || error "docker 未安装"
docker compose version >/dev/null 2>&1 || \
  command -v docker-compose >/dev/null 2>&1 || \
  error "docker compose 未安装"

COMPOSE_CMD="docker compose"
docker compose version >/dev/null 2>&1 || COMPOSE_CMD="docker-compose"

# 初始化 .env
if [ ! -f "$ENV_FILE" ]; then
  warn ".env 不存在，正在从 .env.example 生成..."
  command -v openssl >/dev/null 2>&1 || error "需要 openssl 生成密钥，请先安装"

  cp "$ENV_EXAMPLE" "$ENV_FILE"
  JWT_SECRET=$(openssl rand -hex 32)
  TOTP_KEY=$(openssl rand -hex 32)
  PG_PASS=$(openssl rand -hex 32)

  if sed --version >/dev/null 2>&1; then
    sed -i "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET}/" "$ENV_FILE"
    sed -i "s/^TOTP_ENCRYPTION_KEY=.*/TOTP_ENCRYPTION_KEY=${TOTP_KEY}/" "$ENV_FILE"
    sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${PG_PASS}/" "$ENV_FILE"
  else
    sed -i '' "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET}/" "$ENV_FILE"
    sed -i '' "s/^TOTP_ENCRYPTION_KEY=.*/TOTP_ENCRYPTION_KEY=${TOTP_KEY}/" "$ENV_FILE"
    sed -i '' "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${PG_PASS}/" "$ENV_FILE"
  fi
  chmod 600 "$ENV_FILE"

  echo ""
  success ".env 已生成，密钥如下（请妥善保存）："
  echo "  POSTGRES_PASSWORD:   ${PG_PASS}"
  echo "  JWT_SECRET:          ${JWT_SECRET}"
  echo "  TOTP_ENCRYPTION_KEY: ${TOTP_KEY}"
  echo ""
  warn "若 ADMIN_PASSWORD 未设置，首次启动后请查看日志获取自动生成的管理员密码"
  echo ""
fi

info "启动 Sub2API 服务..."
$COMPOSE_CMD -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d

echo ""
success "服务已启动，访问地址：http://localhost:8080"
echo ""
info "查看日志：  docker compose -f deploy/docker-compose.yml logs -f sub2api"
info "停止服务：  ./stop.sh"
