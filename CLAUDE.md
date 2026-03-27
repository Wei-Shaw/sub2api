# PR 默认语义（最高优先级）

- 未特别说明时，文档或对话中的“PR”一律指**提交到上游仓库** `Wei-Shaw/sub2api:main`
- 如果只是合并回我们自己的仓库，必须明确表述为：
  - “内部同步 PR”
  - “合并回我们的 main”
  - “fork 内部 PR”
- 禁止将“上游 PR”和“我们自己仓库内的同步 PR”混用为同一个概念

## 本地依赖联调

- 本地 `go-sora2api` 仓库固定路径：`C:\Users\16790\GolandProjects\go-sora2api`
- 需要联调 `go-sora2api` 时，优先使用 `backend/go.mod` 的 `replace` 指向该本地路径，而不是使用 `git submodule`
- 联调完成后，如需提交或部署，再切换为 fork 仓库的明确 tag 或 commit

---
# Sub2API 开发说明

## 版本管理策略

### 版本号规则

我们在官方版本号后面添加自己的小版本号：

- 官方版本：`v0.1.68`
- 我们的版本：`v0.1.68.1`、`v0.1.68.2`（递增）

### 分支策略

| 分支 | 说明 |
|------|------|
| `main` | 我们的主分支，包含所有定制功能 |
| `release/custom-X.Y.Z` | 基于官方 `vX.Y.Z` 的发布分支 |
| `upstream/main` | 上游官方仓库 |

---

## 发布流程（基于新官方版本）

当官方发布新版本（如 `v0.1.69`）时：

### 1. 同步上游并创建发布分支

```bash
# 获取上游最新代码
git fetch upstream --tags

# 基于官方标签创建新的发布分支
git checkout v0.1.69 -b release/custom-0.1.69

# 合并我们的 main 分支（包含所有定制功能）
git merge main --no-edit

# 解决可能的冲突后继续
```

### 2. 更新版本号并打标签

```bash
# 更新版本号文件
echo "0.1.69.1" > backend/cmd/server/VERSION
git add backend/cmd/server/VERSION
git commit -m "chore: bump version to 0.1.69.1"

# 打上我们自己的标签
git tag v0.1.69.1

# 推送分支和标签
git push origin release/custom-0.1.69
git push origin v0.1.69.1
```

### 3. 更新 main 分支

```bash
# 将发布分支合并回 main，保持 main 包含最新定制功能
git checkout main
git merge release/custom-0.1.69
git push origin main
```

---

## 热修复发布（在现有版本上修复）

当需要在当前版本上发布修复时：

```bash
# 在当前发布分支上修复
git checkout release/custom-0.1.68
# ... 进行修复 ...
git commit -m "fix: 修复描述"

# 递增小版本号
echo "0.1.68.2" > backend/cmd/server/VERSION
git add backend/cmd/server/VERSION
git commit -m "chore: bump version to 0.1.68.2"

# 打标签并推送
git tag v0.1.68.2
git push origin release/custom-0.1.68
git push origin v0.1.68.2

# 同步修复到 main
git checkout main
git cherry-pick <fix-commit-hash>
git push origin main
```

---

## 服务器部署流程

### 前置条件

- 本地已配置 SSH 别名 `clicodeplus` 连接到生产服务器（运行服务 + 构建镜像）
- 生产服务器部署目录：`/root/sub2api`（正式）、`/root/sub2api-beta`（测试）、`/root/sub2api-star`（Star）
- 生产服务器使用 Docker Compose 部署
- **镜像在生产服务器本机构建**，使用资源限制的 `limited-builder` 构建器（3 核 CPU、4G 内存），避免构建占满服务器资源影响线上服务

### 服务器角色说明

| 服务器 | SSH 别名 | 职责 |
|--------|----------|------|
| 生产服务器 | `clicodeplus` | 拉取代码、构建镜像、运行服务、部署验证 |
| 数据库服务器 | `db-clicodeplus` | PostgreSQL 16 + Redis 7，所有环境共用 |

> 数据库服务器运维手册：`db-clicodeplus:/root/README.md`

### 构建器说明

生产服务器上配置了资源限制的 Docker buildx 构建器 `limited-builder`，**所有构建操作必须使用此构建器**：

- **构建器名称**：`limited-builder`
- **驱动**：`docker-container`（独立容器运行 BuildKit）
- **资源限制**：3 核 CPU、4G 内存（服务器共 6 核 8G，预留一半给线上服务）
- **容器名**：`buildx_buildkit_limited-builder0`

```bash
# 构建命令格式（必须指定 --builder）
ssh clicodeplus "cd /root/sub2api && docker buildx build --builder limited-builder --no-cache --load -t sub2api:latest -f Dockerfile ."

# 查看构建器状态
ssh clicodeplus "docker buildx inspect limited-builder"

# 如果构建器容器被意外删除，重新创建：
ssh clicodeplus "docker buildx create --name limited-builder --driver docker-container --driver-opt 'default-load=true' && docker buildx inspect --builder limited-builder --bootstrap && docker update --cpus=3 --memory=4g --memory-swap=4g buildx_buildkit_limited-builder0"
```

### 部署环境说明

