#!/bin/bash
# =============================================================================
# Sub2API Docker 镜像构建脚本
# =============================================================================
#
# 用法:
#   ./build_docker.sh [选项]
#
# 选项:
#   -t, --tag TAG          指定镜像标签 (默认: latest)
#   -r, --registry REG     指定镜像仓库地址 (默认: 无)
#   -b, --base-registry REG 指定基础镜像仓库地址 (默认: 使用 docker.io)
#   -p, --push             构建后推送镜像到仓库
#   --no-cache             不使用 Docker 构建缓存
#   -h, --help             显示帮助信息
#
# 示例:
#   ./build_docker.sh                           # 构建 sub2api:latest
#   ./build_docker.sh -t v1.0.0                 # 构建 sub2api:v1.0.0
#   ./build_docker.sh -r ghcr.io/user -p        # 构建并推送到 GitHub Container Registry
#   ./build_docker.sh -t v1.0.0 --no-cache      # 不使用缓存构建
#
# =============================================================================

set -e  # 遇到错误立即退出

# =============================================================================
# 颜色定义
# =============================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# =============================================================================
# 默认配置
# =============================================================================
IMAGE_NAME="sub2api"
TAG="latest"
REGISTRY=""
BASE_IMAGE_REGISTRY="${BASE_IMAGE_REGISTRY:-REDACTED"
PUSH=false
NO_CACHE=""

# =============================================================================
# 辅助函数
# =============================================================================

# 打印带颜色的信息
info() {
    echo -e "${BLUEREDACTED[INFO]${NCREDACTED $1"
REDACTED

success() {
    echo -e "${GREENREDACTED[SUCCESS]${NCREDACTED $1"
REDACTED

warn() {
    echo -e "${YELLOWREDACTED[WARN]${NCREDACTED $1"
REDACTED

error() {
    echo -e "${REDREDACTED[ERROR]${NCREDACTED $1"
    exit 1
REDACTED

# 显示帮助信息
show_help() {
    head -30 "$0" | tail -25
    exit 0
REDACTED

# =============================================================================
# 参数解析
# =============================================================================
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--tag)
            TAG="$2"
            shift 2
            ;;
        -r|--registry)
            REGISTRY="$2"
            shift 2
            ;;
        -b|--base-registry)
            BASE_IMAGE_REGISTRY="$2"
            shift 2
            ;;
        -p|--push)
            PUSH=true
            shift
            ;;
        --no-cache)
            NO_CACHE="--no-cache"
            shift
            ;;
        -h|--help)
            show_help
            ;;
        *)
            error "未知选项: $1\n使用 -h 或 --help 查看帮助"
            ;;
    esac
done

# =============================================================================
# 获取构建信息
# =============================================================================

