# PR 默认语义（最高优先级）

- 未特别说明时，文档或对话中的"PR"一律指**提交到上游仓库** `Wei-Shaw/sub2api:main`
- 如果只是合并回我们自己的仓库，必须明确表述为"内部同步 PR" / "合并回我们的 main" / "fork 内部 PR"
- 禁止将"上游 PR"和"我们自己仓库内的同步 PR"混用为同一个概念

## Git remote 列表

| remote | 仓库 | 权限 | 用途 |
|--------|------|------|------|
| `origin` | `touwaeriol/sub2api` | 推送 | 我们的 fork，主开发仓库 |
| `upstream` | `Wei-Shaw/sub2api` | 只读（仅 PR） | 官方上游，所有"PR"默认指这里 |
| `business` | `Sub2API-Devs/sub2api-pro` | **直接推送** | 商业版上游，可直接 push（不走 PR）|

涉及 `business` 仓库的操作必须明确表述（如 "push 到 business"、"business 的 main"），避免与 `upstream` 概念混淆。

## 本地依赖联调

- 本地 `go-sora2api` 仓库固定路径：`C:\Users\user\project\GolandProjects\go-sora2api`
- 需要联调时优先使用 `backend/go.mod` 的 `replace` 指向该路径，而非 `git submodule`
- 联调完成后，提交/部署前切换为 fork 仓库的明确 tag 或 commit

---

# Sub2API 开发说明

## 版本管理策略

### 版本号规则

我们在官方版本号后追加自己的小版本号：官方 `v0.1.68` → 我们的 `v0.1.68.1`、`v0.1.68.2`（递增）

### 分支策略

| 分支 | 说明 |
|------|------|
| `main` | 我们的主分支，包含所有定制功能 |
| `release/custom-X.Y.Z` | 基于我们的 release 分支 + 上游 `vX.Y.Z` 合并 |
| `feat/*` | 功能分支，基于 `upstream/main`，用于提交上游 PR |
| `upstream/main` | 上游官方仓库（remote: upstream） |

---

## 发布流程（基于新官方版本）

> **核心原则**：始终从我们的 release 分支出发合并上游代码。**禁止**基于上游标签创建分支再合并我们的代码——会导致上游非相关改动以 auto-merge 方式混入，破坏定制功能。

```bash
# 1. 从我们的 release 出发，合并上游
git fetch upstream --tags
git checkout -b release/custom-0.1.110 release/custom-0.1.108
git merge v0.1.110 --no-edit
# 冲突时：定制代码优先保留，上游新功能/修复按需采纳

# 2. 验证（重点关注 gateway_service.go / openai_gateway_service.go /
#    antigravity_gateway_service.go / handler.go 的 auto-merge）
git diff release/custom-0.1.108 release/custom-0.1.110 --stat
cd backend && go build ./... && cd ..
cd frontend && pnpm build && cd ..

# 3. 更新版本号并打标签
echo "0.1.110.1" > backend/cmd/server/VERSION
git add backend/cmd/server/VERSION
git commit -m "chore: bump version to 0.1.110.1"
git tag v0.1.110.1
git push origin release/custom-0.1.110
git push origin v0.1.110.1

# 4. 同步回 main
git checkout main
git merge release/custom-0.1.110
git push origin main
```

### ⚠️ 注意事项

- **禁止反向合并**：不要 `git checkout v0.1.110 -b release/custom-0.1.110 && git merge main`，会以上游为基底，导致定制代码被覆盖或 auto-merge 错误
- **从 `feat/*` 取改动用 cherry-pick**：避免 `git merge` 引入 PR 分支的 upstream/main 基底代码
- **合并后必须全量测试**：beta 环境验证 API 转发、支付、认证等核心功能

---

## 热修复发布

```bash
git checkout release/custom-0.1.68
# ... 修复 ...
git commit -m "fix: 修复描述"

echo "0.1.68.2" > backend/cmd/server/VERSION
git add backend/cmd/server/VERSION
git commit -m "chore: bump version to 0.1.68.2"
git tag v0.1.68.2
git push origin release/custom-0.1.68
git push origin v0.1.68.2

# 同步到 main
git checkout main
git cherry-pick <fix-commit-hash>
git push origin main
```

---

## 服务器部署

### 前置条件

