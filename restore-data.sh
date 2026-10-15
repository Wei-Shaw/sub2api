#!/bin/bash
#==============================================================================
# Sub2API 数据恢复脚本
# 用途：将 AWS 备份的数据导入到 Vultr 新服务器
# 使用：./restore-data.sh backup-file.tar.gz
#==============================================================================

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}Sub2API 数据恢复脚本${NC}"
echo -e "${GREEN}================================${NC}"
echo ""

# 检查是否为 root 用户
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}请使用 root 用户运行此脚本${NC}"
    echo "使用: sudo bash $0 backup-file.tar.gz"
    exit 1
fi

# 检查参数
if [ $# -eq 0 ]; then
    echo -e "${RED}错误：请提供备份文件${NC}"
    echo "使用: $0 backup-file.tar.gz"
    exit 1
fi

BACKUP_FILE="$1"

# 检查备份文件是否存在
if [ ! -f "$BACKUP_FILE" ]; then
    echo -e "${RED}错误：备份文件不存在: $BACKUP_FILE${NC}"
    exit 1
fi

echo -e "${BLUE}📦 备份文件: ${BACKUP_FILE}${NC}"
echo -e "${BLUE}📊 文件大小: $(du -h "$BACKUP_FILE" | cut -f1)${NC}"
echo ""

# 确认继续
echo -e "${YELLOW}⚠️  此操作将：${NC}"
echo "  1. 停止当前运行的 Sub2API 服务"
echo "  2. 清空现有数据库"
echo "  3. 导入备份数据"
echo "  4. 重启服务"
echo ""
read -p "是否继续？(y/N): " confirm
if [[ "$confirm" != "y" ]]; then
    echo "已取消"
    exit 0
fi
echo ""

# 设置目录
DEPLOY_DIR="/opt/sub2api"
RESTORE_DIR="/tmp/sub2api-restore-$$"

# 检查部署目录
if [ ! -d "$DEPLOY_DIR" ]; then
    echo -e "${RED}错误：Sub2API 部署目录不存在: $DEPLOY_DIR${NC}"
    echo "请先运行 vultr-deploy.sh 部署服务"
    exit 1
fi

cd "$DEPLOY_DIR"

#==============================================================================
# 1. 解压备份文件
#==============================================================================
echo -e "${GREEN}📂 解压备份文件...${NC}"

mkdir -p "$RESTORE_DIR"
tar -xzf "$BACKUP_FILE" -C "$RESTORE_DIR" --strip-components=1

echo -e "${GREEN}✅ 解压完成${NC}"
echo ""

# 显示备份信息
if [ -f "$RESTORE_DIR/backup-info.txt" ]; then
    echo -e "${BLUE}📝 备份信息：${NC}"
    cat "$RESTORE_DIR/backup-info.txt"
    echo ""
fi

#==============================================================================
# 2. 停止服务
#==============================================================================
echo -e "${GREEN}🛑 停止 Sub2API 服务...${NC}"

docker compose down

echo -e "${GREEN}✅ 服务已停止${NC}"
echo ""

#==============================================================================
# 3. 备份当前 .env（保留新服务器的密钥）
#==============================================================================
echo -e "${GREEN}💾 备份当前配置...${NC}"

if [ -f ".env" ]; then
    cp .env .env.backup
    echo -e "${GREEN}✅ 当前配置已备份到 .env.backup${NC}"
fi
echo ""

#==============================================================================
# 4. 导入数据库
#==============================================================================
echo -e "${GREEN}📊 导入数据库...${NC}"

if [ ! -f "$RESTORE_DIR/database.sql" ]; then
    echo -e "${RED}错误：备份中没有数据库文件${NC}"
    exit 1
fi

# 启动 PostgreSQL 容器
echo "启动 PostgreSQL 容器..."
docker compose up -d postgres

# 等待 PostgreSQL 就绪
echo "等待 PostgreSQL 就绪..."
sleep 10

MAX_WAIT=60
WAITED=0
while [ $WAITED -lt $MAX_WAIT ]; do
    if docker compose exec -T postgres pg_isready -U sub2api > /dev/null 2>&1; then
        echo -e "${GREEN}✅ PostgreSQL 已就绪${NC}"
        break
    fi
    echo -n "."
    sleep 2
    WAITED=$((WAITED + 2))
done

if [ $WAITED -ge $MAX_WAIT ]; then
    echo -e "${RED}PostgreSQL 启动超时${NC}"
    exit 1
fi

# 删除现有数据库并重建
echo "清空现有数据库..."
docker compose exec -T postgres psql -U sub2api -c "DROP DATABASE IF EXISTS sub2api;" postgres
docker compose exec -T postgres psql -U sub2api -c "CREATE DATABASE sub2api;" postgres

# 导入数据
echo "导入数据库备份..."
cat "$RESTORE_DIR/database.sql" | docker compose exec -T postgres psql -U sub2api sub2api

echo -e "${GREEN}✅ 数据库导入完成${NC}"
echo ""

#==============================================================================
# 5. 导入 Redis 数据（如果有）
#==============================================================================
echo -e "${GREEN}💾 导入 Redis 数据...${NC}"

if [ -d "$RESTORE_DIR/redis_data" ]; then
    # 停止 Redis 容器
    docker compose down redis 2>/dev/null || true

    # 清空现有 Redis 数据
    rm -rf redis_data/*

    # 复制备份数据
    cp -r "$RESTORE_DIR/redis_data/"* redis_data/ 2>/dev/null || true

    echo -e "${GREEN}✅ Redis 数据导入完成${NC}"
else
    echo -e "${YELLOW}⚠️  备份中没有 Redis 数据，跳过${NC}"
fi
echo ""

#==============================================================================
# 6. 恢复配置文件（选择性）
#==============================================================================
echo -e "${GREEN}⚙️  处理配置文件...${NC}"

if [ -d "$RESTORE_DIR/config" ]; then
    echo -e "${YELLOW}发现备份的配置文件${NC}"

    # 读取旧配置中的重要信息
    if [ -f "$RESTORE_DIR/config/.env" ]; then
        echo "提取旧配置中的业务配置..."

        # 提取管理员邮箱（如果存在）
        OLD_ADMIN_EMAIL=$(grep "^ADMIN_EMAIL=" "$RESTORE_DIR/config/.env" | cut -d'=' -f2 || echo "")

        # 保持新服务器的密钥，但可以更新其他配置
        if [ -n "$OLD_ADMIN_EMAIL" ] && [ "$OLD_ADMIN_EMAIL" != "admin@sub2api.local" ]; then
            echo "发现自定义管理员邮箱: $OLD_ADMIN_EMAIL"
            sed -i "s/^ADMIN_EMAIL=.*/ADMIN_EMAIL=$OLD_ADMIN_EMAIL/" .env
        fi
    fi

    # 恢复自定义配置文件（如果有）
    if [ -f "$RESTORE_DIR/config/config.yaml" ]; then
        cp "$RESTORE_DIR/config/config.yaml" data/config.yaml
        echo -e "${GREEN}✅ 自定义配置文件已恢复${NC}"
    fi

else
    echo -e "${YELLOW}⚠️  备份中没有配置文件，使用新服务器配置${NC}"
fi
echo ""

#==============================================================================
# 7. 恢复数据目录
#==============================================================================
echo -e "${GREEN}📁 恢复数据目录...${NC}"

if [ -d "$RESTORE_DIR/data" ]; then
    # 备份现有 data 目录（如果有重要文件）
    if [ -d "data" ] && [ "$(ls -A data)" ]; then
        mv data "data.backup.$(date +%Y%m%d-%H%M%S)"
    fi

    # 恢复 data 目录
    cp -r "$RESTORE_DIR/data" ./

    echo -e "${GREEN}✅ 数据目录已恢复${NC}"
else
    echo -e "${YELLOW}⚠️  备份中没有 data 目录，跳过${NC}"
fi
echo ""

#==============================================================================
# 8. 启动服务
#==============================================================================
echo -e "${GREEN}🚀 启动所有服务...${NC}"

docker compose up -d

echo -e "${GREEN}✅ 服务启动中...${NC}"
echo ""

#==============================================================================
# 9. 等待服务就绪
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
# 10. 清理临时文件
#==============================================================================
echo -e "${GREEN}🧹 清理临时文件...${NC}"

rm -rf "$RESTORE_DIR"

echo -e "${GREEN}✅ 清理完成${NC}"
echo ""

#==============================================================================
# 11. 显示恢复结果
#==============================================================================
echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}✅ 数据恢复完成！${NC}"
echo -e "${GREEN}================================${NC}"
echo ""

