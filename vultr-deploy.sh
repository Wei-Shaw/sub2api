#!/bin/bash
#==============================================================================
# Sub2API Vultr 一键部署脚本
# 用途：在全新的 Vultr Ubuntu 22.04 服务器上自动部署 Sub2API
# 使用：curl -sSL https://raw.githubusercontent.com/YOUR_REPO/vultr-deploy.sh | bash
#==============================================================================

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}Sub2API Vultr 自动化部署${NC}"
echo -e "${GREEN}================================${NC}"
echo ""

# 检查是否为 root 用户
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}请使用 root 用户运行此脚本${NC}"
    echo "使用: sudo bash $0"
    exit 1
fi

# 检查系统版本
if [ ! -f /etc/os-release ]; then
    echo -e "${RED}无法检测系统版本${NC}"
    exit 1
fi

source /etc/os-release
if [[ "$ID" != "ubuntu" ]]; then
    echo -e "${YELLOW}警告：此脚本针对 Ubuntu 优化，当前系统：$ID${NC}"
    read -p "是否继续？(y/N): " confirm
    if [[ "$confirm" != "y" ]]; then
        exit 1
    fi
fi

echo -e "${BLUE}系统信息：${NC}"
echo "  操作系统: $NAME $VERSION"
echo "  内核版本: $(uname -r)"
echo "  CPU 核心: $(nproc)"
echo "  内存大小: $(free -h | awk '/^Mem:/ {print $2}')"
echo ""

#==============================================================================
# 1. 更新系统
#==============================================================================
echo -e "${GREEN}📦 更新系统包...${NC}"
apt update -qq
apt upgrade -y -qq
echo -e "${GREEN}✅ 系统更新完成${NC}"
echo ""

#==============================================================================
# 2. 安装基础工具
#==============================================================================
echo -e "${GREEN}🔧 安装基础工具...${NC}"
apt install -y -qq \
    curl \
    wget \
    git \
    vim \
    htop \
    net-tools \
    ufw \
    ca-certificates \
    gnupg \
    lsb-release

echo -e "${GREEN}✅ 基础工具安装完成${NC}"
echo ""

#==============================================================================
# 3. 安装 Docker
#==============================================================================
echo -e "${GREEN}🐳 安装 Docker...${NC}"

# 检查 Docker 是否已安装
if command -v docker &> /dev/null; then
    echo -e "${YELLOW}Docker 已安装，跳过${NC}"
else
    # 添加 Docker 官方 GPG 密钥
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    # 添加 Docker 仓库
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

    # 安装 Docker
    apt update -qq
    apt install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

    # 启动 Docker
    systemctl enable docker
    systemctl start docker

    echo -e "${GREEN}✅ Docker 安装完成${NC}"
    docker --version
fi
echo ""

#==============================================================================
# 4. 配置防火墙
#==============================================================================
echo -e "${GREEN}🔥 配置防火墙...${NC}"

# 允许 SSH
ufw allow 22/tcp comment 'SSH'

# 允许 HTTP/HTTPS
ufw allow 80/tcp comment 'HTTP'
ufw allow 443/tcp comment 'HTTPS'

# 允许 Sub2API 默认端口
ufw allow 8080/tcp comment 'Sub2API'

# 启用防火墙
echo "y" | ufw enable

ufw status

echo -e "${GREEN}✅ 防火墙配置完成${NC}"
echo ""

#==============================================================================
# 5. 创建部署目录
#==============================================================================
echo -e "${GREEN}📁 创建部署目录...${NC}"

DEPLOY_DIR="/opt/sub2api"
mkdir -p "${DEPLOY_DIR}"
cd "${DEPLOY_DIR}"

echo -e "${GREEN}✅ 部署目录：${DEPLOY_DIR}${NC}"
echo ""

#==============================================================================
# 6. 下载 Sub2API Docker Compose 配置
#==============================================================================
echo -e "${GREEN}📥 下载 Sub2API 配置文件...${NC}"

# 下载 docker-compose.local.yml
curl -sSL -o docker-compose.yml \
    https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-compose.local.yml

# 下载 .env.example
curl -sSL -o .env.example \
    https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/.env.example

echo -e "${GREEN}✅ 配置文件下载完成${NC}"
echo ""

#==============================================================================
# 7. 生成配置文件
#==============================================================================
echo -e "${GREEN}⚙️  生成配置文件...${NC}"

# 复制 .env.example 到 .env
cp .env.example .env

# 生成随机密钥
generate_secret() {
    openssl rand -hex 32
}

POSTGRES_PASSWORD=$(generate_secret)
JWT_SECRET=$(generate_secret)
TOTP_ENCRYPTION_KEY=$(generate_secret)
ADMIN_PASSWORD=$(openssl rand -base64 16)

# 写入配置
cat > .env <<EOF
# PostgreSQL 配置
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=sub2api

# Redis 配置
REDIS_PASSWORD=

# 服务器配置
SERVER_PORT=8080
SERVER_MODE=release
TZ=Asia/Shanghai

# 管理员账号
ADMIN_EMAIL=admin@sub2api.local
ADMIN_PASSWORD=${ADMIN_PASSWORD}