- SSH 别名 `clicodeplus` 连接生产服务器（运行 + 构建）
- 部署目录：`/root/sub2api`（正式）、`/root/sub2api-beta`（测试）、`/root/sub2api-openai`、`/root/sub2api-star`
- **镜像在生产服务器本机构建**，使用资源限制的 `limited-builder`（3 核 CPU、4G 内存）

| 服务器 | SSH 别名 | 职责 |
|--------|----------|------|
| 生产 | `clicodeplus` | 拉代码、构建镜像、运行服务 |
| 数据库 | `db-clicodeplus` | PostgreSQL 16 + Redis 7（所有环境共用） |

> 数据库运维手册：`db-clicodeplus:/root/README.md`

### 构建器（limited-builder）

```bash
# 标准构建命令（所有环境）
ssh clicodeplus "cd /root/sub2api && docker buildx build --builder limited-builder --no-cache --load -t sub2api:latest -f Dockerfile ."

# 状态检查
ssh clicodeplus "docker buildx inspect limited-builder"

# 构建器被删时重建：
ssh clicodeplus "docker buildx create --name limited-builder --driver docker-container --driver-opt 'default-load=true' && docker buildx inspect --builder limited-builder --bootstrap && docker update --cpus=3 --memory=4g --memory-swap=4g buildx_buildkit_limited-builder0"
```

### 部署环境一览

| 环境 | 目录 | 端口 | DB 用户/库 | Redis DB | 容器名 | 镜像标签 |
|------|------|------|-----------|----------|--------|----------|
| 正式 | `/root/sub2api` | 8080 | `sub2api` | 0 | `sub2api` | `sub2api:latest` |
| Beta | `/root/sub2api-beta` | 8084 | `beta` | 2 | `sub2api-beta` | `sub2api:beta` |
| OpenAI | `/root/sub2api-openai` | 8083 | `openai` | 3 | `sub2api-openai` | — |
| Star | `/root/sub2api-star` | 8086 | `star` | 4 | `sub2api-star` | — |

### 外部数据库与 Redis

所有环境共用 `db.clicodeplus.com`（PostgreSQL 16 + Redis 7），不使用容器内 DB/Redis。

**配置方式**：
- DB：`.env` 中 `DATABASE_HOST`、`DATABASE_SSLMODE`、`POSTGRES_USER`、`POSTGRES_PASSWORD`、`POSTGRES_DB`
- Redis：`docker-compose.override.yml` 覆盖 `REDIS_HOST=db.clicodeplus.com`（主 compose 硬编码为 `redis`），密码用 `REDIS_PASSWORD`
- 各环境 override 已通过 `depends_on: !reset {}` 和 `redis: profiles: [disabled]` 解除容器 Redis 依赖

**DB 操作示例**（用 `source .env` 避免暴露密码）：

```bash
# 查询迁移记录
ssh clicodeplus "source /root/sub2api/deploy/.env && PGPASSWORD=\"\$POSTGRES_PASSWORD\" psql -h \$DATABASE_HOST -U \$POSTGRES_USER -d \$POSTGRES_DB -c 'SELECT * FROM schema_migrations ORDER BY applied_at DESC LIMIT 5;'"

# 清除指定迁移记录（重新执行迁移）
ssh clicodeplus "source /root/sub2api-beta/deploy/.env && PGPASSWORD=\"\$POSTGRES_PASSWORD\" psql -h \$DATABASE_HOST -U \$POSTGRES_USER -d \$POSTGRES_DB -c \"DELETE FROM schema_migrations WHERE filename LIKE '%049%';\""
```

### 标准部署流程（每次部署都必须递增版本号）

```bash
# 0. 本地：递增版本号并推送
echo "0.1.69.2" > backend/cmd/server/VERSION
git add backend/cmd/server/VERSION
git commit -m "chore: bump version to 0.1.69.2"
git push origin release/custom-0.1.69      # 必须看到推送成功，无 rejected

# 1. 生产服务器：拉取代码 + 验证版本一致
ssh clicodeplus "cd /root/sub2api && git fetch fork && git checkout -B release/custom-0.1.69 fork/release/custom-0.1.69"
ssh clicodeplus "cat /root/sub2api/backend/cmd/server/VERSION"

# 2. 构建镜像
ssh clicodeplus "cd /root/sub2api && docker buildx build --builder limited-builder --no-cache --load -t sub2api:latest -f Dockerfile ."

# 3. 更新标签并重启
ssh clicodeplus "docker tag sub2api:latest weishaw/sub2api:latest"
ssh clicodeplus "cd /root/sub2api/deploy && docker compose up -d --force-recreate sub2api"

# 4. 验证
ssh clicodeplus "docker logs sub2api --tail 20"
ssh clicodeplus "docker ps | grep sub2api"   # 必须 healthy
```

