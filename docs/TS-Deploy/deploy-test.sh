#!/bin/bash
# =============================================================================
# Sub2API 测试版（外包）镜像部署脚本
# 目标容器: sub2api-waibao (端口 8081)
# 用法：./deploy-test.sh build              构建并缓存镜像
#       ./deploy-test.sh deploy [VERSION]    部署指定版本（默认最新构建）
#       ./deploy-test.sh list                列出已缓存的构建
#       ./deploy-test.sh [SERVER] [PORT]     (兼容) 构建 + 部署一条龙
# =============================================================================

set -euo pipefail
trap 'echo ""; echo "错误: 脚本在第 ${LINENO} 行执行失败"; echo "如已部署到一半，请检查服务器状态或使用 rollback-test.sh 还原"; exit 1' ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
CACHE_DIR="${SCRIPT_DIR}/builds-test"
IMAGE_NAME="sub2api"
IMAGE_TAG="ts-dev"
CONTAINER_NAME="sub2api-waibao"
SERVICE_NAME="sub2api"
REMOTE_DIR="/root/sub2api-waibao"
CONTAINER_UID=1000
MAX_BACKUPS=5
HEALTH_TIMEOUT=45

# --------------- 版本信息 ---------------
get_version_info() {
  if ! git rev-parse --git-dir &>/dev/null; then
    echo "错误: 不在 git 仓库中"
    exit 1
  fi
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "警告: 工作区有未提交的更改，构建可能包含未跟踪的修改"
  fi
  COMMIT=$(git rev-parse --short HEAD)
  BRANCH=$(git branch --show-current)
  TAG_VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
  VERSION="${TAG_VERSION}.999-dev-${COMMIT}"
  DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
}

# --------------- 构建 ---------------
do_build() {
  get_version_info
  mkdir -p "${CACHE_DIR}"
  local tarball="${CACHE_DIR}/${IMAGE_NAME}-${VERSION}.tar.gz"

  echo "=========================================="
  echo "  Sub2API 测试版（外包）构建镜像"
  echo "=========================================="
  echo "  分支:   ${BRANCH}"
  echo "  Commit: ${COMMIT}"
  echo "  版本:   ${VERSION}"
  echo "=========================================="

  echo ""
  echo "[1/2] 构建 Docker 镜像..."
  docker buildx build \
    --platform linux/amd64 \
    --build-arg VERSION="${VERSION}" \
    --build-arg COMMIT="${COMMIT}" \
    --build-arg DATE="${DATE}" \
    -t "${IMAGE_NAME}:${IMAGE_TAG}" \
    --load \
    -f "${PROJECT_ROOT}/Dockerfile" "${PROJECT_ROOT}"

  echo ""
  echo "[2/2] 导出镜像为 tar.gz..."
  set +e
  docker save "${IMAGE_NAME}:${IMAGE_TAG}" | gzip > "${tarball}"
  local pipe_statuses=("${PIPESTATUS[@]}")
  set -e
  if [[ ${pipe_statuses[0]} -ne 0 || ${pipe_statuses[1]} -ne 0 ]]; then
    rm -f "${tarball}"
    echo "错误: docker save/gzip 失败 (save=${pipe_statuses[0]}, gzip=${pipe_statuses[1]})"
    exit 1
  fi
  echo "  文件: ${tarball} ($(du -h "${tarball}" | cut -f1))"

  # 写入 latest 软链接方便 deploy 默认取
  ln -sf "${IMAGE_NAME}-${VERSION}.tar.gz" "${CACHE_DIR}/latest.tar.gz"

  echo ""
  echo "=========================================="
  echo "  构建完成！"
  echo "  版本:   ${VERSION}"
  echo "  缓存:   ${tarball}"
  echo ""
  echo "  部署命令: ./deploy-test.sh deploy"
  echo "  指定版本: ./deploy-test.sh deploy root@server 22 ${VERSION}"
  echo "=========================================="
}