# JWT 配置
JWT_SECRET=${JWT_SECRET}
JWT_EXPIRE_HOUR=24

# TOTP 加密密钥
TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY}

# 自动设置
AUTO_SETUP=true

# PostgreSQL 性能优化
POSTGRES_MAX_CONNECTIONS=100
POSTGRES_SHARED_BUFFERS=256MB
POSTGRES_EFFECTIVE_CACHE_SIZE=1GB
POSTGRES_MAINTENANCE_WORK_MEM=128MB
EOF

chmod 600 .env

echo -e "${GREEN}✅ 配置文件已生成${NC}"
echo ""
echo -e "${YELLOW}📝 重要信息（请保存）：${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "数据库密码: ${POSTGRES_PASSWORD}"
echo -e "JWT 密钥:   ${JWT_SECRET}"
echo -e "TOTP 密钥:  ${TOTP_ENCRYPTION_KEY}"
echo -e "管理员邮箱: admin@sub2api.local"
echo -e "管理员密码: ${ADMIN_PASSWORD}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${RED}⚠️  请立即保存以上信息！${NC}"
read -p "按回车继续..."
echo ""

#==============================================================================
# 8. 创建数据目录
#==============================================================================
echo -e "${GREEN}📂 创建数据目录...${NC}"

mkdir -p data postgres_data redis_data

echo -e "${GREEN}✅ 数据目录已创建${NC}"
echo ""

#==============================================================================
# 9. 启动 Sub2API 服务
#==============================================================================
echo -e "${GREEN}🚀 启动 Sub2API 服务...${NC}"

# 拉取镜像
docker compose pull

# 启动服务
docker compose up -d

echo -e "${GREEN}✅ 服务启动中...${NC}"
echo ""

#==============================================================================
# 10. 等待服务就绪
#==============================================================================
echo -e "${GREEN}⏳ 等待服务就绪...${NC}"

MAX_WAIT=120
WAITED=0

while [ $WAITED -lt $MAX_WAIT ]; do
    if curl -sf http://localhost:8080/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ 服务已就绪！${NC}"
        break
    fi

    echo -n "."
    sleep 2
    WAITED=$((WAITED + 2))
done

if [ $WAITED -ge $MAX_WAIT ]; then
    echo -e "${RED}⚠️  服务启动超时，请检查日志${NC}"
    echo "查看日志: docker compose logs -f sub2api"
fi
echo ""

#==============================================================================
# 11. 显示部署结果
#==============================================================================
echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}✅ 部署完成！${NC}"
echo -e "${GREEN}================================${NC}"
echo ""

# 获取服务器 IP
SERVER_IP=$(curl -s ifconfig.me || hostname -I | awk '{print $1}')

echo -e "${BLUE}📊 服务状态：${NC}"
docker compose ps
echo ""

echo -e "${BLUE}🌐 访问信息：${NC}"
echo -e "  Web 界面: ${GREEN}http://${SERVER_IP}:8080${NC}"
echo -e "  健康检查: ${GREEN}http://${SERVER_IP}:8080/health${NC}"
echo ""

echo -e "${BLUE}👤 登录信息：${NC}"
echo -e "  邮箱: ${GREEN}admin@sub2api.local${NC}"
echo -e "  密码: ${GREEN}${ADMIN_PASSWORD}${NC}"
echo ""

echo -e "${BLUE}📁 部署目录：${NC}"
echo -e "  ${DEPLOY_DIR}"
echo ""

echo -e "${BLUE}🔧 常用命令：${NC}"
echo "  查看日志:   docker compose logs -f sub2api"
echo "  重启服务:   docker compose restart"
echo "  停止服务:   docker compose down"
echo "  查看状态:   docker compose ps"
echo ""

echo -e "${YELLOW}📝 下一步：${NC}"
echo "1. 使用浏览器访问 http://${SERVER_IP}:8080"
echo "2. 使用上述管理员账号登录"
echo "3. 如需导入数据，运行: ./restore-data.sh backup-file.tar.gz"
echo "4. 如需配置域名和 SSL，参考 MIGRATION_GUIDE.md"
echo ""

echo -e "${GREEN}🎉 祝使用愉快！${NC}"
echo ""

# 保存部署信息到文件
cat > "${DEPLOY_DIR}/DEPLOY_INFO.txt" <<EOF
Sub2API 部署信息
=====================================
部署时间: $(date '+%Y-%m-%d %H:%M:%S')
服务器 IP: ${SERVER_IP}
部署目录: ${DEPLOY_DIR}

访问地址: http://${SERVER_IP}:8080
管理员邮箱: admin@sub2api.local
管理员密码: ${ADMIN_PASSWORD}

数据库密码: ${POSTGRES_PASSWORD}
JWT 密钥: ${JWT_SECRET}
TOTP 密钥: ${TOTP_ENCRYPTION_KEY}

配置文件: ${DEPLOY_DIR}/.env
Docker Compose: ${DEPLOY_DIR}/docker-compose.yml
=====================================
EOF

chmod 600 "${DEPLOY_DIR}/DEPLOY_INFO.txt"

echo -e "${YELLOW}💾 部署信息已保存到: ${DEPLOY_DIR}/DEPLOY_INFO.txt${NC}"
