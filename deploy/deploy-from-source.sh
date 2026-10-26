#!/usr/bin/env bash
# =============================================================================
# Sub2API 源码一键部署脚本（Ubuntu）
# =============================================================================
# 在已 clone 的仓库中运行，完成：
#   1. 安装 Docker Engine + docker compose 插件（已安装则跳过）
#   2. 首次运行时生成 .env（随机密钥）和数据目录；已存在则保留不动
#   3. 检查 Nginx 证书（可选 --self-signed 生成自签证书用于测试）
#   4. 从源码构建 aibridge 镜像并启动全部服务，等待健康检查通过
#
# 用法：
#   sudo bash deploy/deploy-from-source.sh [选项]
#
# 选项：
#   --pull                 部署前先 git pull 更新代码
#   --self-signed          证书缺失时生成自签证书（仅测试用）
#   --skip-docker-install  不安装 Docker（已自行安装时使用）
#   -h, --help             显示帮助
#
# 环境变量：
#   DOMAIN                  首次生成 .env 时写入的域名（不设则交互询问）
#   DOCKER_APT_MIRROR       Docker apt 源，默认 https://download.docker.com
#                           国内可用 https://mirrors.aliyun.com/docker-ce
#   DOCKER_REGISTRY_MIRROR  Docker Hub 镜像加速地址，仅在 /etc/docker/daemon.json
#                           不存在时写入（构建需拉取 node/golang/alpine 等镜像）
#
# 首次部署示例（国内服务器）：
#   sudo DOMAIN=api.example.com \
#        DOCKER_APT_MIRROR=https://mirrors.aliyun.com/docker-ce \
#        DOCKER_REGISTRY_MIRROR=https://<your-mirror> \
#        bash deploy/deploy-from-source.sh
#
# 后续更新：
#   sudo bash deploy/deploy-from-source.sh --pull
# =============================================================================

# Re-exec under bash when started via `sh script.sh` (dash on Ubuntu lacks
# pipefail, arrays, [[ ]] and BASH_SOURCE used below)
if [ -z "${BASH_VERSION:-}" ]; then
    exec bash "$0" "$@"
fi

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="docker-compose.local.yml"

DOCKER_APT_MIRROR="${DOCKER_APT_MIRROR:-https://download.docker.com}"
DOCKER_REGISTRY_MIRROR="${DOCKER_REGISTRY_MIRROR:-}"

OPT_PULL=false
OPT_SELF_SIGNED=false
OPT_SKIP_DOCKER_INSTALL=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

generate_secret() {
    openssl rand -hex 32
}

usage() {
    sed -n '2,36p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

compose() {
    docker compose -f "${SCRIPT_DIR}/${COMPOSE_FILE}" --project-directory "${SCRIPT_DIR}" "$@"
}

# Read a KEY=value from .env (last occurrence wins, surrounding quotes stripped)
env_value() {
    grep -E "^$1=" "${SCRIPT_DIR}/.env" 2>/dev/null | tail -n 1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//'
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --pull) OPT_PULL=true ;;
            --self-signed) OPT_SELF_SIGNED=true ;;
            --skip-docker-install) OPT_SKIP_DOCKER_INSTALL=true ;;
            -h|--help) usage; exit 0 ;;
            *) print_error "未知选项: $1"; usage; exit 1 ;;
        esac
        shift
    done
}

require_root() {
    if [ "${EUID}" -ne 0 ]; then
        if command_exists sudo; then
            print_info "需要 root 权限，使用 sudo 重新执行..."
            exec sudo -E bash "${BASH_SOURCE[0]}" "$@"
        fi
        print_error "请使用 root 或 sudo 运行此脚本。"
        exit 1
    fi
}

check_os() {
    if [ ! -f /etc/os-release ]; then
        print_error "无法识别操作系统（缺少 /etc/os-release）。"
        exit 1
    fi
    # shellcheck disable=SC1091
    . /etc/os-release
    if [ "${ID:-}" != "ubuntu" ]; then
        print_warning "当前系统为 ${PRETTY_NAME:-unknown}，此脚本仅在 Ubuntu 上测试。"
        if [ "${OPT_SKIP_DOCKER_INSTALL}" = false ]; then
            print_error "非 Ubuntu 系统请自行安装 Docker 后使用 --skip-docker-install。"
            exit 1
        fi
    fi
}

