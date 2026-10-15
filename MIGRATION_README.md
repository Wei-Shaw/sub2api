# Sub2API 迁移工具包

本目录包含从 AWS EC2 迁移到 Vultr 的完整自动化工具。

## 📦 文件清单

```
migration/
├── MIGRATION_GUIDE.md      # 完整迁移指南（详细文档）
├── QUICKSTART.md            # 快速入门（3步完成）
├── aws-backup.sh            # AWS 数据备份脚本
├── vultr-deploy.sh          # Vultr 一键部署脚本
├── restore-data.sh          # 数据恢复脚本
└── README.md                # 本文件
```

## 🚀 使用方法

### 方式一：在线执行（推荐）

```bash
# 1. AWS 备份（在 AWS EC2 上执行）
curl -o aws-backup.sh https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/aws-backup.sh
chmod +x aws-backup.sh
./aws-backup.sh

# 2. Vultr 部署（在 Vultr 新服务器上执行）
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/vultr-deploy.sh | bash

# 3. 数据恢复（在 Vultr 服务器上执行）
curl -o restore-data.sh https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/restore-data.sh
chmod +x restore-data.sh
./restore-data.sh backup-file.tar.gz
```

### 方式二：离线执行

```bash
# 1. 克隆仓库
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api

# 2. 使用本地脚本
chmod +x *.sh
./aws-backup.sh
./vultr-deploy.sh
./restore-data.sh backup-file.tar.gz
```

## ⚡ 快速开始

只需三步，25分钟完成迁移：

1. **AWS 备份**（5分钟）：导出数据库、配置、文件
2. **Vultr 部署**（10分钟）：自动安装 Docker、启动服务
3. **数据恢复**（5分钟）：导入所有数据

详细步骤请查看 [QUICKSTART.md](QUICKSTART.md)

## 📊 迁移收益

```
成本节省：
- AWS EC2: $70+/月
- Vultr: $24/月（前10个月免费用 $250 赠金）
- 年度节省: $552+

性能提升：
- 无 CPU 积分限制
- 持续稳定性能
- 更快的网络速度（东京机房）

运维简化：
- 固定价格计费
- 无复杂账单
- 简单易懂的管理界面
```

## ✅ 功能特性

### aws-backup.sh
- ✅ 自动检测部署方式（Docker/Binary）
- ✅ 导出 PostgreSQL 数据库
- ✅ 导出 Redis 数据（如果有）
- ✅ 打包配置文件和数据目录
- ✅ 生成压缩备份文件
- ✅ MD5 校验

### vultr-deploy.sh
- ✅ 更新系统包
- ✅ 安装 Docker 和 Docker Compose
- ✅ 配置防火墙（UFW）
- ✅ 生成安全密钥
- ✅ 创建配置文件
- ✅ 启动 Sub2API 服务
- ✅ 健康检查验证

### restore-data.sh
- ✅ 解压备份文件
- ✅ 清空现有数据库
- ✅ 导入数据库数据
- ✅ 恢复 Redis 数据
- ✅ 恢复配置和文件
- ✅ 保留新服务器密钥
- ✅ 自动重启服务

## 🛡️ 安全性

所有脚本都经过以下安全考虑：

- ✅ 使用 `set -e`，错误时立即退出
- ✅ 生成强随机密钥（32字节）
- ✅ 配置文件权限设置为 600
- ✅ 保留新服务器的 JWT/TOTP 密钥
- ✅ 防火墙自动配置
- ✅ 备份原始配置文件

## ⚠️ 注意事项

1. **备份重要性**
   - 迁移前务必备份
   - 本地保留备份副本
   - AWS 服务器保留 1-2 周

2. **密钥管理**
   - 新服务器会生成新的 JWT_SECRET
   - 这意味着现有 token 会失效
   - 用户需要重新登录

3. **DNS 切换**
   - DNS 传播需要时间（5-60分钟）
   - 切换期间新旧服务器可能并存
   - 提前降低 TTL 值

4. **测试验证**
   - 迁移后充分测试
   - 确认所有功能正常
   - 检查数据完整性

## 🐛 故障排查

### 脚本执行失败

```bash
# 检查网络连接
ping -c 3 github.com

# 手动下载脚本
wget https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/aws-backup.sh

# 查看详细错误
bash -x aws-backup.sh
```

### 服务启动失败

```bash
# 查看 Docker 日志
docker compose logs -f

# 检查端口占用
netstat -tlnp | grep 8080

# 检查磁盘空间
df -h

# 检查内存使用
free -h
```

### 数据库导入失败

```bash
# 检查数据库容器
docker compose ps postgres

# 查看 PostgreSQL 日志
docker compose logs postgres

# 手动测试连接
docker compose exec postgres psql -U sub2api
```

## 📖 文档说明

- **MIGRATION_GUIDE.md**: 完整的迁移指南，包含详细步骤、配置说明、常见问题
- **QUICKSTART.md**: 快速入门指南，3步完成迁移
- **本 README**: 工具包总览和使用说明

## 🔄 更新脚本

脚本会持续更新和改进，获取最新版本：

```bash
# 在线使用（自动获取最新版）
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/vultr-deploy.sh | bash

# 或更新本地仓库
git pull origin main
```

## 💡 最佳实践

1. **分阶段迁移**
   - 先部署测试环境验证
   - 确认无误后迁移生产环境
   - 保留 AWS 作为备份

2. **监控资源使用**
   - 部署后监控内存使用
   - 调整 PostgreSQL 配置
   - 必要时升级配置

3. **定期备份**
   - 启用 Vultr 自动备份
   - 或使用 cron 定时备份
   - 异地保存重要数据

4. **安全加固**
   - 修改默认端口
   - 配置 SSL 证书
   - 限制 SSH 登录
   - 启用双因素认证

## 🆘 获取帮助

- GitHub Issues: https://github.com/Wei-Shaw/sub2api/issues
- 项目文档: https://github.com/Wei-Shaw/sub2api
- 社区支持: [项目讨论区]

## 📝 许可证

与 Sub2API 主项目相同

## 🙏 致谢

感谢 Sub2API 项目作者和所有贡献者

---

**祝迁移顺利！** 🎉