| 环境 | 目录（生产服务器） | 端口 | 数据库 | Redis DB | 容器名 |
|------|------|------|--------|----------|--------|
| 正式 | `/root/sub2api` | 8080 | `sub2api` | 0 | `sub2api` |
| Beta | `/root/sub2api-beta` | 8084 | `beta` | 2 | `sub2api-beta` |
| OpenAI | `/root/sub2api-openai` | 8083 | `openai` | 3 | `sub2api-openai` |
| Star | `/root/sub2api-star` | 8086 | `star` | 4 | `sub2api-star` |

### 外部数据库与 Redis

所有环境（正式、Beta、OpenAI、Star）共用 `db.clicodeplus.com` 上的 **PostgreSQL 16** 和 **Redis 7**，不使用容器内数据库或 Redis。

**PostgreSQL**（端口 5432，TLS 加密，scram-sha-256 认证）：

| 环境 | 用户名 | 数据库 |
|------|--------|--------|
| 正式 | `sub2api` | `sub2api` |
| Beta | `beta` | `beta` |
| OpenAI | `openai` | `openai` |
| Star | `star` | `star` |

**Redis**（端口 6379，密码认证）：

| 环境 | DB |
|------|-----|
| 正式 | 0 |
| Beta | 2 |
| OpenAI | 3 |
| Star | 4 |

**配置方式**：
- 数据库通过 `.env` 中的 `DATABASE_HOST`、`DATABASE_SSLMODE`、`POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB` 配置
- Redis 通过 `docker-compose.override.yml` 覆盖 `REDIS_HOST`（因主 compose 文件硬编码为 `redis`），密码通过 `.env` 中的 `REDIS_PASSWORD` 配置
- 各环境的 `docker-compose.override.yml` 已通过 `depends_on: !reset {}` 和 `redis: profiles: [disabled]` 去掉了对容器 Redis 的依赖

#### 数据库操作命令

通过 SSH 在服务器上执行数据库操作：

```bash
# 正式环境 - 查询迁移记录
ssh clicodeplus "source /root/sub2api/deploy/.env && PGPASSWORD=\"\$POSTGRES_PASSWORD\" psql -h \$DATABASE_HOST -U \$POSTGRES_USER -d \$POSTGRES_DB -c 'SELECT * FROM schema_migrations ORDER BY applied_at DESC LIMIT 5;'"

# Beta 环境 - 查询迁移记录
ssh clicodeplus "source /root/sub2api-beta/deploy/.env && PGPASSWORD=\"\$POSTGRES_PASSWORD\" psql -h \$DATABASE_HOST -U \$POSTGRES_USER -d \$POSTGRES_DB -c 'SELECT * FROM schema_migrations ORDER BY applied_at DESC LIMIT 5;'"

# Beta 环境 - 清除指定迁移记录（重新执行迁移）
ssh clicodeplus "source /root/sub2api-beta/deploy/.env && PGPASSWORD=\"\$POSTGRES_PASSWORD\" psql -h \$DATABASE_HOST -U \$POSTGRES_USER -d \$POSTGRES_DB -c \"DELETE FROM schema_migrations WHERE filename LIKE '%049%';\""

# Beta 环境 - 更新账号数据
ssh clicodeplus "source /root/sub2api-beta/deploy/.env && PGPASSWORD=\"\$POSTGRES_PASSWORD\" psql -h \$DATABASE_HOST -U \$POSTGRES_USER -d \$POSTGRES_DB -c \"UPDATE accounts SET credentials = credentials - 'model_mapping' WHERE platform = 'antigravity';\""
```

> **注意**：使用 `source .env` 加载环境变量，避免在命令行中暴露密码。

### 部署步骤

**重要：每次部署都必须递增版本号！**

#### 0. 递增版本号并推送（本地操作）

每次部署前，先在本地递增小版本号并确保推送成功：

```bash
# 查看当前版本号
cat backend/cmd/server/VERSION
# 假设当前是 0.1.69.1

# 递增版本号
echo "0.1.69.2" > backend/cmd/server/VERSION
git add backend/cmd/server/VERSION
git commit -m "chore: bump version to 0.1.69.2"
git push origin release/custom-0.1.69

# ⚠️ 确认推送成功（必须看到分支更新输出，不能有 rejected 错误）
```

> **检查点**：如果有其他未提交的改动，应先 commit 并 push，确保 release 分支上的所有代码都已推送到远程。

#### 1. 生产服务器拉取代码

```bash
# 拉取最新代码并切换分支
ssh clicodeplus "cd /root/sub2api && git fetch fork && git checkout -B release/custom-0.1.69 fork/release/custom-0.1.69"

# ⚠️ 验证版本号与步骤 0 一致
ssh clicodeplus "cat /root/sub2api/backend/cmd/server/VERSION"
```

#### 2. 生产服务器构建镜像（使用 limited-builder）

```bash
ssh clicodeplus "cd /root/sub2api && docker buildx build --builder limited-builder --no-cache --load -t sub2api:latest -f Dockerfile ."

# ⚠️ 必须看到构建成功输出，如果失败需要先排查问题
```

> **常见构建问题**：
> - 构建器未启动 → `docker buildx inspect --builder limited-builder --bootstrap`
> - 磁盘空间不足 → `docker system prune -f` 清理无用镜像
> - 构建器被删除 → 参见上方「构建器说明」重新创建

#### 3. 更新镜像标签并重启

