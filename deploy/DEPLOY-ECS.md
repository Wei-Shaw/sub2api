# Sub2API ECS 部署指南

## 概述

本指南描述如何在阿里云 ECS（Ubuntu）上手动部署 Sub2API，不依赖 Docker。

- **本地**：macOS（编译前端 + 交叉编译后端）
- **ECS**：Ubuntu（只运行，不编译）

---

## 前置要求

### 本地（开发机）

- Go 1.26+
- Node.js 20+ + pnpm

### ECS

- Ubuntu 22.04+
- 内存：2GB+（编译在本地进行，ECS 只需运行）
- 安全组：开放 80 端口（Nginx）、8080 端口（直接访问后端）

---

## 1. 本地编译

### 1.1 编译后端（交叉编译 Linux 版本）

```bash
cd sub2api/backend

# 设置 GOSUMDB（国内环境）
export GOSUMDB=sum.golang.google.cn

# 编译 Linux 二进制
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o sub2api-linux ./cmd/server
```

### 1.2 编译前端

```bash
cd sub2api/frontend
pnpm install
pnpm run build
```

产物位置：`sub2api/backend/internal/web/dist/`

---

## 2. 上传到 ECS

```bash
# 后端二进制
scp backend/sub2api-linux root@ECS_IP:/root/workspace/sub2api/sub2api

# 前端产物（Vite 输出到 backend/internal/web/dist/）
scp -r backend/internal/web/dist root@ECS_IP:/root/workspace/sub2api/frontend/

# 运行脚本
scp deploy/run-ecs.sh root@ECS_IP:/root/workspace/sub2api/deploy/
```

### 添加执行权限

```bash
ssh root@ECS_IP
chmod +x /root/workspace/sub2api/sub2api
```

---

## 3. ECS 上启动

### 3.1 启动后端（自动初始化 PG + Redis）

```bash
sudo bash /root/workspace/sub2api/deploy/run-ecs.sh
```

首次运行会自动：
- 安装 PostgreSQL、Redis（如未安装）
- 初始化 PostgreSQL 数据目录
- 创建数据库和用户
- 生成 `.env` 配置文件
- 启动 Redis
- 启动 Sub2API

启动后显示管理员密码，访问 `http://ECS_IP:8080`。

### 3.2 停止

按 `Ctrl+C`，脚本会自动关闭 PG + Redis + App。

### 3.3 重新启动

```bash
sudo bash /root/workspace/sub2api/deploy/run-ecs.sh
```

---

## 4. Nginx 反向代理（可选，推荐）

使用 Nginx 监听 80 端口，代理到后端 8080。

### 4.1 上传 Nginx 配置脚本

```bash
scp deploy/setup-nginx.sh root@ECS_IP:/root/workspace/sub2api/deploy/
```

### 4.2 ECS 上执行

```bash
sudo bash /root/workspace/sub2api/deploy/setup-nginx.sh
```

这会：
- 安装 Nginx
- 写入站点配置
- 启动 Nginx

### 4.3 同时启动后端

```bash
sudo bash /root/workspace/sub2api/deploy/run-ecs.sh
```

访问 `http://ECS_IP/`（80 端口）。

---

## 5. 配置说明

### 5.1 环境变量文件

位置：`/root/workspace/sub2api/.env`

首次运行时自动生成。如需修改：

```bash
sudo nano /root/workspace/sub2api/.env
```

关键配置：

| 变量 | 说明 |
|------|------|
| `SERVER_PORT` | 后端端口，默认 8080 |
| `ADMIN_PASSWORD` | 管理员密码 |
| `JWT_SECRET` | JWT 密钥（固定后重启不会登出） |
| `TOTP_ENCRYPTION_KEY` | 2FA 密钥 |

### 5.2 数据目录

| 路径 | 用途 |
|------|------|
| `/var/lib/sub2api/postgres` | PostgreSQL 数据 |
| `/var/lib/sub2api/redis` | Redis 数据 |
| `/var/lib/sub2api/runtime` | 运行时文件（socket、pid、日志） |

### 5.3 日志

| 路径 | 用途 |
|------|------|
| `/var/log/nginx/access.log` | Nginx 访问日志 |
| `/var/log/nginx/error.log` | Nginx 错误日志 |
| `/var/lib/sub2api/runtime/postgres.log` | PostgreSQL 日志 |

---

## 6. 重置数据

如果数据库密码损坏或需要清空数据：

```bash
# 停止所有
sudo pkill -f sub2api 2>/dev/null || true
sudo redis-cli shutdown 2>/dev/null || true
PG_VER=$(ls /usr/lib/postgresql/ | sort -V | tail -1)
sudo gosu postgres /usr/lib/postgresql/${PG_VER}/bin/pg_ctl -D /var/lib/sub2api/postgres stop 2>/dev/null || true

# 删除数据和配置
sudo rm -rf /var/lib/sub2api/postgres /var/lib/sub2api/redis /var/lib/sub2api/runtime
sudo rm -rf /var/lib/sub2api/redis /var/lib/sub2api/runtime/redis.conf /var/lib/sub2api/runtime/redis.pid
sudo rm -f /root/workspace/sub2api/.env

# 重新初始化
sudo bash /root/workspace/sub2api/deploy/run-ecs.sh
```

---

## 7. 常见问题

### 7.1 `sub2api` 没有执行权限

```bash
chmod +x /root/workspace/sub2api/sub2api
```

### 7.1 `nginx` 没有访问权限

```bash
chmod 755 /root
chmod 755 /root/workspace
chmod 755 /root/workspace/sub2api
chmod -R 755 /root/workspace/sub2api/frontend/dist
```

### 7.3 密码认证失败

`.env` 里的密码和数据库实际密码不匹配。按第 6 节重置数据。

### 7.4 `GOSUMDB=off` 导致编译失败

```bash
export GOSUMDB=sum.golang.google.cn
```

### 7.5 本地 Go 版本低于项目要求

```bash
# 强制使用本地 Go，不自动下载工具链
GOTOOLCHAIN=local GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o sub2api-linux ./cmd/server
```

### 7.6 前端产物位置

Vite 配置将产物输出到 `backend/internal/web/dist/`，不是 `frontend/dist/`。

```bash
# 正确的前端产物路径
scp -r backend/internal/web/dist root@ECS_IP:/root/workspace/sub2api/frontend/
```
