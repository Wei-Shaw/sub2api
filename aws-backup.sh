#!/bin/bash
#==============================================================================
# Sub2API AWS 数据备份脚本
# 用途：从 AWS EC2 导出所有数据
# 使用：./aws-backup.sh
#==============================================================================

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}Sub2API AWS 数据备份脚本${NC}"
echo -e "${GREEN}================================${NC}"
echo ""

# 生成备份文件名（带时间戳）
BACKUP_NAME="sub2api-backup-$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="/tmp/${BACKUP_NAME}"
BACKUP_FILE="${HOME}/${BACKUP_NAME}.tar.gz"

echo -e "${YELLOW}📦 备份目录：${BACKUP_DIR}${NC}"
echo -e "${YELLOW}📦 备份文件：${BACKUP_FILE}${NC}"
echo ""

# 创建备份目录
mkdir -p "${BACKUP_DIR}"

#==============================================================================
# 1. 检测 Sub2API 部署方式
#==============================================================================
echo -e "${GREEN}🔍 检测 Sub2API 部署方式...${NC}"

DEPLOY_METHOD=""
SUB2API_DIR=""

# 检测 Docker Compose 部署
if command -v docker &> /dev/null && docker ps | grep -q sub2api; then
    DEPLOY_METHOD="docker"
    echo -e "${GREEN}✅ 检测到 Docker Compose 部署${NC}"

    # 查找 docker-compose.yml 位置
    if [ -f "/opt/sub2api/docker-compose.yml" ]; then
        SUB2API_DIR="/opt/sub2api"
    elif [ -f "${HOME}/sub2api/deploy/docker-compose.yml" ]; then
        SUB2API_DIR="${HOME}/sub2api/deploy"
    elif [ -f "${HOME}/deploy/docker-compose.yml" ]; then
        SUB2API_DIR="${HOME}/deploy"
    else
        echo -e "${YELLOW}⚠️  未找到标准位置，请手动指定：${NC}"
        read -p "请输入 docker-compose.yml 所在目录: " SUB2API_DIR
    fi

# 检测二进制部署
elif systemctl is-active --quiet sub2api; then
    DEPLOY_METHOD="binary"
    SUB2API_DIR="/opt/sub2api"
    echo -e "${GREEN}✅ 检测到二进制部署${NC}"
else
    echo -e "${RED}❌ 未检测到运行中的 Sub2API${NC}"
    echo "请确保 Sub2API 正在运行"
    exit 1
fi

echo -e "${GREEN}📂 Sub2API 目录：${SUB2API_DIR}${NC}"
echo ""

#==============================================================================
# 2. 备份 PostgreSQL 数据库
#==============================================================================
echo -e "${GREEN}📊 备份 PostgreSQL 数据库...${NC}"

if [ "${DEPLOY_METHOD}" = "docker" ]; then
    # Docker 部署
    cd "${SUB2API_DIR}"

    # 检测使用的 compose 文件
    COMPOSE_FILE="docker-compose.yml"
    if [ -f "docker-compose.local.yml" ]; then
        COMPOSE_FILE="docker-compose.local.yml"
    fi

    echo "使用 Compose 文件: ${COMPOSE_FILE}"

    # 导出数据库
    docker compose -f "${COMPOSE_FILE}" exec -T postgres pg_dump \
        -U sub2api sub2api > "${BACKUP_DIR}/database.sql" || {
        echo -e "${YELLOW}⚠️  直接导出失败，尝试通过容器名...${NC}"
        docker exec sub2api-postgres pg_dump -U sub2api sub2api > "${BACKUP_DIR}/database.sql"
    }

    echo -e "${GREEN}✅ 数据库备份完成 ($(du -h ${BACKUP_DIR}/database.sql | cut -f1))${NC}"

else
    # 二进制部署
    sudo -u postgres pg_dump sub2api > "${BACKUP_DIR}/database.sql"
    echo -e "${GREEN}✅ 数据库备份完成${NC}"
fi
echo ""

#==============================================================================
# 3. 备份 Redis 数据（如果有）
#==============================================================================
echo -e "${GREEN}💾 备份 Redis 数据...${NC}"

if [ "${DEPLOY_METHOD}" = "docker" ]; then
    cd "${SUB2API_DIR}"

    # 检查 Redis 容器是否存在
    if docker ps | grep -q "sub2api-redis\|redis"; then
        # 触发 Redis 保存
        docker compose -f "${COMPOSE_FILE}" exec -T redis redis-cli SAVE || \
            docker exec sub2api-redis redis-cli SAVE

        # 查找 Redis 数据目录
        if [ -d "redis_data" ]; then
            cp -r redis_data "${BACKUP_DIR}/"
            echo -e "${GREEN}✅ Redis 数据备份完成${NC}"
        else
            echo -e "${YELLOW}⚠️  未找到 redis_data 目录，跳过${NC}"
        fi
    else
        echo -e "${YELLOW}⚠️  未检测到 Redis 容器，跳过${NC}"
    fi