> **构建问题排查**：构建器未启动 → `docker buildx inspect --builder limited-builder --bootstrap`；磁盘不足 → `docker system prune -f`；构建器被删 → 见上方"构建器"重建命令。

### Beta 并行部署（不影响现网）

**设计原则**：独立目录 `/root/sub2api-beta`、独立端口 8084、独立 compose project（`-p sub2api-beta`），敏感信息只放 `.env`。

**首次部署**：

```bash
# 1) 拉代码并构建 beta 镜像
ssh clicodeplus "cd /root/sub2api-beta && git fetch --all --tags && git checkout -f release/custom-0.1.71 && git reset --hard origin/release/custom-0.1.71"
ssh clicodeplus "cd /root/sub2api-beta && docker buildx build --builder limited-builder --no-cache --load -t sub2api:beta -f Dockerfile ."

# 2) 准备 .env（从现网复制，仅改 SERVER_PORT/POSTGRES_USER/POSTGRES_DB）
ssh clicodeplus
cd /root/sub2api-beta/deploy
cp -f /root/sub2api/deploy/.env ./.env
perl -pi -e 's/^SERVER_PORT=.*/SERVER_PORT=8084/' ./.env
perl -pi -e 's/^POSTGRES_USER=.*/POSTGRES_USER=beta/' ./.env
perl -pi -e 's/^POSTGRES_DB=.*/POSTGRES_DB=beta/' ./.env

# 3) compose override
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

# 4) 启动并验证
docker compose -p sub2api-beta --env-file .env -f docker-compose.yml -f docker-compose.override.yml up -d
curl -fsS http://127.0.0.1:8084/health
```

> 数据库侧需已存在 `beta` 用户与库并授权，否则容器会启动失败。

**更新 beta**：

```bash
ssh clicodeplus "cd /root/sub2api-beta && git fetch --all --tags && git checkout -f release/custom-0.1.71 && git reset --hard origin/release/custom-0.1.71"
ssh clicodeplus "cd /root/sub2api-beta && docker buildx build --builder limited-builder --no-cache --load -t sub2api:beta -f Dockerfile ."
ssh clicodeplus "cd /root/sub2api-beta/deploy && docker compose -p sub2api-beta --env-file .env -f docker-compose.yml -f docker-compose.override.yml up -d --no-deps --force-recreate sub2api"
ssh clicodeplus "sleep 5 && curl -fsS http://127.0.0.1:8084/health"
```

**停止 beta**：`docker compose -p sub2api-beta -f docker-compose.yml -f docker-compose.override.yml down`

### 服务器首次部署（一次性）

```bash
ssh clicodeplus
cd /root && git clone https://github.com/Wei-Shaw/sub2api.git && cd sub2api
git remote add fork https://github.com/touwaeriol/sub2api.git
git fetch fork && git checkout -B release/custom-0.1.69 fork/release/custom-0.1.69

cd deploy && cp .env.example .env
vim .env   # DATABASE_HOST=db.clicodeplus.com、POSTGRES_PASSWORD、REDIS_PASSWORD、JWT_SECRET 等

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

# 创建 limited-builder（首次）
docker buildx create --name limited-builder --driver docker-container --driver-opt "default-load=true"
docker buildx inspect --builder limited-builder --bootstrap
docker update --cpus=3 --memory=4g --memory-swap=4g buildx_buildkit_limited-builder0

# 构建并启动
cd /root/sub2api
docker buildx build --builder limited-builder --no-cache --load -t sub2api:latest -f Dockerfile .
docker tag sub2api:latest weishaw/sub2api:latest
cd /root/sub2api/deploy && docker compose up -d
```

### 常用运维命令

```bash
docker logs -f sub2api                  # 实时日志
docker compose restart sub2api          # 重启
docker compose down                     # 停止
docker compose down -v                  # ⚠️ 停止并删除数据卷（删数据库数据）
docker stats sub2api                    # 资源使用
```

### 部署注意事项