```bash
# 更新镜像标签并重启
ssh clicodeplus "docker tag sub2api:latest weishaw/sub2api:latest"
ssh clicodeplus "cd /root/sub2api/deploy && docker compose up -d --force-recreate sub2api"
```

#### 4. 验证部署

```bash
# 查看启动日志
ssh clicodeplus "docker logs sub2api --tail 20"

# 确认版本号（必须与步骤 0 中设置的版本号一致）
ssh clicodeplus "cat /root/sub2api/backend/cmd/server/VERSION"

# 检查容器状态（必须显示 healthy）
ssh clicodeplus "docker ps | grep sub2api"
```

---

## Beta 并行部署（不影响现网）

目标：在同一台服务器上并行启动一个 beta 实例（例如端口 `8084`），**严禁改动/重启**现网实例（默认目录 `/root/sub2api`）。

### 设计原则

- **新目录**：beta 使用独立目录，例如 `/root/sub2api-beta`。
- **敏感信息只放 `.env`**：beta 的数据库密码、JWT_SECRET 等只写入 `/root/sub2api-beta/deploy/.env`，不要提交到 git。
- **独立 Compose Project**：通过 `docker compose -p sub2api-beta ...` 启动，确保 network/volume 隔离。
- **独立端口**：通过 `.env` 的 `SERVER_PORT` 映射宿主机端口（例如 `8084:8080`）。

### 前置检查

```bash
# 1) 确保 8084 未被占用
ssh clicodeplus "ss -ltnp | grep :8084 || echo '8084 is free'"

# 2) 确认现网容器还在（只读检查）
ssh clicodeplus "docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}' | sed -n '1,200p'"
```

### 首次部署步骤

> **构建说明**：正式和 beta 通过不同的镜像标签区分（`sub2api:latest` 用于正式，`sub2api:beta` 用于测试），均在生产服务器本机使用 `limited-builder` 构建。

```bash
# 1) 在生产服务器上拉取代码并构建 beta 镜像
ssh clicodeplus "cd /root/sub2api-beta && git fetch --all --tags && git checkout -f release/custom-0.1.71 && git reset --hard origin/release/custom-0.1.71"
ssh clicodeplus "cd /root/sub2api-beta && docker buildx build --builder limited-builder --no-cache --load -t sub2api:beta -f Dockerfile ."

# 2) 在生产服务器上准备 beta 环境
ssh clicodeplus

# 克隆代码（仅用于 deploy 配置和版本号确认，不在此构建）
cd /root
git clone https://github.com/touwaeriol/sub2api.git sub2api-beta
cd /root/sub2api-beta
git checkout release/custom-0.1.71

# 4) 准备 beta 的 .env（敏感信息只写这里）
cd /root/sub2api-beta/deploy

# 推荐：从现网 .env 复制，保证除 DB 名/用户/端口外完全一致
cp -f /root/sub2api/deploy/.env ./.env

# 仅修改以下三项（其他保持不变）
perl -pi -e 's/^SERVER_PORT=.*/SERVER_PORT=8084/' ./.env
perl -pi -e 's/^POSTGRES_USER=.*/POSTGRES_USER=beta/' ./.env
perl -pi -e 's/^POSTGRES_DB=.*/POSTGRES_DB=beta/' ./.env

# 5) 写 compose override（避免与现网容器名冲突，镜像使用本机构建的 sub2api:beta，Redis 使用外部服务）
cat > docker-compose.override.yml <<'YAML'
services:
  sub2api:
    image: sub2api:beta
    container_name: sub2api-beta
    environment:
      - DATABASE_HOST=${DATABASE_HOST:-postgres}
      - DATABASE_SSLMODE=${DATABASE_SSLMODE:-disable}
      - REDIS_HOST=db.clicodeplus.com
    depends_on: !reset {}
  redis:
    profiles:
      - disabled
YAML

# 6) 启动 beta（独立 project，确保不影响现网）
cd /root/sub2api-beta/deploy
docker compose -p sub2api-beta --env-file .env -f docker-compose.yml -f docker-compose.override.yml up -d

# 7) 验证 beta
curl -fsS http://127.0.0.1:8084/health
docker logs sub2api-beta --tail 50
```

### 数据库配置约定（beta）

- 数据库地址/SSL/密码：与现网一致（从现网 `.env` 复制即可），均指向 `db.clicodeplus.com`。
- 仅修改：
  - `POSTGRES_USER=beta`
  - `POSTGRES_DB=beta`
  - `REDIS_DB=2`

注意：需要数据库侧已存在 `beta` 用户与 `beta` 数据库，并授予权限；否则容器会启动失败并不断重启。

### 更新 beta（本机构建 + 仅重启 beta 容器）

```bash
# 1) 生产服务器拉取代码并构建镜像
ssh clicodeplus "cd /root/sub2api-beta && git fetch --all --tags && git checkout -f release/custom-0.1.71 && git reset --hard origin/release/custom-0.1.71"
ssh clicodeplus "cd /root/sub2api-beta && docker buildx build --builder limited-builder --no-cache --load -t sub2api:beta -f Dockerfile ."
# ⚠️ 必须看到构建成功输出

# 2) 重启 beta 容器并验证
ssh clicodeplus "cd /root/sub2api-beta/deploy && docker compose -p sub2api-beta --env-file .env -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --force-recreate sub2api"
ssh clicodeplus "sleep 5 && curl -fsS http://127.0.0.1:8084/health"
ssh clicodeplus "cat /root/sub2api-beta/backend/cmd/server/VERSION"
```

