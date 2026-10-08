# Sub2API 从 AWS 迁移到 Vultr 完整方案

## 📋 迁移概览

从 AWS EC2 迁移到 Vultr Tokyo 的完整流程，包括自动化部署和数据迁移。

---

## 🎯 迁移流程

```
第一步：AWS 备份数据
    ↓
第二步：Vultr 部署新服务器（自动化）
    ↓
第三步：导入数据
    ↓
第四步：测试验证
    ↓
第五步：切换 DNS
    ↓
第六步：关闭 AWS 服务器
```

---

## 📦 第一步：从 AWS 导出数据

### 1.1 SSH 登录到 AWS EC2

```bash
# 使用你的 AWS EC2 密钥登录
ssh -i your-aws-key.pem ubuntu@your-aws-ip

# 或者如果用密码登录
ssh ec2-user@your-aws-ip
```

### 1.2 使用自动化备份脚本

上传并运行 `aws-backup.sh` 脚本（见下方脚本文件）

```bash
# 下载备份脚本
curl -o aws-backup.sh https://raw.githubusercontent.com/YOUR_REPO/aws-backup.sh
chmod +x aws-backup.sh

# 执行备份
./aws-backup.sh
```

**脚本会自动完成：**
- ✅ 导出 PostgreSQL 数据库
- ✅ 导出 Redis 数据（如果有）
- ✅ 打包配置文件（.env, config.yaml）
- ✅ 打包 data 目录
- ✅ 创建压缩包 `sub2api-backup-YYYYMMDD-HHMMSS.tar.gz`

### 1.3 下载备份文件到本地

```bash
# 在你的本地电脑执行
scp -i your-aws-key.pem ubuntu@your-aws-ip:~/sub2api-backup-*.tar.gz ./

# 验证文件完整性
tar -tzf sub2api-backup-*.tar.gz | head -20
```

---

## 🚀 第二步：Vultr 自动化部署

### 2.1 部署 Vultr 服务器

1. **在 Vultr 控制面板点击 "Deploy Now"**
2. **等待 1-2 分钟服务器创建完成**
3. **检查邮件获取登录信息：**
   - IP Address: xxx.xxx.xxx.xxx
   - Username: root
   - Password: <随机密码>

### 2.2 首次登录

```bash
# SSH 登录到新服务器
ssh root@your-vultr-ip

# 输入邮件中的密码
# 首次登录会提示修改密码
```

### 2.3 运行一键部署脚本

```bash
# 下载并运行自动化部署脚本
curl -sSL https://raw.githubusercontent.com/YOUR_REPO/vultr-deploy.sh | bash

# 或者手动下载后运行
curl -o vultr-deploy.sh https://raw.githubusercontent.com/YOUR_REPO/vultr-deploy.sh
chmod +x vultr-deploy.sh
./vultr-deploy.sh
```

**脚本会自动完成：**
- ✅ 更新系统
- ✅ 安装 Docker 和 Docker Compose
- ✅ 配置防火墙
- ✅ 创建部署目录
- ✅ 下载 Sub2API 配置文件
- ✅ 生成必要的密钥（JWT_SECRET, TOTP_ENCRYPTION_KEY）
- ✅ 启动 Sub2API 服务

**预计时间：5-10 分钟**

---

## 📥 第三步：导入数据

### 3.1 上传备份文件到 Vultr

```bash
# 在本地电脑执行
scp sub2api-backup-*.tar.gz root@your-vultr-ip:/root/

# 登录到 Vultr 服务器
ssh root@your-vultr-ip
```

### 3.2 运行数据导入脚本

```bash
# 下载导入脚本
curl -o restore-data.sh https://raw.githubusercontent.com/YOUR_REPO/restore-data.sh
chmod +x restore-data.sh

# 运行导入（会自动找到备份文件）
./restore-data.sh sub2api-backup-*.tar.gz
```

**脚本会自动完成：**
- ✅ 解压备份文件
- ✅ 停止 Sub2API 服务
- ✅ 导入 PostgreSQL 数据库
- ✅ 导入 Redis 数据（如果有）
- ✅ 恢复配置文件（保留新生成的密钥）
- ✅ 恢复 data 目录
- ✅ 重启服务

**预计时间：2-5 分钟**

---

## ✅ 第四步：验证测试

### 4.1 检查服务状态

```bash
# 查看容器运行状态
cd /opt/sub2api
docker compose ps

# 应该看到：
# sub2api          running
# sub2api-postgres running
# sub2api-redis    running

# 查看日志
docker compose logs -f sub2api
```

### 4.2 测试 API 访问

```bash
# 健康检查
curl http://your-vultr-ip:8080/health

# 应该返回：
# {"status":"ok"}

# 测试前端访问
curl -I http://your-vultr-ip:8080

# 应该返回 200 OK
```

### 4.3 测试登录