1. **前端必须打包进镜像**：Dockerfile 自动编译前端并 embed 到后端二进制
2. **镜像标签**：docker-compose.yml 用 `weishaw/sub2api:latest`，构建后需 `docker tag` 覆盖
3. **Windows 换行符**：`.gitattributes` 已确保 `*.sql` 使用 LF
4. **版本号管理**：每次发布必须更新 `backend/cmd/server/VERSION` 并打标签

---

## Admin API 接口

### ⚠️ 接口流程

收到操作 Web 界面的新需求但本文档未记录对应接口时：
1. 搜索代码（`backend/internal/server/routes/`、`backend/internal/handler/admin/`）确定端点、方法、请求体
2. 把新接口补充到本节
3. 执行操作

### 认证 / 环境

- 认证头：`x-api-key: admin-xxx`
- Admin API Key 存放在项目根目录 `.env` 的 `ADMIN_API_KEY`（被 `.gitignore` 排除）。401 时提示用户提供新 token 并更新 `.env`。**禁止把实际 token 写入文档/代码**
- 操作前 `source .env` 加载 `KEY=$ADMIN_API_KEY`

| 环境 | 基础地址 |
|------|----------|
| 正式 | `https://clicodeplus.com` |
| Beta | `http://<服务器IP>:8084`（仅内网） |
| OpenAI | `http://<服务器IP>:8083`（仅内网） |
| Star | `https://hyntoken.com` |

`${BASE}` = 环境基础地址，`${KEY}` = Admin API Key。

### 1. 账号管理

| 接口 | 方法 + 路径 | 说明 |
|------|-------------|------|
| 列表 | `GET /api/v1/admin/accounts` | 参数：`platform` / `type` / `status` / `search` / `page` / `page_size` |
| 详情 | `GET /api/v1/admin/accounts/:id` | |
| 测试连接 | `POST /api/v1/admin/accounts/:id/test` | Body `{"model_id": "..."}`（可选），SSE 流响应 |
| 清限流 | `POST /api/v1/admin/accounts/:id/clear-rate-limit` | |
| 清错误 | `POST /api/v1/admin/accounts/:id/clear-error` | |
| 可用模型 | `GET /api/v1/admin/accounts/:id/models` | |
| 刷新 token | `POST /api/v1/admin/accounts/:id/refresh` | |
| 刷新等级 | `POST /api/v1/admin/accounts/:id/refresh-tier` | |
| 统计 | `GET /api/v1/admin/accounts/:id/stats` | |
| 用量 | `GET /api/v1/admin/accounts/:id/usage` | |
| 更新单个 | `PUT /api/v1/admin/accounts/:id` | 可选字段见下方 |
| 批量更新 | `POST /api/v1/admin/accounts/bulk-update` | `account_ids` 必填 + 共享字段 |

**更新字段**（`PUT /:id` 与 bulk-update 通用，指针 `*` 表示可区分零值与未提供）：

```
name, notes(*), type, credentials, extra, proxy_id(*),
concurrency(*), priority(*), rate_multiplier(*), status,
schedulable(*), group_ids(*), expires_at(*), auto_pause_on_expired(*)
```

`type`: `oauth` / `setup-token` / `apikey` / `upstream`
`status`: `active` / `inactive` / `error`

**SSE 测试事件**：`test_start`(model) / `content`(text) / `test_end`(success, error) / `error`(text)

**示例**：

```bash
# 列表
curl -s "${BASE}/api/v1/admin/accounts?platform=antigravity&page=1&page_size=100" -H "x-api-key: ${KEY}"

# 测试
curl -N -X POST "${BASE}/api/v1/admin/accounts/1/test" \
  -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
  -d '{"model_id":"claude-opus-4-6"}'

# 单个/批量更新
curl -X PUT "${BASE}/api/v1/admin/accounts/1" -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" -d '{"priority":100}'
curl -X POST "${BASE}/api/v1/admin/accounts/bulk-update" -H "x-api-key: ${KEY}" \
  -H "Content-Type: application/json" -d '{"account_ids":[1,2,3],"priority":100}'
```

**批量测试脚本**（用户提供 BASE/KEY/MODEL）：