### 停止/回滚 beta（只影响 beta）

```bash
ssh clicodeplus "cd /root/sub2api-beta/deploy && docker compose -p sub2api-beta -f docker-compose.yml -f docker-compose.override.yml down"
```

---

## 服务器首次部署

### 1. 生产服务器：克隆代码并配置环境

```bash
ssh clicodeplus
cd /root
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api

# 添加 fork 仓库
git remote add fork https://github.com/touwaeriol/sub2api.git
git fetch fork
git checkout -B release/custom-0.1.69 fork/release/custom-0.1.69

# 配置环境变量
cd deploy
cp .env.example .env
vim .env  # 配置 DATABASE_HOST=db.clicodeplus.com, POSTGRES_PASSWORD, REDIS_PASSWORD, JWT_SECRET 等

# 创建 override 文件（Redis 指向外部服务，去掉容器 Redis 依赖）
cat > docker-compose.override.yml <<'YAML'
services:
  sub2api:
    environment:
      - REDIS_HOST=db.clicodeplus.com
    depends_on: !reset {}
  redis:
    profiles:
      - disabled
YAML
```

### 2. 生产服务器：创建构建器并构建镜像

```bash
# 创建资源限制的构建器（首次执行一次即可）
docker buildx create --name limited-builder --driver docker-container --driver-opt "default-load=true"
docker buildx inspect --builder limited-builder --bootstrap
docker update --cpus=3 --memory=4g --memory-swap=4g buildx_buildkit_limited-builder0

# 构建镜像
cd /root/sub2api
docker buildx build --builder limited-builder --no-cache --load -t sub2api:latest -f Dockerfile .

# 更新镜像标签并启动
docker tag sub2api:latest weishaw/sub2api:latest
cd /root/sub2api/deploy && docker compose up -d
```

### 3. 验证部署

```bash
# 查看应用日志
docker logs sub2api --tail 50

# 检查健康状态
curl http://localhost:8080/health

# 确认版本号
cat /root/sub2api/backend/cmd/server/VERSION
```

### 4. 常用运维命令

```bash
# 查看实时日志
docker logs -f sub2api

# 重启服务
docker compose restart sub2api

# 停止所有服务
docker compose down

# 停止并删除数据卷（慎用！会删除数据库数据）
docker compose down -v

# 查看资源使用情况
docker stats sub2api
```

---

## Admin API 接口文档

### ⚠️ API 操作流程规范

当收到操作正式环境 Web 界面的新需求，但文档中未记录对应 API 接口时，**必须按以下流程执行**：

1. **探索接口**：通过代码库搜索路由定义（`backend/internal/server/routes/`）、Handler（`backend/internal/handler/admin/`）和请求结构体，确定正确的 API 端点、请求方法、请求体格式
2. **更新文档**：将新发现的接口补充到本文档的 Admin API 接口文档章节中，包含端点、参数说明和 curl 示例
3. **执行操作**：根据最新文档中记录的接口完成用户需求

> **目的**：避免每次遇到相同需求都重复探索代码库，确保 API 文档持续完善，后续操作可直接查阅文档执行。

---

### 认证方式

所有 Admin API 通过 `x-api-key` 请求头传递 Admin API Key 认证。

```
x-api-key: admin-xxx
```

> **使用说明**：Admin API Key 统一存放在项目根目录 `.env` 文件的 `ADMIN_API_KEY` 变量中（该文件已被 `.gitignore` 排除，不会提交到代码库）。操作前先从 `.env` 读取密钥；若密钥失效（返回 401），应提示用户提供新的密钥并更新到 `.env` 中。Token 格式为 `admin-` + 64 位十六进制字符，在管理后台 `设置 > Admin API Key` 中生成。**请勿将实际 token 写入文档或代码中。**

### 环境地址

| 环境 | 基础地址 | 说明 |
|------|----------|------|
| 正式 | `https://clicodeplus.com` | 生产环境 |
| Beta | `http://<服务器IP>:8084` | 仅内网访问 |
| OpenAI | `http://<服务器IP>:8083` | 仅内网访问 |
| Star | `https://hyntoken.com` | 独立环境 |

> 以下接口文档中，`${BASE}` 代表环境基础地址，`${KEY}` 代表 `.env` 中的 `ADMIN_API_KEY`。操作前执行 `source .env` 或 `export KEY=$ADMIN_API_KEY` 加载。

---

### 1. 账号管理

#### 1.1 获取账号列表

```
GET /api/v1/admin/accounts
```

**查询参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `platform` | string | 否 | 平台筛选：`antigravity` / `anthropic` / `openai` / `gemini` |
| `type` | string | 否 | 账号类型：`oauth` / `api_key` / `cookie` |
| `status` | string | 否 | 状态：`active` / `disabled` / `error` |
| `search` | string | 否 | 搜索关键词（名称、备注） |
| `page` | int | 否 | 页码，默认 1 |
| `page_size` | int | 否 | 每页数量，默认 20 |

```bash
curl -s "${BASE}/api/v1/admin/accounts?platform=antigravity&page=1&page_size=100" \
  -H "x-api-key: ${KEY}"
```