else
    # 二进制部署
    if systemctl is-active --quiet redis; then
        redis-cli SAVE
        cp /var/lib/redis/dump.rdb "${BACKUP_DIR}/" 2>/dev/null || \
            echo -e "${YELLOW}⚠️  Redis 数据备份失败，跳过${NC}"
    else
        echo -e "${YELLOW}⚠️  Redis 未运行，跳过${NC}"
    fi
fi
echo ""

#==============================================================================
# 4. 备份配置文件
#==============================================================================
echo -e "${GREEN}⚙️  备份配置文件...${NC}"

mkdir -p "${BACKUP_DIR}/config"

if [ "${DEPLOY_METHOD}" = "docker" ]; then
    cd "${SUB2API_DIR}"

    # 备份 .env 文件
    if [ -f ".env" ]; then
        cp .env "${BACKUP_DIR}/config/"
        echo -e "${GREEN}✅ .env 已备份${NC}"
    fi

    # 备份 docker-compose 文件
    cp "${COMPOSE_FILE}" "${BACKUP_DIR}/config/" 2>/dev/null || true

    # 备份自定义配置
    if [ -f "config.yaml" ]; then
        cp config.yaml "${BACKUP_DIR}/config/"
        echo -e "${GREEN}✅ config.yaml 已备份${NC}"
    fi

else
    # 二进制部署
    if [ -f "/etc/sub2api/config.yaml" ]; then
        cp /etc/sub2api/config.yaml "${BACKUP_DIR}/config/"
        echo -e "${GREEN}✅ config.yaml 已备份${NC}"
    fi

    # 备份 systemd 服务文件
    cp /etc/systemd/system/sub2api.service "${BACKUP_DIR}/config/" 2>/dev/null || true
fi
echo ""

#==============================================================================
# 5. 备份数据目录
#==============================================================================
echo -e "${GREEN}📁 备份数据目录...${NC}"

if [ "${DEPLOY_METHOD}" = "docker" ]; then
    cd "${SUB2API_DIR}"

    # 备份 data 目录
    if [ -d "data" ]; then
        cp -r data "${BACKUP_DIR}/"
        echo -e "${GREEN}✅ data 目录已备份 ($(du -sh data | cut -f1))${NC}"
    fi

    # 备份 postgres_data（可选，数据库导出已包含数据）
    # if [ -d "postgres_data" ]; then
    #     cp -r postgres_data "${BACKUP_DIR}/"
    # fi

else
    # 二进制部署
    if [ -d "/opt/sub2api/data" ]; then
        cp -r /opt/sub2api/data "${BACKUP_DIR}/"
        echo -e "${GREEN}✅ data 目录已备份${NC}"
    fi
fi
echo ""

#==============================================================================
# 6. 创建备份元信息
#==============================================================================
echo -e "${GREEN}📝 创建备份元信息...${NC}"

cat > "${BACKUP_DIR}/backup-info.txt" <<EOF
Sub2API 备份信息
=====================================
备份时间: $(date '+%Y-%m-%d %H:%M:%S')
服务器: $(hostname)
部署方式: ${DEPLOY_METHOD}
部署目录: ${SUB2API_DIR}

包含内容:
- PostgreSQL 数据库导出 (database.sql)
- Redis 数据 (redis_data/)
- 配置文件 (config/)
- 数据目录 (data/)

恢复说明:
请参考 MIGRATION_GUIDE.md
=====================================
EOF

echo -e "${GREEN}✅ 备份信息已创建${NC}"
echo ""

#==============================================================================
# 7. 压缩备份文件
#==============================================================================
echo -e "${GREEN}🗜️  压缩备份文件...${NC}"

cd /tmp
tar -czf "${BACKUP_FILE}" "${BACKUP_NAME}/"

# 清理临时目录
rm -rf "${BACKUP_DIR}"

echo -e "${GREEN}✅ 压缩完成${NC}"
echo ""

#==============================================================================
# 8. 显示备份结果
#==============================================================================
echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}✅ 备份完成！${NC}"
echo -e "${GREEN}================================${NC}"
echo ""
echo -e "${YELLOW}📦 备份文件位置：${NC}"
echo -e "   ${BACKUP_FILE}"
echo ""
echo -e "${YELLOW}📊 文件大小：${NC}"
ls -lh "${BACKUP_FILE}" | awk '{print "   " $5}'
echo ""
echo -e "${YELLOW}🔍 文件校验（MD5）：${NC}"
md5sum "${BACKUP_FILE}" | awk '{print "   " $1}'
echo ""
echo -e "${GREEN}下一步：${NC}"
echo "1. 下载备份文件到本地："
echo "   ${YELLOW}scp $(whoami)@$(hostname -I | awk '{print $1}'):${BACKUP_FILE} ./${NC}"
echo ""
echo "2. 在 Vultr 新服务器上运行："
echo "   ${YELLOW}./restore-data.sh ${BACKUP_NAME}.tar.gz${NC}"
echo ""