# 获取 Git commit hash
get_commit() {
    if git rev-parse --git-dir > /dev/null 2>&1; then
        git rev-parse --short HEAD 2>/dev/null || echo "unknown"
    else
        echo "unknown"
    fi
REDACTED

# 获取版本号 (优先使用 git tag)
get_version() {
    if git rev-parse --git-dir > /dev/null 2>&1; then
        # 尝试获取最近的 tag
        local tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
        if [[ -n "$tag" ]]; then
            echo "$tag"
        else
            # 没有 tag，使用 commit hash
            echo "dev-$(get_commit)"
        fi
    else
        echo "dev"
    fi
REDACTED

# 获取构建日期 (ISO 8601 格式)
get_date() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
REDACTED

# =============================================================================
# 主逻辑
# =============================================================================

# 切换到项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]REDACTED")" && pwd)"
cd "$SCRIPT_DIR"

# 检查 Dockerfile 是否存在
if [[ ! -f "Dockerfile" ]]; then
    error "Dockerfile 不存在，请确保在项目根目录运行此脚本"
fi

# 获取构建参数
VERSION=$(get_version)
COMMIT=$(get_commit)
DATE=$(get_date)

# 解析基础镜像
NODE_IMAGE="node:24-alpine"
GOLANG_IMAGE="golang:1.25-alpine"
ALPINE_IMAGE="alpine:3.19"
if [[ -n "$BASE_IMAGE_REGISTRY" ]]; then
    BASE_IMAGE_REGISTRY="${BASE_IMAGE_REGISTRY%/REDACTED"
    NODE_IMAGE="${BASE_IMAGE_REGISTRYREDACTED/library/node:24-alpine"
    GOLANG_IMAGE="${BASE_IMAGE_REGISTRYREDACTED/library/golang:1.25-alpine"
    ALPINE_IMAGE="${BASE_IMAGE_REGISTRYREDACTED/library/alpine:3.19"
fi

# 构建完整镜像名称
if [[ -n "$REGISTRY" ]]; then
    FULL_IMAGE_NAME="${REGISTRYREDACTED/${IMAGE_NAMEREDACTED:${TAGREDACTED"
else
    FULL_IMAGE_NAME="${IMAGE_NAMEREDACTED:${TAGREDACTED"
fi

# 打印构建信息
echo ""
echo "=============================================="
echo "  Sub2API Docker 镜像构建"
echo "=============================================="
info "镜像名称: ${FULL_IMAGE_NAMEREDACTED"
info "版本: ${VERSIONREDACTED"
info "Commit: ${COMMITREDACTED"
info "构建时间: ${DATEREDACTED"
if [[ -n "$BASE_IMAGE_REGISTRY" ]]; then
    info "基础镜像仓库: ${BASE_IMAGE_REGISTRYREDACTED"
fi
if [[ -n "$NO_CACHE" ]]; then
    info "缓存: 禁用"
fi
echo "=============================================="
echo ""

# 执行构建
info "开始构建 Docker 镜像..."

docker build \
    ${NO_CACHEREDACTED \
    --build-arg VERSION="${VERSIONREDACTED" \
    --build-arg COMMIT="${COMMITREDACTED" \
    --build-arg DATE="${DATEREDACTED" \
    --build-arg NODE_IMAGE="${NODE_IMAGEREDACTED" \
    --build-arg GOLANG_IMAGE="${GOLANG_IMAGEREDACTED" \
    --build-arg ALPINE_IMAGE="${ALPINE_IMAGEREDACTED" \
    -t "${FULL_IMAGE_NAMEREDACTED" \
    -f Dockerfile \
    .

# 检查构建结果
if [[ $? -eq 0 ]]; then
    success "镜像构建成功: ${FULL_IMAGE_NAMEREDACTED"
else
    error "镜像构建失败"
fi

# 如果指定了 latest 以外的 tag，同时打上 latest 标签
if [[ "$TAG" != "latest" && -z "$REGISTRY" ]]; then
    info "同时标记为 latest..."
    docker tag "${FULL_IMAGE_NAMEREDACTED" "${IMAGE_NAMEREDACTED:latest"
fi

# 推送镜像
if [[ "$PUSH" == true ]]; then
    if [[ -z "$REGISTRY" ]]; then
        warn "未指定镜像仓库 (-r)，跳过推送"
    else
        info "推送镜像到仓库..."
        docker push "${FULL_IMAGE_NAMEREDACTED"

        # 如果不是 latest，也推送 latest 标签
        if [[ "$TAG" != "latest" ]]; then
            LATEST_IMAGE="${REGISTRYREDACTED/${IMAGE_NAMEREDACTED:latest"
            docker tag "${FULL_IMAGE_NAMEREDACTED" "${LATEST_IMAGEREDACTED"
            docker push "${LATEST_IMAGEREDACTED"
        fi

        success "镜像推送成功"
    fi
fi

# 显示镜像信息
echo ""
echo "=============================================="
info "构建完成！"
echo "=============================================="
echo ""
info "运行容器示例:"
echo "  docker run -d \\"
echo "    --name sub2api \\"
echo "    -p 8080:8080 \\"
echo "    -e AUTO_SETUP=true \\"
echo "    -e DATABASE_HOST=your-db-host \\"
echo "    -e DATABASE_PASSWORD=your-password \\"
echo "    -e REDIS_HOST=your-redis-host \\"
echo "    ${FULL_IMAGE_NAMEREDACTED"
echo ""
info "查看镜像大小:"
docker images "${IMAGE_NAMEREDACTED" --format "table {{.RepositoryREDACTEDREDACTED\t{{.TagREDACTEDREDACTED\t{{.SizeREDACTEDREDACTED"
echo ""