install_base_packages() {
    local missing=()
    for pkg in ca-certificates curl gnupg openssl git; do
        dpkg -s "${pkg}" >/dev/null 2>&1 || missing+=("${pkg}")
    done
    if [ ${#missing[@]} -gt 0 ]; then
        print_info "安装基础依赖: ${missing[*]}"
        apt-get update -y
        DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing[@]}"
    fi
}

install_docker() {
    if command_exists docker && docker compose version >/dev/null 2>&1; then
        print_success "Docker 已安装: $(docker --version)"
        return
    fi

    print_info "安装 Docker Engine（源: ${DOCKER_APT_MIRROR}）..."

    # Remove distro-packaged Docker variants that conflict with docker-ce
    for pkg in docker.io docker-doc docker-compose docker-compose-v2 podman-docker containerd runc; do
        if dpkg -s "${pkg}" >/dev/null 2>&1; then
            apt-get remove -y "${pkg}"
        fi
    done

    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL "${DOCKER_APT_MIRROR}/linux/ubuntu/gpg" -o /etc/apt/keyrings/docker.asc
    chmod a+r /etc/apt/keyrings/docker.asc

    # shellcheck disable=SC1091
    local codename
    codename="$(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")"
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] ${DOCKER_APT_MIRROR}/linux/ubuntu ${codename} stable" \
        > /etc/apt/sources.list.d/docker.list

    apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y \
        docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    systemctl enable --now docker
    print_success "Docker 安装完成: $(docker --version)"
}

configure_registry_mirror() {
    [ -n "${DOCKER_REGISTRY_MIRROR}" ] || return 0

    if [ -f /etc/docker/daemon.json ]; then
        print_warning "/etc/docker/daemon.json 已存在，未写入镜像加速，请手动添加 registry-mirrors。"
        return
    fi

    print_info "配置 Docker Hub 镜像加速: ${DOCKER_REGISTRY_MIRROR}"
    mkdir -p /etc/docker
    cat > /etc/docker/daemon.json <<EOF
{
  "registry-mirrors": ["${DOCKER_REGISTRY_MIRROR}"],
  "log-driver": "json-file",
  "log-opts": { "max-size": "100m", "max-file": "3" }
}
EOF
    systemctl restart docker
    print_success "镜像加速已配置"
}

update_source() {
    [ "${OPT_PULL}" = true ] || return 0

    # Pull as the repo owner so root does not take ownership of .git objects
    local owner
    owner="$(stat -c %U "${REPO_ROOT}")"
    print_info "更新代码 (git pull，用户: ${owner})..."
    sudo -u "${owner}" git -C "${REPO_ROOT}" pull --ff-only
    print_success "代码已更新: $(sudo -u "${owner}" git -C "${REPO_ROOT}" log --oneline -1)"
}

prepare_env() {
    cd "${SCRIPT_DIR}"

    if [ -f .env ]; then
        print_info ".env 已存在，保留现有配置。"
    else
        print_info "生成 .env 及随机密钥..."
        local domain="${DOMAIN:-}"
        if [ -z "${domain}" ] && [ -t 0 ]; then
            read -r -p "请输入访问域名（如 api.example.com，留空则匹配任意 Host）: " domain
        fi

        cp .env.example .env
        sed -i "s|^JWT_SECRET=.*|JWT_SECRET=$(generate_secret)|" .env
        sed -i "s|^TOTP_ENCRYPTION_KEY=.*|TOTP_ENCRYPTION_KEY=$(generate_secret)|" .env
        sed -i "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$(generate_secret)|" .env
        sed -i "s|^DOMAIN=.*|DOMAIN=${domain}|" .env
        chmod 600 .env
        print_success ".env 已生成（密钥仅保存在此文件中，请妥善备份）"
    fi

    mkdir -p data postgres_data redis_data nginx/certs
}

