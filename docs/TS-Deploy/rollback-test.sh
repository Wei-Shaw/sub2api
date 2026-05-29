#!/bin/bash
# =============================================================================
# Sub2API 测试版（外包）还原脚本
# 停止当前容器，还原 docker-compose.yml 到上一次备份，重启服务
#
# 用法：./rollback-test.sh [SERVER] [PORT]
#       ./rollback-test.sh                          使用默认服务器
#       ./rollback-test.sh root@1.2.3.4 22          指定服务器和端口
# =============================================================================

set -euo pipefail

SERVER="${1:-${SUB2API_DEPLOY_SERVER:-root@47.251.17.185}}"
SSH_PORT="${2:-${SUB2API_DEPLOY_PORT:-22}}"
REMOTE_DIR="/root/sub2api-waibao"
CONTAINER_NAME="sub2api-waibao"
SERVICE_PORT="8081"
HEALTH_TIMEOUT=45

trap 'echo ""; echo "错误: 脚本在第 ${LINENO} 行执行失败"; echo "紧急操作: ssh -p ${SSH_PORT} ${SERVER} \"cd ${REMOTE_DIR} && docker compose up -d ${CONTAINER_NAME}\""; exit 1' ERR

echo "=========================================="
echo "  Sub2API 测试版（外包）还原"
echo "=========================================="
echo "  服务器: ${SERVER}:${SSH_PORT}"
echo "  容器:   ${CONTAINER_NAME}"
echo "=========================================="

