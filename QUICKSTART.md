# Sub2API 迁移快速入门

## 🚀 三步完成迁移

### 准备工作
- ✅ Vultr 服务器已部署（Tokyo, vhp-2c-4gb, Ubuntu 22.04）
- ✅ 本地电脑能 SSH 登录 AWS 和 Vultr
- ✅ 磁盘空间充足（备份文件可能几百 MB）

---

## 第一步：备份 AWS 数据（5分钟）

```bash
# 1. SSH 登录到 AWS EC2
ssh -i your-aws-key.pem ubuntu@your-aws-ip

# 2. 下载并运行备份脚本
curl -o aws-backup.sh https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/aws-backup.sh
chmod +x aws-backup.sh
./aws-backup.sh

# 3. 下载备份文件到本地（在本地电脑执行）
scp -i your-aws-key.pem ubuntu@your-aws-ip:~/sub2api-backup-*.tar.gz ./
```

**完成标志：** 本地有一个 `sub2api-backup-YYYYMMDD-HHMMSS.tar.gz` 文件

---

## 第二步：部署 Vultr 服务器（10分钟）

```bash
# 1. SSH 登录到 Vultr 新服务器
ssh root@your-vultr-ip
# 输入邮件中的密码，首次登录会提示修改

# 2. 运行一键部署脚本
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/vultr-deploy.sh | bash

# 3. 记录显示的管理员密码和重要信息
```

**完成标志：** 浏览器访问 `http://your-vultr-ip:8080` 能看到登录页面

---

## 第三步：导入数据（5分钟）

```bash
# 1. 上传备份文件到 Vultr（在本地电脑执行）
scp sub2api-backup-*.tar.gz root@your-vultr-ip:/root/

# 2. SSH 登录到 Vultr
ssh root@your-vultr-ip

# 3. 下载并运行恢复脚本
curl -o restore-data.sh https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/restore-data.sh
chmod +x restore-data.sh
./restore-data.sh /root/sub2api-backup-*.tar.gz
```

**完成标志：** 使用原来的账号能登录，数据完整

---

## ✅ 验证迁移成功

访问 `http://your-vultr-ip:8080`，检查：

- [ ] 能够登录（使用原来的账号密码）
- [ ] 用户列表完整
- [ ] API Keys 正常工作
- [ ] 统计数据正确
- [ ] 所有功能正常

---

## 🌐 切换域名（可选）

如果使用域名访问：

1. 在域名服务商（阿里云/Cloudflare）修改 A 记录
2. 将域名指向新 IP：`your-vultr-ip`
3. 等待 DNS 传播（5-60分钟）
4. 测试访问：`curl https://your-domain.com/health`

---

## 📊 成本节省

```
迁移前（AWS）：$70+/月
迁移后（Vultr）：$0/月（前10个月用赠金）
                  $24/月（之后）

年度节省：$552+
```

---

## 🆘 遇到问题？

### 脚本下载失败
```bash
# 方法1：使用项目仓库中的脚本
# 脚本已保存在项目根目录

# 方法2：手动创建脚本文件
# 复制脚本内容，粘贴到文件中
nano aws-backup.sh
# 粘贴内容，Ctrl+O 保存，Ctrl+X 退出
chmod +x aws-backup.sh
```

### 服务无法访问
```bash
# 检查容器状态
docker compose ps

# 查看日志
docker compose logs -f sub2api

# 检查防火墙
ufw status

# 检查端口
netstat -tlnp | grep 8080
```

### 数据库导入失败
```bash
# 查看 PostgreSQL 日志
docker compose logs postgres

# 手动导入
docker compose exec -T postgres psql -U sub2api sub2api < database.sql
```

### 无法登录
```bash
# 检查管理员密码
cat /opt/sub2api/DEPLOY_INFO.txt

# 或查看日志中的自动生成密码
docker compose logs sub2api | grep "admin password"
```

---

## 📞 支持

- 项目文档：`MIGRATION_GUIDE.md`
- GitHub Issues: https://github.com/Wei-Shaw/sub2api/issues
- 查看完整日志：`docker compose logs -f`

---

## ⏰ 时间表

| 步骤 | 时间 | 说明 |
|------|------|------|
| AWS 备份 | 5分钟 | 导出数据库和配置 |
| Vultr 部署 | 10分钟 | 自动安装所有依赖 |
| 数据导入 | 5分钟 | 恢复数据和配置 |
| 验证测试 | 5分钟 | 确认功能正常 |
| DNS 切换 | 即时 | 传播需5-60分钟 |
| **总计** | **25分钟** | 实际操作时间 |

---

## 🎯 迁移后优化（可选）

### 1. 配置 SSL 证书
```bash
# 安装 Certbot
apt install -y certbot

# 获取证书
certbot certonly --standalone -d your-domain.com
```

### 2. 启用自动备份
```bash
# Vultr 控制面板
# Settings → Backups → Enable Automatic Backups
# 费用：+$4.8/月
```

### 3. 设置监控告警
```bash
# 安装监控工具
apt install -y htop iotop

# 或使用 Vultr 内置监控
# Dashboard → Monitoring
```

### 4. 优化性能
```bash
# 调整 PostgreSQL 配置
# 编辑 .env 文件
nano /opt/sub2api/.env

# 根据服务器内存调整：
# POSTGRES_SHARED_BUFFERS=512MB  # 2核4G 建议
# POSTGRES_EFFECTIVE_CACHE_SIZE=2GB
```

---

## ✨ 完成！

迁移完成后，你的 Sub2API 已经在 Vultr 东京服务器上运行，享受：

- ✅ 更低的价格（节省 60%+）
- ✅ 更好的性能（无 CPU 积分限制）
- ✅ 10个月免费使用（$250 赠金）
- ✅ 简单透明的计费

AWS 服务器保留 1-2 周确认无误后即可关闭。

🎉 **祝使用愉快！**