# --------------- 部署 ---------------
do_deploy() {
  local server="${1:-${SUB2API_DEPLOY_SERVER:-root@47.251.17.185}}"
  local port="${2:-${SUB2API_DEPLOY_PORT:-22}}"
  local ver="${3:-}"

  # 解析要部署的 tarball
  local tarball
  if [[ -n "$ver" ]]; then
    tarball="${CACHE_DIR}/${IMAGE_NAME}-${ver}.tar.gz"
    if [[ ! -f "$tarball" ]]; then
      echo "错误: 未找到版本 ${ver} 的构建缓存"
      echo "  期望路径: ${tarball}"
      echo "  使用 './deploy-test.sh list' 查看可用版本"
      exit 1
    fi
  else
    tarball="${CACHE_DIR}/latest.tar.gz"
    if [[ ! -f "$tarball" ]]; then
      echo "错误: 没有可用的构建缓存，请先运行 './deploy-test.sh build'"
      exit 1
    fi
    # 解析 latest 软链接获取真实版本（兼容 macOS 和 Linux）
    local real_link
    real_link="$(readlink -f "${tarball}" 2>/dev/null)" || \
    real_link="${CACHE_DIR}/$(readlink "${tarball}")"
    tarball="$real_link"
  fi

  # 从文件名提取版本号
  local filename
  filename="$(basename "${tarball}")"
  local deploy_ver="${filename#${IMAGE_NAME}-}"
  deploy_ver="${deploy_ver%.tar.gz}"

  echo "=========================================="
  echo "  Sub2API 测试版（外包）部署"
  echo "=========================================="
  echo "  版本:   ${deploy_ver}"
  echo "  文件:   ${tarball} ($(du -h "${tarball}" | cut -f1))"
  echo "  服务器: ${server}:${port}"
  echo "  容器:   ${CONTAINER_NAME}"
  echo "=========================================="
  read -p "确认部署？(y/N) " confirm
  [[ "$confirm" != "y" && "$confirm" != "Y" ]] && echo "已取消" && exit 0

  local remote_tmp="/tmp/${filename}"

  echo ""
  echo "[1/4] 上传到服务器并加载..."
  scp -P "${port}" "${tarball}" "${server}:${remote_tmp}"
  ssh -p "${port}" "${server}" bash <<REMOTE_LOAD
  docker load < ${remote_tmp}
  LOAD_STATUS=\$?
  rm -f ${remote_tmp}
  exit \${LOAD_STATUS}
REMOTE_LOAD

  echo ""
  echo "[2/4] 备份当前运行状态..."
  ssh -p "${port}" "${server}" bash <<REMOTE_BACKUP
  cd ${REMOTE_DIR}
  BACKUP_DIR="backups/\$(date +%Y%m%d_%H%M%S)"
  mkdir -p "\${BACKUP_DIR}"
  cp docker-compose.yml "\${BACKUP_DIR}/docker-compose.yml"
  CURRENT_IMAGE=\$(grep 'image:.*sub2api' docker-compose.yml | head -1 | awk '{print \$2}')
  echo "\${CURRENT_IMAGE}" > "\${BACKUP_DIR}/previous_image.txt"
  docker inspect ${CONTAINER_NAME} --format "{{.Config.Image}} {{.Created}}" > "\${BACKUP_DIR}/container_info.txt" 2>/dev/null || true
  echo "  备份目录: ${REMOTE_DIR}/\${BACKUP_DIR}"
  echo "  当前镜像: \${CURRENT_IMAGE}"

  # 轮转备份：保留最近 ${MAX_BACKUPS} 个
  cd backups
  BACKUP_COUNT=\$(ls -dt */ 2>/dev/null | wc -l | tr -d ' ')
  if [ "\${BACKUP_COUNT}" -gt ${MAX_BACKUPS} ]; then
    ls -dt */ | tail -n +$((MAX_BACKUPS + 1)) | while IFS= read -r dir; do rm -rf "\$dir"; done
    echo "  已清理旧备份，保留最近 ${MAX_BACKUPS} 个"
  fi
REMOTE_BACKUP

  echo ""
  echo "[3/4] 切换镜像并重启服务..."
  ssh -p "${port}" "${server}" bash <<REMOTE_SWITCH
  cd ${REMOTE_DIR}
  CURRENT_IMAGE=\$(grep 'image:.*sub2api' docker-compose.yml | head -1 | awk '{print \$2}')
  LINE_NUM=\$(grep -n 'image:.*sub2api' docker-compose.yml | head -1 | cut -d: -f1)
  if [ -z "\${LINE_NUM}" ]; then
    echo "错误: 无法从 docker-compose.yml 中提取当前 sub2api 镜像名"
    exit 1
  fi
  echo "  当前镜像: \${CURRENT_IMAGE} (行 \${LINE_NUM})"
  echo "  目标镜像: ${IMAGE_NAME}:${IMAGE_TAG}"
  sed -i "\${LINE_NUM}s|image: .*|image: ${IMAGE_NAME}:${IMAGE_TAG}|" docker-compose.yml

  mkdir -p "${REMOTE_DIR}/sub2api"
  chown -R ${CONTAINER_UID}:${CONTAINER_UID} "${REMOTE_DIR}/sub2api"
  echo "  数据目录权限已修正"

  docker compose stop ${SERVICE_NAME} 2>/dev/null || true
  docker compose rm -f ${SERVICE_NAME} 2>/dev/null || true
  docker compose up -d ${SERVICE_NAME}
REMOTE_SWITCH

  echo ""
  echo "[4/4] 等待服务启动..."
  local health_result
  health_result=$(ssh -p "${port}" "${server}" bash <<REMOTE_HEALTH
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

  local service_ready=false
  if [[ "$health_result" == READY:* ]]; then
    local wait_sec="${health_result#READY:}"
    echo "  服务已就绪（等待 ${wait_sec} 秒）"
    service_ready=true
  else
    echo "  警告: 服务在 ${HEALTH_TIMEOUT} 秒内未通过健康检查，请手动检查"
    echo "  ssh -p ${port} ${server} 'docker logs ${CONTAINER_NAME} --tail 50'"
  fi

  echo ""
  ssh -p "${port}" "${server}" \
    "docker ps --filter 'name=^${CONTAINER_NAME}$' --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'"

  echo ""
  echo "=========================================="
  if [[ "$service_ready" == "true" ]]; then
    echo "  测试版部署完成！版本: ${deploy_ver}"
  else
    echo "  测试版部署可能未完成，版本: ${deploy_ver}"
    echo "  请检查服务器日志确认状态"
  fi
  echo "=========================================="
}

# --------------- 列出缓存 ---------------
do_list() {
  if [[ ! -d "$CACHE_DIR" ]] || ! ls "${CACHE_DIR}"/${IMAGE_NAME}-*.tar.gz &>/dev/null; then
    echo "没有缓存的构建，请先运行 './deploy-test.sh build'"
    exit 0
  fi

  echo "已缓存的构建（测试版）："
  echo "-------------------------------------------"
  local latest_target=""
  if [[ -L "${CACHE_DIR}/latest.tar.gz" ]]; then
    latest_target="$(basename "$(readlink -f "${CACHE_DIR}/latest.tar.gz" 2>/dev/null || readlink "${CACHE_DIR}/latest.tar.gz")")"
  fi

  for f in "${CACHE_DIR}"/${IMAGE_NAME}-*.tar.gz; do
    local name size ts marker=""
    name="$(basename "$f")"
    size="$(du -h "$f" | cut -f1)"
    ts="$(stat -f '%Sm' -t '%Y-%m-%d %H:%M' "$f" 2>/dev/null || stat -c '%y' "$f" 2>/dev/null | cut -d. -f1)"
    [[ "$name" == "$latest_target" ]] && marker=" <- latest"
    local ver="${name#${IMAGE_NAME}-}"
    ver="${ver%.tar.gz}"
    printf "  %-40s %6s  %s%s\n" "$ver" "$size" "$ts" "$marker"
  done
}

# --------------- 清理缓存 ---------------
do_clean() {
  if [[ ! -d "$CACHE_DIR" ]]; then
    echo "没有缓存目录"
    exit 0
  fi
  local count
  count=$(ls "${CACHE_DIR}"/${IMAGE_NAME}-*.tar.gz 2>/dev/null | wc -l | tr -d ' ')
  echo "将删除 ${count} 个缓存构建（测试版）"
  read -p "确认？(y/N) " confirm
  [[ "$confirm" != "y" && "$confirm" != "Y" ]] && echo "已取消" && exit 0
  rm -f "${CACHE_DIR}"/${IMAGE_NAME}-*.tar.gz "${CACHE_DIR}/latest.tar.gz"
  echo "已清理"
}

# --------------- 主入口 ---------------
case "${1:-}" in
  build)
    do_build
    ;;
  deploy)
    shift
    do_deploy "${1:-}" "${2:-}" "${3:-}"
    ;;
  list)
    do_list
    ;;
  clean)
    do_clean
    ;;
  -h|--help|help)
    echo "用法（测试版/外包）："
    echo "  ./deploy-test.sh build                  构建并缓存镜像"
    echo "  ./deploy-test.sh deploy [SERVER] [PORT] [VERSION]"
    echo "                                          部署到服务器（默认最新构建）"
    echo "  ./deploy-test.sh list                   列出已缓存的构建"
    echo "  ./deploy-test.sh clean                  清理所有缓存构建"
    echo ""
    echo "环境变量："
    echo "  SUB2API_DEPLOY_SERVER                   默认服务器（如 root@1.2.3.4）"
    echo "  SUB2API_DEPLOY_PORT                     默认 SSH 端口（默认 22）"
    echo ""
    echo "目标容器: ${CONTAINER_NAME} (端口 8081)"
    echo ""
    echo "示例："
    echo "  ./deploy-test.sh build"
    echo "  ./deploy-test.sh deploy"
    echo "  ./deploy-test.sh deploy root@1.2.3.4 22"
    echo "  ./deploy-test.sh deploy root@1.2.3.4 22 0.1.0.999-dev-abc1234"
    ;;
  *)
    # 兼容旧用法：./deploy-test.sh [SERVER] [PORT] = build + deploy
    do_build
    do_deploy "${1:-}" "${2:-}" ""
    ;;
esac