**响应**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [{"id": 1, "name": "xxx@gmail.com", "platform": "antigravity", "status": "active", ...}],
    "total": 66
  }
}
```

#### 1.2 获取账号详情

```
GET /api/v1/admin/accounts/:id
```

```bash
curl -s "${BASE}/api/v1/admin/accounts/1" -H "x-api-key: ${KEY}"
```

#### 1.3 测试账号连接

```
POST /api/v1/admin/accounts/:id/test
```

**请求体**（JSON，可选）：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model_id` | string | 否 | 指定测试模型，如 `claude-opus-4-6`；不传则使用默认模型 |

**响应格式**：SSE（Server-Sent Events）流

```bash
curl -N -X POST "${BASE}/api/v1/admin/accounts/1/test" \
  -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model_id": "claude-opus-4-6"}'
```

**SSE 事件类型**：

| type | 字段 | 说明 |
|------|------|------|
| `test_start` | `model` | 测试开始，返回测试模型名 |
| `content` | `text` | 模型响应内容（流式文本片段） |
| `test_end` | `success`, `error` | 测试结束，`success=true` 表示成功 |
| `error` | `text` | 错误信息 |

#### 1.4 清除账号限流

```
POST /api/v1/admin/accounts/:id/clear-rate-limit
```

```bash
curl -X POST "${BASE}/api/v1/admin/accounts/1/clear-rate-limit" \
  -H "x-api-key: ${KEY}"
```

#### 1.5 清除账号错误状态

```
POST /api/v1/admin/accounts/:id/clear-error
```

```bash
curl -X POST "${BASE}/api/v1/admin/accounts/1/clear-error" \
  -H "x-api-key: ${KEY}"
```

#### 1.6 获取账号可用模型

```
GET /api/v1/admin/accounts/:id/models
```

```bash
curl -s "${BASE}/api/v1/admin/accounts/1/models" -H "x-api-key: ${KEY}"
```

#### 1.7 刷新 OAuth Token

```
POST /api/v1/admin/accounts/:id/refresh
```

```bash
curl -X POST "${BASE}/api/v1/admin/accounts/1/refresh" -H "x-api-key: ${KEY}"
```

#### 1.8 刷新账号等级

```
POST /api/v1/admin/accounts/:id/refresh-tier
```

```bash
curl -X POST "${BASE}/api/v1/admin/accounts/1/refresh-tier" -H "x-api-key: ${KEY}"
```

#### 1.9 获取账号统计

```
GET /api/v1/admin/accounts/:id/stats
```

```bash
curl -s "${BASE}/api/v1/admin/accounts/1/stats" -H "x-api-key: ${KEY}"
```

#### 1.10 获取账号用量

```
GET /api/v1/admin/accounts/:id/usage
```

```bash
curl -s "${BASE}/api/v1/admin/accounts/1/usage" -H "x-api-key: ${KEY}"
```

#### 1.11 更新单个账号

```
PUT /api/v1/admin/accounts/:id
```

**请求体**（JSON，所有字段均为可选，仅传需要更新的字段）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 账号名称 |
| `notes` | *string | 备注 |
| `type` | string | 类型：`oauth` / `setup-token` / `apikey` / `upstream` |
| `credentials` | object | 凭证信息 |
| `extra` | object | 额外配置 |
| `proxy_id` | *int64 | 代理 ID |
| `concurrency` | *int | 并发数 |
| `priority` | *int | 优先级（默认 50） |
| `rate_multiplier` | *float64 | 速率倍数 |
| `status` | string | 状态：`active` / `inactive` |
| `group_ids` | *[]int64 | 分组 ID 列表 |
| `expires_at` | *int64 | 过期时间戳 |
| `auto_pause_on_expired` | *bool | 过期后自动暂停 |

> 使用指针类型（`*`）的字段可以区分"未提供"和"设置为零值"。

```bash
# 示例：更新账号优先级为 100
curl -X PUT "${BASE}/api/v1/admin/accounts/1" \
  -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"priority": 100}'
```

#### 1.12 批量更新账号

```
POST /api/v1/admin/accounts/bulk-update
```

**请求体**（JSON）：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `account_ids` | []int64 | **是** | 要更新的账号 ID 列表 |
| `priority` | *int | 否 | 优先级 |
| `concurrency` | *int | 否 | 并发数 |
| `rate_multiplier` | *float64 | 否 | 速率倍数 |
| `status` | string | 否 | 状态：`active` / `inactive` / `error` |
| `schedulable` | *bool | 否 | 是否可调度 |
| `group_ids` | *[]int64 | 否 | 分组 ID 列表 |
| `proxy_id` | *int64 | 否 | 代理 ID |
| `credentials` | object | 否 | 凭证信息（批量覆盖） |
| `extra` | object | 否 | 额外配置（批量覆盖） |

```bash
# 示例：批量设置多个账号优先级为 100
curl -X POST "${BASE}/api/v1/admin/accounts/bulk-update" \
  -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"account_ids": [1, 2, 3], "priority": 100}'
```

#### 1.13 批量测试账号（脚本）

批量测试指定平台所有账号的指定模型连通性：