1. 浏览器访问：`http://your-vultr-ip:8080`
2. 使用 AWS 上的管理员账号登录
3. 检查：
   - ✅ 用户数据完整
   - ✅ API Keys 可用
   - ✅ 配额数据正确
   - ✅ 统计数据显示

---

## 🌐 第五步：切换 DNS

### 5.1 灰度切换（推荐）

```bash
# 方式 1：本地测试
# 在本地电脑修改 hosts 文件测试

# Mac/Linux
sudo nano /etc/hosts
# 添加：your-vultr-ip your-domain.com

# Windows
# 编辑 C:\Windows\System32\drivers\etc\hosts
# 添加：your-vultr-ip your-domain.com

# 测试访问
curl https://your-domain.com/health
```

### 5.2 修改 DNS

**在你的域名服务商（如阿里云、Cloudflare）：**

```
1. 找到 A 记录
2. 修改 IP 地址：
   旧：AWS IP (xxx.xxx.xxx.xxx)
   新：Vultr IP (xxx.xxx.xxx.xxx)
3. TTL 设置为 300（5分钟）
4. 保存
```

### 5.3 等待 DNS 传播

```bash
# 检查 DNS 是否更新
nslookup your-domain.com
dig your-domain.com

# 等待时间：5-60 分钟
# 期间新旧服务器可能同时接收流量
```

---

## 🛡️ 第六步：收尾工作

### 6.1 在 Vultr 服务器上配置 SSL（如果使用域名）

```bash
# 安装 Certbot
apt install -y certbot

# 获取 SSL 证书
certbot certonly --standalone -d your-domain.com

# 配置自动续期
certbot renew --dry-run
```

### 6.2 启用自动备份

```bash
# Vultr 控制面板
# Settings → Backups → Enable Automatic Backups

# 或使用定时任务备份
crontab -e

# 添加每日备份（凌晨 2 点）
0 2 * * * /opt/sub2api/backup.sh
```

### 6.3 监控设置

```bash
# 安装监控工具（可选）
apt install -y htop iotop nethogs

# 查看资源使用
htop

# 设置告警（可选）
# 使用 Vultr 内置监控或第三方服务
```

### 6.4 保留 AWS 服务器 1-2 周

```
⚠️ 重要提示：

不要立即删除 AWS 服务器：
1. 保留 1-2 周观察
2. 确认 Vultr 稳定运行
3. 确认没有数据丢失
4. 确认用户没有问题反馈

之后再关闭 AWS 实例
```

---

## 📊 费用对比

```
迁移前（AWS）:
- EC2 t3.medium: ~$35/月
- EBS 存储: ~$10/月
- 流量: ~$10/月
- 其他: ~$15/月
━━━━━━━━━━━━━━━━━━━
总计: ~$70/月

迁移后（Vultr）:
- vhp-2c-4gb: $24/月
- 使用赠金: $0（前10个月）
━━━━━━━━━━━━━━━━━━━
总计: $0/月（赠金期）
       $24/月（之后）

年度节省: $552+
```

---

## ⚠️ 常见问题

### Q1: 迁移会停机多久？
```
A: 使用自动化脚本，停机时间约 5-10 分钟
   - 导出数据: 2-3 分钟
   - 导入数据: 2-5 分钟
   - DNS 切换: 0 停机（新旧并行）
```

### Q2: 如果迁移失败怎么办？
```
A: AWS 服务器保持运行：
   1. AWS 继续提供服务
   2. 在 Vultr 重新部署
   3. 无风险回滚
```

### Q3: 数据会丢失吗？
```
A: 不会，多重保护：
   1. AWS 原始数据保留
   2. 本地备份副本
   3. Vultr 可快照备份
```

### Q4: 需要多长时间？
```
A: 总时间约 30-60 分钟：
   - AWS 备份: 5 分钟
   - Vultr 部署: 10 分钟
   - 数据导入: 5 分钟
   - 测试验证: 10 分钟
   - DNS 切换: 即时（传播需等待）
```

### Q5: 用户会受影响吗？
```
A: 几乎无感知：
   1. DNS 切换前旧服务器运行
   2. 切换时新旧可能并存
   3. 传播后全部指向新服务器
   4. 用户体验无缝切换
```

---

## 📞 需要帮助？

遇到问题请检查：
1. 日志文件：`docker compose logs`
2. 服务状态：`docker compose ps`
3. 防火墙：`ufw status`
4. 端口监听：`netstat -tlnp | grep 8080`

---

## 🎉 迁移完成检查清单

```
□ AWS 数据已备份
□ Vultr 服务器已部署
□ Docker 和 Docker Compose 已安装
□ Sub2API 服务运行正常
□ 数据库已导入
□ 配置文件已恢复
□ API 测试通过
□ 前端可访问
□ 用户可登录
□ DNS 已切换
□ SSL 已配置（如需要）
□ 备份已设置
□ 监控已配置
□ AWS 服务器保留观察
```

---

**下一个文件将提供具体的自动化脚本...**
