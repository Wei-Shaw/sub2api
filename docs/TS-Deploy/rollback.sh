#!/bin/bash
# =============================================================================
# Sub2API 还原到正式版本
# 停止 Docker 开发容器，还原 docker-compose.yml，重启 systemd 官方服务
#
# 用法：./rollback.sh [SERVER] [PORT]
#       ./rollback.sh                          使用默认服务器
#       ./rollback.sh root@1.2.3.4 22          指定服务器和端口
# =============================================================================

set -euo pipefail

SERVER="${1:-${SUB2API_DEPLOY_SERVER:-root@47.251.17.185}}"
SSH_PORT="${2:-${SUB2API_DEPLOY_PORT:-22}}"
REMOTE_DIR="/root/sub2api"
SERVICE_PORT="8080"

trap 'echo ""; echo "错误: 脚本在第 ${LINENO} 行执行失败"; echo "紧急操作: ssh -p ${SSH_PORT} ${SERVER} systemctl start sub2api"; echo "如 systemd 无法启动: ssh -p ${SSH_PORT} ${SERVER} \"cd ${REMOTE_DIR} && docker compose up -d sub2api\""; exit 1' ERR

echo "=========================================="
echo "  Sub2API 还原到正式版本"
echo "=========================================="
echo "  服务器: ${SERVER}:${SSH_PORT}"
echo "=========================================="