```bash
# 用户需提供：BASE（环境地址）、KEY（admin token）、MODEL（测试模型）
ACCOUNT_IDS=$(curl -s "${BASE}/api/v1/admin/accounts?platform=antigravity&page=1&page_size=100" \
  -H "x-api-key: ${KEY}" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for item in data['data']['items']:
    print(f\"{item['id']}|{item['name']}\")
")

while IFS='|' read -r ID NAME; do
    echo "测试账号 ID=${ID} (${NAME})..."
    RESPONSE=$(curl -s --max-time 60 -N \
      -X POST "${BASE}/api/v1/admin/accounts/${ID}/test" \
      -H "x-api-key: ${KEY}" \
      -H "Content-Type: application/json" \
      -d "{\"model_id\": \"${MODEL}\"}" 2>&1)
    if echo "$RESPONSE" | grep -q '"success":true'; then
        echo "  ✅ 成功"
    elif echo "$RESPONSE" | grep -q '"type":"content"'; then
        echo "  ✅ 成功（有内容响应）"
    else
        ERROR_MSG=$(echo "$RESPONSE" | grep -o '"error":"[^"]*"' | tail -1)
        echo "  ❌ 失败: ${ERROR_MSG}"
    fi
done <<< "$ACCOUNT_IDS"
```

---

### 2. 运维监控

#### 2.1 并发统计

```
GET /api/v1/admin/ops/concurrency
```

```bash
curl -s "${BASE}/api/v1/admin/ops/concurrency" -H "x-api-key: ${KEY}"
```

#### 2.2 账号可用性

```
GET /api/v1/admin/ops/account-availability
```

```bash
curl -s "${BASE}/api/v1/admin/ops/account-availability" -H "x-api-key: ${KEY}"
```

#### 2.3 实时流量摘要

```
GET /api/v1/admin/ops/realtime-traffic
```

```bash
curl -s "${BASE}/api/v1/admin/ops/realtime-traffic" -H "x-api-key: ${KEY}"
```

#### 2.4 请求错误列表

```
GET /api/v1/admin/ops/request-errors
```

**查询参数**：`page`、`page_size`

```bash
curl -s "${BASE}/api/v1/admin/ops/request-errors?page=1&page_size=50" \
  -H "x-api-key: ${KEY}"
```

#### 2.5 上游错误列表

```
GET /api/v1/admin/ops/upstream-errors
```

```bash
curl -s "${BASE}/api/v1/admin/ops/upstream-errors?page=1&page_size=50" \
  -H "x-api-key: ${KEY}"
```

#### 2.6 仪表板概览

```
GET /api/v1/admin/ops/dashboard/overview
```

```bash
curl -s "${BASE}/api/v1/admin/ops/dashboard/overview" -H "x-api-key: ${KEY}"
```

---

### 3. 系统设置

#### 3.1 获取系统设置

```
GET /api/v1/admin/settings
```

```bash
curl -s "${BASE}/api/v1/admin/settings" -H "x-api-key: ${KEY}"
```

#### 3.2 更新系统设置

```
PUT /api/v1/admin/settings
```

```bash
curl -X PUT "${BASE}/api/v1/admin/settings" \
  -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{ ... }'
```

#### 3.3 Admin API Key 状态（脱敏）

```
GET /api/v1/admin/settings/admin-api-key
```

```bash
curl -s "${BASE}/api/v1/admin/settings/admin-api-key" -H "x-api-key: ${KEY}"
```

---

### 4. 用户管理

#### 4.1 用户列表

```
GET /api/v1/admin/users
```

```bash
curl -s "${BASE}/api/v1/admin/users?page=1&page_size=20" -H "x-api-key: ${KEY}"
```

#### 4.2 用户详情

```
GET /api/v1/admin/users/:id
```

```bash
curl -s "${BASE}/api/v1/admin/users/1" -H "x-api-key: ${KEY}"
```

#### 4.3 更新用户余额

```
POST /api/v1/admin/users/:id/balance
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/1/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" \
  -d '{"amount": 100, "reason": "充值"}'
```

---

### 5. 分组管理

#### 5.1 分组列表

```
GET /api/v1/admin/groups
```

```bash
curl -s "${BASE}/api/v1/admin/groups" -H "x-api-key: ${KEY}"
```

#### 5.2 所有分组（不分页）

```
GET /api/v1/admin/groups/all
```

```bash
curl -s "${BASE}/api/v1/admin/groups/all" -H "x-api-key: ${KEY}"
```

---

## 注意事项

1. **前端必须打包进镜像**：使用 `docker buildx build --builder limited-builder` 在生产服务器（`clicodeplus`）本机构建，Dockerfile 会自动编译前端并 embed 到后端二进制中

2. **镜像标签**：docker-compose.yml 使用 `weishaw/sub2api:latest`，本地构建后需要 `docker tag` 覆盖

3. **Windows 换行符问题**：已通过 `.gitattributes` 解决，确保 `*.sql` 文件始终使用 LF

4. **版本号管理**：每次发布必须更新 `backend/cmd/server/VERSION` 并打标签

5. **合并冲突**：合并上游新版本时，重点关注以下文件可能的冲突：
   - `backend/internal/service/antigravity_gateway_service.go`
   - `backend/internal/service/gateway_service.go`
   - `backend/internal/pkg/antigravity/request_transformer.go`

---

## Go 代码规范

### 1. 函数设计

#### 单一职责原则
- **函数行数**：单个函数常规不应超过 **30 行**，超过时应拆分为子函数。若某段逻辑确实不可拆分（如复杂的状态机、协议解析等），可以例外，但需添加注释说明原因
- **嵌套层级**：避免超过 3 层嵌套，使用 early return 减少嵌套