```bash
ACCOUNT_IDS=$(curl -s "${BASE}/api/v1/admin/accounts?platform=antigravity&page=1&page_size=100" \
  -H "x-api-key: ${KEY}" | python3 -c "
import json, sys
for item in json.load(sys.stdin)['data']['items']:
    print(f\"{item['id']}|{item['name']}\")")
while IFS='|' read -r ID NAME; do
    echo "测试 ID=${ID} (${NAME})..."
    R=$(curl -s --max-time 60 -N -X POST "${BASE}/api/v1/admin/accounts/${ID}/test" \
      -H "x-api-key: ${KEY}" -H "Content-Type: application/json" \
      -d "{\"model_id\":\"${MODEL}\"}" 2>&1)
    if echo "$R" | grep -q '"success":true' || echo "$R" | grep -q '"type":"content"'; then
        echo "  ✅ 成功"
    else
        echo "  ❌ 失败: $(echo "$R" | grep -o '"error":"[^"]*"' | tail -1)"
    fi
done <<< "$ACCOUNT_IDS"
```

### 2. 运维监控

```
GET  /api/v1/admin/ops/concurrency
GET  /api/v1/admin/ops/account-availability
GET  /api/v1/admin/ops/realtime-traffic
GET  /api/v1/admin/ops/request-errors?page=1&page_size=50
GET  /api/v1/admin/ops/upstream-errors?page=1&page_size=50
GET  /api/v1/admin/ops/dashboard/overview
```

### 3. 系统设置

```
GET  /api/v1/admin/settings
PUT  /api/v1/admin/settings
GET  /api/v1/admin/settings/admin-api-key   # 脱敏
```

### 4. 用户管理

```
GET  /api/v1/admin/users?page=1&page_size=20
GET  /api/v1/admin/users/:id
POST /api/v1/admin/users/:id/balance        # Body: {"amount":100,"reason":"充值"}
```

### 5. 分组管理

```
GET  /api/v1/admin/groups
GET  /api/v1/admin/groups/all
```

---

## Go / 前端代码规范

### 函数与结构

- 函数 ≤ 30 行（不可拆分逻辑除外，需注释说明）；嵌套 ≤ 3 层，用 early return 减少嵌套
- 复杂条件/处理逻辑提取为独立函数
- 重复的配置/状态判断提取为方法，避免散落的 `if a != nil && a.b != nil && ...`
- 接口定义最小化（接口隔离原则），不要塞 20+ 方法
- 重复逻辑必须提取公共方法（差异通过参数控制，而非复制粘贴）
- 兼容旧版本通过结果对象的字段（如 `resolved.Mode`）自然分支，禁止用散落的 `if hasNewFeature` 布尔开关
- 变量作用域最小化，禁止跨作用域传递布尔开关

### 常量与命名

- **禁止魔法值**：所有硬编码数值和业务字符串（状态值、模式标识、配置键名等）必须定义为常量
- 注释中引用常量名而非值（写 `< rateLimitThreshold` 而非 `< 10s`）
- 常量 camelCase（`rateLimitThreshold`），类型 PascalCase（`AntigravityQuotaScope`），同一概念统一命名（`Threshold` / `Limit` 不混用）
- 跨文件/组件共享的常量**只在一处定义**（如 `payment.OrderStatusPending`），其他地方引用；前后端共享值各自常量文件保持同步

### 日志与错误

- 优先用 `slog` 结构化日志，禁止 `log.Printf("...key=%v", ...)`
- 日志包含足够上下文，敏感数据脱敏（卡号、token、密码等）
- **API 错误响应格式**：`{ "code": <错误码>, "message": "<英文描述>", "details": { 上下文参数 } }`
  - `message`：英文，给开发者调试用，前端不直接展示
  - 前端根据 `code` 和 `details` 拼装 i18n 文案，后端不负责拼接用户文案

### 前端 API 错误处理

所有前端 catch 块**必须**用 `extractApiErrorMessage()` 提取错误：

```typescript
import { extractApiErrorMessage } from '@/utils/apiError'
catch (err: unknown) {
  appStore.showError(extractApiErrorMessage(err, t('common.error')))
}
```

原因：API 客户端拦截器把后端错误转为 `{ status, code, message }` 普通对象（非 Error 实例），`err instanceof Error` 为 false，`String(err)` 得到 `[object Object]`。

### 测试

- 修改函数签名时**必须同步更新所有 mock 函数签名**
- 单元测试统一加构建标签 `//go:build unit`

### 时间格式