# --------------- 远程探测当前状态 ---------------
echo ""
echo "[探测] 检查服务器当前运行状态..."
REMOTE_STATUS=$(ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_DETECT
  DOCKER_RUNNING=false
  SYSTEMD_RUNNING=false
  DOCKER_IMAGE=none
  SYSTEMD_VERSION=none
  HAS_BACKUP=false

  if docker ps --format "{{.Names}}" 2>/dev/null | grep -qx sub2api; then
    DOCKER_RUNNING=true
    DOCKER_IMAGE=\$(docker inspect sub2api --format "{{.Config.Image}}" 2>/dev/null || echo unknown)
  fi

  if systemctl is-active --quiet sub2api 2>/dev/null; then
    SYSTEMD_RUNNING=true
    SYSTEMD_VERSION=\$(/opt/sub2api/sub2api --version 2>/dev/null | grep -oE "v?[0-9]+\.[0-9]+\.[0-9]+" || echo unknown)
  fi

  LATEST_BACKUP=\$(ls -t ${REMOTE_DIR}/backups/*/docker-compose.yml 2>/dev/null | head -1)
  if [ -n "\${LATEST_BACKUP}" ]; then
    HAS_BACKUP=true
  fi

  echo "DOCKER_RUNNING=\${DOCKER_RUNNING}"
  echo "DOCKER_IMAGE=\${DOCKER_IMAGE}"
  echo "SYSTEMD_RUNNING=\${SYSTEMD_RUNNING}"
  echo "SYSTEMD_VERSION=\${SYSTEMD_VERSION}"
  echo "HAS_BACKUP=\${HAS_BACKUP}"
REMOTE_DETECT
) || { echo "错误: 无法连接到服务器 ${SERVER}:${SSH_PORT}"; exit 1; }

# 安全解析远程变量（不使用 eval，防止命令注入）
DOCKER_RUNNING=false
DOCKER_IMAGE=none
SYSTEMD_RUNNING=false
SYSTEMD_VERSION=none
HAS_BACKUP=false

while IFS='=' read -r key value; do
  case "$key" in
    DOCKER_RUNNING|DOCKER_IMAGE|SYSTEMD_RUNNING|SYSTEMD_VERSION|HAS_BACKUP)
      if [[ "$value" =~ ^[a-zA-Z0-9._:/_-]+$ ]]; then
        declare "$key=$value"
      fi
      ;;
  esac
done <<< "${REMOTE_STATUS}"

echo "  Docker 容器: ${DOCKER_RUNNING} (${DOCKER_IMAGE})"
echo "  Systemd 服务: ${SYSTEMD_RUNNING} (${SYSTEMD_VERSION})"
echo "  可用备份: ${HAS_BACKUP}"

if [[ "$DOCKER_RUNNING" != "true" ]]; then
  echo ""
  echo "未检测到 Docker 开发容器在运行，无需还原。"
  if [[ "$SYSTEMD_RUNNING" == "true" ]]; then
    echo "官方服务已在运行 (${SYSTEMD_VERSION})。"
  else
    echo "提示: 官方服务也未运行，可尝试: ssh -p ${SSH_PORT} ${SERVER} 'systemctl start sub2api'"
  fi
  exit 0
fi

if [[ "$HAS_BACKUP" != "true" ]]; then
  echo ""
  echo "警告: 未找到 docker-compose.yml 备份，还原后可能需要手动配置"
fi

echo ""
read -p "确认还原？将停止 Docker 开发容器并启动 systemd 官方服务 (y/N) " confirm
[[ "$confirm" != "y" && "$confirm" != "Y" ]] && echo "已取消" && exit 0

# --------------- 执行还原 ---------------
echo ""
echo "[1/3] 停止 Docker 开发容器..."
ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_STOP
  cd ${REMOTE_DIR}
  docker compose stop 2>/dev/null && docker compose rm -f 2>/dev/null || docker stop sub2api 2>/dev/null || true

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
# 还原目的：deploy.sh 修改了 compose 文件中的镜像名为 sub2api:ts-dev，
# 还原到备份状态以便下次 deploy.sh 部署时有正确的基准。
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
echo "[3/3] 启动 systemd 官方服务..."
ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_START
  if ! systemctl is-active --quiet sub2api; then
    systemctl start sub2api
    echo "  systemd 服务已启动"
  else
    echo "  systemd 服务已在运行"
  fi
REMOTE_START

# --------------- 验证 ---------------
echo ""
echo "等待服务启动..."
SERVICE_READY=false
for i in {1..15}; do
  if ssh -p "${SSH_PORT}" "${SERVER}" "systemctl is-active --quiet sub2api" 2>/dev/null; then
    echo "  服务已就绪（等待 ${i} 秒）"
    SERVICE_READY=true
    break
  fi
  sleep 1
done
if [[ "$SERVICE_READY" != "true" ]]; then
  echo "  警告: 服务在 15 秒内未就绪，请手动检查"
  echo "  ssh -p ${SSH_PORT} ${SERVER} 'journalctl -u sub2api --no-pager -n 20'"
fi

echo ""
echo "服务状态:"
ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_VERIFY
  echo "--- systemd ---"
  systemctl status sub2api --no-pager -l 2>/dev/null | head -5 || echo "  systemd 服务未找到"
  echo ""
  echo "--- Docker (应为空) ---"
  docker ps --filter "name=^sub2api$" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}" 2>/dev/null || true
  echo ""
  VERSION=\$(/opt/sub2api/sub2api --version 2>/dev/null || echo unknown)
  echo "当前版本: \${VERSION}"
REMOTE_VERIFY

# --------------- 确认成功后清理 ---------------
if [[ "$SERVICE_READY" == "true" ]]; then
  echo ""
  echo "清理开发镜像..."
  ssh -p "${SSH_PORT}" "${SERVER}" bash <<REMOTE_CLEAN
  docker rmi sub2api:ts-dev 2>/dev/null && echo "  已删除 sub2api:ts-dev" || echo "  无需清理"
  echo "[\$(date)] rollback from ${DOCKER_IMAGE} to systemd" >> ${REMOTE_DIR}/rollback.log
REMOTE_CLEAN
else
  echo ""
  echo "服务未确认就绪，跳过清理开发镜像（保留以便回退）"
fi

echo ""
echo "=========================================="
if [[ "$SERVICE_READY" == "true" ]]; then
  echo "  已还原到官方版本"
else
  echo "  还原可能未完成，请检查服务器状态"
fi
echo "=========================================="