```go
// ❌ 不推荐：深层嵌套
func process(data []Item) {
    for _, item := range data {
        if item.Valid {
            if item.Type == "A" {
                if item.Status == "active" {
                    // 业务逻辑...
                }
            }
        }
    }
}

// ✅ 推荐：early return
func process(data []Item) {
    for _, item := range data {
        if !item.Valid {
            continue
        }
        if item.Type != "A" {
            continue
        }
        if item.Status != "active" {
            continue
        }
        // 业务逻辑...
    }
}
```

#### 复杂逻辑提取
将复杂的条件判断或处理逻辑提取为独立函数：

```go
// ❌ 不推荐：内联复杂逻辑
if resp.StatusCode == 429 || resp.StatusCode == 503 {
    // 80+ 行处理逻辑...
}

// ✅ 推荐：提取为独立函数
result := handleRateLimitResponse(resp, params)
switch result.action {
case actionRetry:
    continue
case actionBreak:
    return result.resp, nil
}
```

### 2. 重复代码消除

#### 配置获取模式
将重复的配置获取逻辑提取为方法：

```go
// ❌ 不推荐：重复代码
logBody := s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.LogUpstreamErrorBody
maxBytes := 2048
if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes > 0 {
    maxBytes = s.settingService.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
}

// ✅ 推荐：提取为方法
func (s *Service) getLogConfig() (logBody bool, maxBytes int) {
    maxBytes = 2048
    if s.settingService == nil || s.settingService.cfg == nil {
        return false, maxBytes
    }
    cfg := s.settingService.cfg.Gateway
    if cfg.LogUpstreamErrorBodyMaxBytes > 0 {
        maxBytes = cfg.LogUpstreamErrorBodyMaxBytes
    }
    return cfg.LogUpstreamErrorBody, maxBytes
}
```

### 3. 常量管理

#### 避免魔法数字
所有硬编码的数值都应定义为常量：

```go
// ❌ 不推荐
if retryDelay >= 10*time.Second {
    resetAt := time.Now().Add(30 * time.Second)
}

// ✅ 推荐
const (
    rateLimitThreshold       = 10 * time.Second
    defaultRateLimitDuration = 30 * time.Second
)

if retryDelay >= rateLimitThreshold {
    resetAt := time.Now().Add(defaultRateLimitDuration)
}
```

#### 注释引用常量名
在注释中引用常量名而非硬编码值：

```go
// ❌ 不推荐
// < 10s: 等待后重试

// ✅ 推荐
// < rateLimitThreshold: 等待后重试
```

### 4. 错误处理

#### 使用结构化日志
优先使用 `slog` 进行结构化日志记录：

```go
// ❌ 不推荐
log.Printf("%s status=%d model_rate_limit_failed model=%s error=%v", prefix, statusCode, modelName, err)

// ✅ 推荐
slog.Error("failed to set model rate limit",
    "prefix", prefix,
    "status_code", statusCode,
    "model", modelName,
    "error", err,
)
```

### 5. 测试规范

#### Mock 函数签名同步
修改函数签名时，必须同步更新所有测试中的 mock 函数：

```go
// 如果修改了 handleError 签名
handleError func(..., groupID int64, sessionHash string) *Result

// 必须同步更新测试中的 mock
handleError: func(..., groupID int64, sessionHash string) *Result {
    return nil
},
```

#### 测试构建标签
统一使用测试构建标签：

```go
//go:build unit

package service
```

### 6. 时间格式解析

#### 使用标准库
优先使用 `time.ParseDuration`，支持所有 Go duration 格式：

```go
// ❌ 不推荐：手动限制格式
if !strings.HasSuffix(delay, "s") || strings.Contains(delay, "m") {
    continue
}

// ✅ 推荐：使用标准库
dur, err := time.ParseDuration(delay) // 支持 "0.5s", "4m50s", "1h30m" 等
```

### 7. 接口设计

#### 接口隔离原则
定义最小化接口，只包含必需的方法：

```go
// ❌ 不推荐：使用过于宽泛的接口
type AccountRepository interface {
    // 20+ 个方法...
}

// ✅ 推荐：定义最小化接口
type ModelRateLimiter interface {
    SetModelRateLimit(ctx context.Context, id int64, modelKey string, resetAt time.Time) error
}
```

### 8. 并发安全

#### 共享数据保护
访问可能被并发修改的数据时，确保线程安全：

```go
// 如果 Account.Extra 可能被并发修改
// 需要使用互斥锁或原子操作保护读取
func (a *Account) GetRateLimitRemainingTime(model string) time.Duration {
    a.mu.RLock()
    defer a.mu.RUnlock()
    // 读取 Extra 字段...
}
```

### 9. 命名规范

#### 一致的命名风格
- 常量使用 camelCase：`rateLimitThreshold`
- 类型使用 PascalCase：`AntigravityQuotaScope`
- 同一概念使用统一命名：`Threshold` 或 `Limit`，不要混用

```go
// ❌ 不推荐：命名不一致
antigravitySmartRetryMinWait    // 使用 Min
antigravityRateLimitThreshold   // 使用 Threshold

// ✅ 推荐：统一风格
antigravityMinRetryWait
antigravityRateLimitThreshold
```

### 10. 代码审查清单

在提交代码前，检查以下项目：