- 用 `time.ParseDuration` 解析时长，支持 `0.5s` / `4m50s` / `1h30m` 等所有 Go duration 格式，禁止手写格式限制（如 `strings.HasSuffix(delay, "s")`）

### 并发安全

- 访问可能被并发修改的数据（如 `Account.Extra`）需要互斥锁或原子操作保护
- **`sync.Once` 重置必须与使用方同锁**：避免 `RefreshProviders` 重置 `s.once` 时 `EnsureProviders` 不持锁导致竞态。统一用 `mutex + bool 标记` 替代 `sync.Once` 重置

### 文件拆分

- **行数限制**：Go ≤ 500 行（>300 评估拆分）；Vue ≤ 300 行；TS 工具文件 ≤ 200 行
- **按职责域拆分**，禁止按序号（`payment_service_1.go` ❌）。各文件共享同一 package 和 receiver type（如 `*PaymentService`），不影响外部调用方
  - 例：`payment_order.go`（订单 CRUD）/ `payment_fulfillment.go`（履约）/ `payment_refund.go`（退款）/ `payment_stats.go`（统计）

### 前端显示

- 浮点精度：价格单位换算用 `toPrecision(10)` 避免 IEEE 754 误差；倍率显示用自适应精度，确保小数值（0.001）不被截断为 0.00
- 条件渲染基于业务字段（`billing_mode`），不基于数据存在性（`image_count > 0`）
- 同类数值（倍率等）在管理端、用户端、CSV 导出**用相同的格式化函数**

### 前端 TypeScript 严格规范

- **禁止 `any`**（包括 catch 块），用 `unknown`：`catch (err: unknown)`、`let instance: Stripe | null = null`
- **禁止硬编码用户可见文本**，必须走 i18n（`t('payment.stripeNotConfigured')`）
- Vue Router query 值是 `string | string[] | undefined`，禁止 `as string` 断言，用 `String(route.query.order_id || '')`

### 支付系统专项

- **订单不可物理删除**：失败用 `UpdateOneID().SetStatus("FAILED").SetFailReason(err.Error())`，禁止 `DeleteOneID`。原因：provider 可能已扣款但网络超时，物理删除会导致永久丢失、审计断链
- **金额计算必须用 `shopspring/decimal`**：禁止 `int64(math.Round(amount * 100))` 等 float 裸算术。元/分等公共换算提取为 `payment` 包函数
- **循环解码必须设最大迭代数**（如 `maxDecodeIterations = 10`），防无限循环
- **HTTP 响应体必须用 `io.LimitReader`**（如 `1 << 20` 1MB），防 OOM
- **Webhook 日志禁记完整请求体**：截断 200 字节 + 仅 Debug 级别；Error 级只记 `bodyLen` 等元数据
- **加密密钥初始化必须校验**：`hex.DecodeString` 错误必须返回，长度必须 == 32 字节，校验失败拒绝启动

### 代码审查清单（提交前自查）

- [ ] 函数 ≤ 30 行？嵌套 ≤ 3 层？
- [ ] 文件 ≤ 500 行（Go）/ 300 行（Vue）？
- [ ] 是否有重复代码可提取？是否有魔法值？
- [ ] Mock 函数签名是否同步？测试是否覆盖新增逻辑？
- [ ] 日志是否含足够上下文？敏感数据是否脱敏？
- [ ] 是否考虑并发安全？
- [ ] 金额是否用 decimal 精确运算？
- [ ] HTTP 外部调用是否限制响应体大小？
- [ ] 前端是否有 `any` / 硬编码文本？
- [ ] 支付订单操作是否有物理删除？（禁止）
- [ ] 共享常量是否只在一处定义？

---

## CI 检查与发布门禁

### GitHub Actions（4 个 job 必须全绿）

| Workflow | Job | 本地命令 |
|----------|-----|---------|
| CI | `test` | `cd backend && make test-unit && make test-integration` |
| CI | `golangci-lint` | `cd backend && golangci-lint run --timeout=5m` |
| Security Scan | `backend-security` | `cd backend && govulncheck ./... && gosec -severity high -confidence high ./...` |
| Security Scan | `frontend-security` | `cd frontend && pnpm audit --prod --audit-level=high` |

### 自有分支推送（release / 功能分支 / main）

推送前必须本地执行：