# 获取服务器 IP
SERVER_IP=$(curl -s ifconfig.me || hostname -I | awk '{print $1}')

echo -e "${BLUE}📊 服务状态：${NC}"
docker compose ps
echo ""

echo -e "${BLUE}🌐 访问信息：${NC}"
echo -e "  Web 界面: ${GREEN}http://${SERVER_IP}:8080${NC}"
echo ""

echo -e "${BLUE}🔍 数据验证：${NC}"
echo "  1. 访问 Web 界面"
echo "  2. 使用原来的管理员账号登录"
echo "  3. 检查用户数据是否完整"
echo "  4. 检查 API Keys 是否可用"
echo "  5. 检查配额和统计数据"
echo ""

echo -e "${BLUE}📁 备份文件：${NC}"
echo "  配置备份: ${DEPLOY_DIR}/.env.backup"
if [ -d "${DEPLOY_DIR}/data.backup."* ]; then
    echo "  数据备份: ${DEPLOY_DIR}/data.backup.*"
fi
echo ""

echo -e "${YELLOW}💡 提示：${NC}"
echo "  - 如果遇到问题，可以查看日志: docker compose logs -f"
echo "  - 备份文件已保留: $BACKUP_FILE"
echo "  - 配置备份已保存: .env.backup"
echo ""

echo -e "${GREEN}🎉 恢复成功！${NC}"
