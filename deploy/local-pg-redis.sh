#!/usr/bin/env bash
# 启动本地 PostgreSQL 18 + Redis 8（podman rootless）并创建 sub2api 数据库。
# 用法： bash deploy/local-pg-redis.sh
set -euo pipefail

PG_USER=sub2api
PG_PASS=sub2api
PG_DB=sub2api
PG_CONTAINER=sub2api-pg
REDIS_CONTAINER=sub2api-redis

# 1. PostgreSQL 18
if ! podman ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  if podman ps -a --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
    echo "[pg] starting existing container..."
    podman start "${PG_CONTAINER}"
  else
    echo "[pg] creating container from postgres:18-alpine..."
    podman run -d --name "${PG_CONTAINER}" \
      -e POSTGRES_USER="${PG_USER}" \
      -e POSTGRES_PASSWORD="${PG_PASS}" \
      -e POSTGRES_DB="${PG_DB}" \
      -e TZ=Asia/Shanghai \
      -p 5432:5432 \
      docker.io/library/postgres:18-alpine
  fi
else
  echo "[pg] already running"
fi

# 2. Redis 8
if ! podman ps --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
  if podman ps -a --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
    echo "[redis] starting existing container..."
    podman start "${REDIS_CONTAINER}"
  else
    echo "[redis] creating container from redis:8-alpine..."
    podman run -d --name "${REDIS_CONTAINER}" \
      -e TZ=Asia/Shanghai \
      -p 6379:6379 \
      docker.io/library/redis:8-alpine \
      redis-server --save 60 1 --appendonly yes --appendfsync everysec
  fi
else
  echo "[redis] already running"
fi

# 3. 等 PostgreSQL 就绪并确认 sub2api 数据库存在
echo "[pg] waiting for readiness..."
for i in $(seq 1 30); do
  if podman exec "${PG_CONTAINER}" pg_isready -U "${PG_USER}" -d "${PG_DB}" >/dev/null 2>&1; then
    echo "[pg] ready"
    break
  fi
  sleep 1
done

# POSTGRES_DB 已在 run 时创建了 sub2api 库，这里只是校验它存在
echo "[pg] databases:"
podman exec "${PG_CONTAINER}" psql -U "${PG_USER}" -d "${PG_DB}" -c "\l" | grep -E "sub2api|Name"

echo
echo "=== 依赖已就绪 ==="
echo "PostgreSQL: localhost:5432  user=${PG_USER} pass=${PG_PASS} db=${PG_DB}"
echo "Redis:      localhost:6379  (no password, db=0)"
echo
echo "项目配置已写入 deploy/config.yaml"
echo "启动后端: cd backend && go run ./cmd/server/"
echo "启动前端: cd frontend && pnpm dev   (http://localhost:3000)"