prepare_certs() {
    cd "${SCRIPT_DIR}/nginx/certs"

    if [ -f fullchain.pem ] && [ -f privkey.pem ]; then
        chmod 600 privkey.pem
        print_success "已找到 TLS 证书"
        return
    fi

    if [ "${OPT_SELF_SIGNED}" = false ]; then
        print_error "缺少 TLS 证书，Nginx 无法启动。请放置以下文件后重新运行："
        echo "  ${SCRIPT_DIR}/nginx/certs/fullchain.pem"
        echo "  ${SCRIPT_DIR}/nginx/certs/privkey.pem"
        echo "或使用 --self-signed 生成自签证书（仅测试用）。"
        exit 1
    fi

    local domain
    domain="$(env_value DOMAIN)"
    [ -n "${domain}" ] || domain="localhost"
    print_warning "生成自签证书（CN=${domain}），浏览器会提示不安全，仅用于测试。"
    openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
        -keyout privkey.pem -out fullchain.pem \
        -subj "/CN=${domain}" \
        -addext "subjectAltName=DNS:${domain}" >/dev/null 2>&1
    chmod 600 privkey.pem
    chmod 644 fullchain.pem
}

build_and_start() {
    print_info "从源码构建镜像（首次需下载依赖，耗时较长）..."
    bash "${SCRIPT_DIR}/build_image.sh"
    print_success "镜像构建完成"

    print_info "启动服务..."
    # --force-recreate: avoid reusing stale containers (old image / broken network state)
    compose up -d --remove-orphans --force-recreate
}

wait_healthy() {
    local port
    port="$(env_value SERVER_PORT)"
    port="${port:-8080}"

    print_info "等待服务就绪（最多 180 秒）..."
    # /health only accepts GET, so do not probe it with HEAD (curl -I)
    for _ in $(seq 1 60); do
        if curl -fsS -o /dev/null "http://127.0.0.1:${port}/health"; then
            print_success "后端健康检查通过"
            compose ps
            return
        fi
        sleep 3
    done

    print_error "服务未在预期时间内就绪，最近日志如下："
    compose ps
    compose logs --tail 50 aibridge
    exit 1
}

print_summary() {
    local domain port
    domain="$(env_value DOMAIN)"
    port="$(env_value SERVER_PORT)"

    echo ""
    echo "=========================================="
    echo "  部署完成"
    echo "=========================================="
    echo ""
    [ -n "${domain}" ] && echo "  访问地址:   https://${domain}"
    echo "  直连后端:   http://<server-ip>:${port:-8080}  (明文 HTTP，仅限可信 IP)"
    echo ""
    if [ -z "$(env_value ADMIN_PASSWORD)" ]; then
        echo "  .env 未设置 ADMIN_PASSWORD，首次启动会自动生成，查看："
        echo "    docker compose -f ${SCRIPT_DIR}/${COMPOSE_FILE} logs aibridge | grep -i password"
        echo ""
    fi
    echo "  常用命令（在 ${SCRIPT_DIR} 下执行）:"
    echo "    docker compose -f ${COMPOSE_FILE} ps"
    echo "    docker compose -f ${COMPOSE_FILE} logs -f aibridge"
    echo "    docker compose -f ${COMPOSE_FILE} down"
    echo ""
    print_warning "防火墙：开放 80/443；8080 仅对管理 IP 开放；不要开放 5432/6379。"
    echo ""
}

main() {
    parse_args "$@"
    require_root "$@"
    check_os

    echo ""
    echo "=========================================="
    echo "  Sub2API Deploy From Source"
    echo "=========================================="
    echo ""

    if [ "${OPT_SKIP_DOCKER_INSTALL}" = false ]; then
        install_base_packages
        install_docker
    elif ! docker compose version >/dev/null 2>&1; then
        print_error "未检测到 docker compose 插件。"
        exit 1
    fi
    configure_registry_mirror

    update_source
    prepare_env
    prepare_certs
    build_and_start
    wait_healthy
    print_summary
}

main "$@"