- [ ] 函数是否超过 30 行？（不可拆分的逻辑除外，需注释说明）
- [ ] 嵌套是否超过 3 层？
- [ ] 是否有重复代码可以提取？
- [ ] 是否使用了魔法数字？
- [ ] Mock 函数签名是否与实际函数一致？
- [ ] 测试是否覆盖了新增逻辑？
- [ ] 日志是否包含足够的上下文信息？
- [ ] 是否考虑了并发安全？

---

## CI 检查与发布门禁

### GitHub Actions 检查项

本项目有 4 个 CI 任务，**任何代码推送或发布前都必须全部通过**：

| Workflow | Job | 说明 | 本地验证命令 |
|----------|-----|------|-------------|
| CI | `test` | 单元测试 + 集成测试 | `cd backend && make test-unit && make test-integration` |
| CI | `golangci-lint` | Go 代码静态检查（golangci-lint v2.7） | `cd backend && golangci-lint run --timeout=5m` |
| Security Scan | `backend-security` | govulncheck + gosec 安全扫描 | `cd backend && govulncheck ./... && gosec -severity high -confidence high ./...` |
| Security Scan | `frontend-security` | pnpm audit 前端依赖安全检查 | `cd frontend && pnpm audit --prod --audit-level=high` |

### 向上游提交 PR

PR 目标是上游官方仓库，**只包含通用功能改动**（bug fix、新功能、性能优化等）。

**以下文件禁止出现在 PR 中**（属于我们 fork 的定制化内容）：
- `CLAUDE.md`、`AGENTS.md` — 我们的开发文档
- `backend/cmd/server/VERSION` — 我们的版本号文件
- UI 定制改动（GitHub 链接移除、微信客服按钮、首页定制等）
- 部署配置（`deploy/` 目录下的定制修改）

**PR 流程**：
1. 从我们的当前开发分支（如 `release/custom-0.1.93`）或对应功能分支创建 PR 分支，只包含要提交给上游的通用改动
2. 推送分支后，**等待 4 个 CI job 全部通过**
3. 确认通过后再创建 PR
4. 使用 `gh run list --repo touwaeriol/sub2api --branch <branch>` 检查状态

### 自有分支推送（release/custom-X.Y.Z / 功能分支 / main）

推送到我们自己的 `release/custom-X.Y.Z`、其他开发功能分支或 `main` 分支时，包含所有改动（定制化 + 通用功能）。

**推送前必须在本地执行全部 CI 检查**（不要等 GitHub Actions）：

```bash
# 确保 Go 工具链可用（macOS homebrew）
export PATH="/opt/homebrew/bin:$HOME/go/bin:$PATH"

# 1. 单元测试（必须）
cd backend && make test-unit

# 2. 集成测试（推荐，需要 Docker）
make test-integration

# 3. golangci-lint 静态检查（必须）
golangci-lint run --timeout=5m

# 4. gofmt 格式检查（必须）
gofmt -l ./...
# 如果有输出，运行 gofmt -w <file> 修复
```

**推送后确认**：
1. 使用 `gh run list --repo touwaeriol/sub2api --branch <branch>` 检查 GitHub Actions 状态
2. 确认 CI 和 Security Scan 两个 workflow 的 4 个 job 全部绿色 ✅
3. 任何 job 失败必须立即修复，**禁止在 CI 未通过的状态下继续后续操作**

### 发布版本

1. 本地执行上述全部 CI 检查通过
2. 递增 `backend/cmd/server/VERSION`，提交并推送
3. 推送后确认 GitHub Actions 的 4 个 CI job 全部通过
4. **CI 未通过时禁止部署** — 必须先修复问题
5. 使用 `gh run list --repo touwaeriol/sub2api --limit 10` 确认状态

### 常见 CI 失败原因及修复
- **gofmt**：struct 字段对齐不一致 → 运行 `gofmt -w <file>` 修复
- **golangci-lint**：未使用的变量/导入 → 删除或使用 `_` 忽略
- **test 失败**：mock 函数签名不一致 → 同步更新 mock
- **gosec**：安全漏洞 → 根据提示修复或添加例外

---

## PR 描述格式规范

所有 PR 描述使用中英文同步（先中文、后英文），包含以下三个部分：

### 模板

```markdown
## 背景 / Background

<一两句说明问题现状或触发原因>

<English version of the background>

---

## 目的 / Purpose

<本次改动要解决的问题或达到的目标>

<English version of the purpose>

---

## 改动内容 / Changes

### 后端 / Backend

- **改动点 1**：说明
- **改动点 2**：说明

---

- **Change 1**: description
- **Change 2**: description

### 前端 / Frontend

- **改动点 1**：说明
- **改动点 2**：说明

---

- **Change 1**: description
- **Change 2**: description

---

## 截图 / Screenshot（可选）

ASCII 示意图或实际截图
```

### 规范要点

- **标题**：使用 conventional commits 格式，如 `feat(scope): description`
- **中英文顺序**：同一段落先中文后英文，用空行分隔，不用 `---` 分割同段内容
- **改动分类**：按 Backend / Frontend / Config 等模块分组，先列中文要点再列英文要点
- **截图/示意图**：有 UI 变动时必须附上，可用 ASCII 示意布局
- **目标分支**：提交到 `touwaeriol/sub2api` 的 `main` 分支