```bash
export PATH="/opt/homebrew/bin:$HOME/go/bin:$PATH"
cd backend && make test-unit
make test-integration               # 推荐，需 Docker
golangci-lint run --timeout=5m
gofmt -l ./...                      # 有输出则 gofmt -w <file> 修复
```

推送后用 `gh run list --repo touwaeriol/sub2api --branch <branch>` 确认 4 个 job 全绿。**CI 未通过禁止部署。**

### 常见 CI 失败

- **gofmt**：struct 字段对齐 → `gofmt -w <file>`
- **golangci-lint**：未使用变量/导入 → 删除或 `_` 忽略
- **test 失败**：mock 签名不一致 → 同步更新
- **gosec**：根据提示修复或加例外

---

## 向上游提交 PR

PR 目标 `Wei-Shaw/sub2api:main`，**只包含通用功能改动**（bug fix、新功能、性能优化），不含 fork 定制。

### 创建 PR 分支

```bash
# 1. 从 upstream/main 创建分支
git fetch upstream
git checkout -b feat/my-feature upstream/main

# 2. 把 release 全部差量 squash 进来（不创建 commit）
git merge --squash release/custom-0.1.115

# 3. revert 掉 fork-only 文件 + 不属于本 PR 的功能改动
#    a) 完全 fork-only 整文件
git checkout HEAD -- CLAUDE.md AGENTS.md backend/cmd/server/VERSION \
  frontend/src/components/layout/AppHeader.vue \
  frontend/src/views/HomeView.vue \
  deploy/docker-compose.yml .gitattributes .gitignore
rm -f frontend/src/components/common/WechatServiceButton.vue \
  frontend/public/wechat-qr.jpg

#    b) 与本 PR 无关的功能文件
git checkout HEAD -- backend/internal/service/account_service.go \
  backend/internal/service/openai_*.go ...

#    c) 混合文件（i18n / sidebar / router / settings）需 surgical 编辑
git restore --staged --patch frontend/src/components/layout/AppSidebar.vue
git restore --staged --patch frontend/src/i18n/locales/en.ts

# 4. 验证只剩本功能改动
git diff --cached --name-only
git diff --cached --stat | tail -5

# 5. 一次提交并推送
git commit -m "feat(scope): description"
git push origin feat/my-feature
gh run list --repo touwaeriol/sub2api --branch feat/my-feature
```

**冲突处理**：
- 出现 `<<<<<<<` 标记按本 PR 功能选边
- 文件被 release 重命名（R100）但内容未改的，**保留 upstream 命名**：迁移已部署 DB 的 `schema_migrations` 不能凭空改名
- 拿不准时 `git checkout HEAD -- <path>`（保留上游版本），再单独决定是否带入

### 把 PR 改进带回 release（反向同步）

```bash
git checkout release/custom-0.1.108
git checkout -b release/custom-0.1.110
git cherry-pick <pr-improvement-commit1> <pr-improvement-commit2>
# ⚠️ 禁止 git merge feat/my-feature，会带入 upstream/main 全部代码
```

### 禁止出现在 PR 中的文件（fork 定制）

- `CLAUDE.md`、`AGENTS.md`、`backend/cmd/server/VERSION`
- UI 定制（GitHub 链接移除、微信客服按钮、首页定制）
- `deploy/` 下的部署配置
- `sora_client_enabled` 相关代码
- 测试脚本（`stress_test_*.sh`、`test_*.py`）
- partner logos（`assets/partners/`）

### PR 提交检查清单

1. 基于最新 `upstream/main` ✅
2. 只含通用功能代码，无 fork 定制 ✅
3. 4 个 CI job 全绿 ✅（`gh run list --repo touwaeriol/sub2api --branch <branch>`）
4. 描述符合中英文双语模板 ✅

---

## PR 描述模板（中英双语）

```markdown
## 背景 / Background

<一两句问题现状或触发原因>

<English version>

---

## 目的 / Purpose

<本次改动要解决的问题或目标>

<English version>

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

---

- **Change 1**: description

---

## 截图 / Screenshot（可选）

ASCII 示意图或实际截图
```

**要点**：
- 标题用 conventional commits（`feat(scope): description`）
- 同段先中文后英文，空行分隔，不用 `---` 分割同段
- 改动按 Backend / Frontend / Config 分组，先中文后英文
- UI 变动必须附截图/ASCII 示意
- 目标分支：`touwaeriol/sub2api` 的 `main`