# --------------- 远程探测当前状态 ---------------
echo ""
echo "[探测] 检查服务器当前运行状态..."
REMOTE_STATUS=$(ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_DETECT
  DOCKER_RUNNING=false
  DOCKER_IMAGE=none
  HAS_BACKUP=false
  PREVIOUS_IMAGE=none

  if docker ps --format "{{.Names}}" 2>/dev/null | grep -qx ${CONTAINER_NAME}; then
    DOCKER_RUNNING=true
    DOCKER_IMAGE=\$(docker inspect ${CONTAINER_NAME} --format "{{.Config.Image}}" 2>/dev/null || echo unknown)
  fi

  LATEST_BACKUP=\$(ls -t ${REMOTE_DIR}/backups/*/docker-compose.yml 2>/dev/null | head -1)
  if [ -n "\${LATEST_BACKUP}" ]; then
    HAS_BACKUP=true
    BACKUP_DIR=\$(dirname "\${LATEST_BACKUP}")
    if [ -f "\${BACKUP_DIR}/previous_image.txt" ]; then
      PREVIOUS_IMAGE=\$(cat "\${BACKUP_DIR}/previous_image.txt")
    fi
  fi

  echo "DOCKER_RUNNING=\${DOCKER_RUNNING}"
  echo "DOCKER_IMAGE=\${DOCKER_IMAGE}"
  echo "HAS_BACKUP=\${HAS_BACKUP}"
  echo "PREVIOUS_IMAGE=\${PREVIOUS_IMAGE}"
REMOTE_DETECT
) || { echo "错误: 无法连接到服务器 ${SERVER}:${SSH_PORT}"; exit 1; }

# 安全解析远程变量
DOCKER_RUNNING=false
DOCKER_IMAGE=none
HAS_BACKUP=false
PREVIOUS_IMAGE=none

while IFS='=' read -r key value; do
  case "$key" in
    DOCKER_RUNNING|DOCKER_IMAGE|HAS_BACKUP|PREVIOUS_IMAGE)
      if [[ "$value" =~ ^[a-zA-Z0-9._:/_-]+$ ]]; then
        declare "$key=$value"
      fi
      ;;
  esac
done <<< "${REMOTE_STATUS}"

echo "  Docker 容器: ${DOCKER_RUNNING} (${DOCKER_IMAGE})"
echo "  可用备份: ${HAS_BACKUP}"
echo "  上一版本镜像: ${PREVIOUS_IMAGE}"

if [[ "$DOCKER_RUNNING" != "true" ]]; then
  echo ""
  echo "未检测到 ${CONTAINER_NAME} 容器在运行。"
  echo "提示: 可尝试启动: ssh -p ${SSH_PORT} ${SERVER} 'cd ${REMOTE_DIR} && docker compose up -d ${CONTAINER_NAME}'"
  exit 0
fi

if [[ "$HAS_BACKUP" != "true" ]]; then
  echo ""
  echo "错误: 未找到 docker-compose.yml 备份，无法还原"
  echo "  请手动检查: ssh -p ${SSH_PORT} ${SERVER} 'ls ${REMOTE_DIR}/backups/'"
  exit 1
fi

echo ""
read -p "确认还原？将停止当前容器并还原到上一版本 (y/N) " confirm
[[ "$confirm" != "y" && "$confirm" != "Y" ]] && echo "已取消" && exit 0

# --------------- 执行还原 ---------------
echo ""
echo "[1/3] 停止当前容器..."
ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_STOP
  cd ${REMOTE_DIR}
  docker compose stop ${CONTAINER_NAME} 2>/dev/null || docker stop ${CONTAINER_NAME} 2>/dev/null || true
  docker compose rm -f ${CONTAINER_NAME} 2>/dev/null || true

  PORT_FREE=false
  for i in \$(seq 1 15); do
    if ! ss -tlnp 2>/dev/null | grep -q ":${SERVICE_PORT} "; then
      echo "  端口 ${SERVICE_PORT} 已释放"
      PORT_FREE=true
      break
    fi
    echo "  等待端口释放... (\${i}/15)"
    sleep 1
  done
  if [ "\${PORT_FREE}" != "true" ]; then
    echo "错误: 端口 ${SERVICE_PORT} 在 15 秒后仍被占用，无法继续"
    echo "  排查: ss -tlnp | grep :${SERVICE_PORT}"
    exit 1
  fi
REMOTE_STOP

echo ""
echo "[2/3] 还原 docker-compose.yml..."
ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_RESTORE
  cd ${REMOTE_DIR}
  LATEST_BACKUP=\$(ls -t backups/*/docker-compose.yml 2>/dev/null | head -1)
  if [ -n "\${LATEST_BACKUP}" ]; then
    cp "\${LATEST_BACKUP}" docker-compose.yml
    echo "  已从备份还原: \${LATEST_BACKUP}"
  else
    echo "  无可用备份，跳过还原 docker-compose.yml"
  fi
REMOTE_RESTORE

echo ""
echo "[3/3] 重启服务..."
ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_START
  cd ${REMOTE_DIR}
  docker compose up -d ${CONTAINER_NAME}
REMOTE_START

# --------------- 健康检查 ---------------
echo ""
echo "等待服务启动..."
health_result=$(ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_HEALTH
for i in \$(seq 1 ${HEALTH_TIMEOUT}); do
  status=\$(docker inspect ${CONTAINER_NAME} --format '{{.State.Health.Status}}' 2>/dev/null || echo "none")
  if [ "\$status" = "healthy" ]; then
    echo "READY:\${i}"
    exit 0
  fi
  sleep 1
done
echo "TIMEOUT"
REMOTE_HEALTH
) || health_result="TIMEOUT"

SERVICE_READY=false
if [[ "$health_result" == READY:* ]]; then
  wait_sec="${health_result#READY:}"
  echo "  服务已就绪（等待 ${wait_sec} 秒）"
  SERVICE_READY=true
else
  echo "  警告: 服务在 ${HEALTH_TIMEOUT} 秒内未通过健康检查，请手动检查"
  echo "  ssh -p ${SSH_PORT} ${SERVER} 'docker logs ${CONTAINER_NAME} --tail 50'"
fi

echo ""
echo "服务状态:"
ssh -p "${SSH_PORT}" "${SERVER}" \
  "docker ps --filter 'name=^${CONTAINER_NAME}$' --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'"

# --------------- 清理开发镜像 ---------------
if [[ "$SERVICE_READY" == "true" ]]; then
  echo ""
  echo "清理开发镜像..."
  ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_CLEAN
  docker rmi sub2api:ts-dev 2>/dev/null && echo "  已删除 sub2api:ts-dev" || echo "  无需清理"
  echo "[\$(date)] rollback ${CONTAINER_NAME} from ${DOCKER_IMAGE} to previous" >> ${REMOTE_DIR}/rollback.log
REMOTE_CLEAN
else
  echo ""
  echo "服务未确认就绪，跳过清理开发镜像（保留以便回退）"
fi

echo ""
echo "=========================================="
if [[ "$SERVICE_READY" == "true" ]]; then
  echo "  测试版已还原到上一版本"
else
  echo "  还原可能未完成，请检查服务器状态"
fi
echo "=========================================="
