# sub2api 项目开发指南

> 本文档记录项目环境配置、常见坑点和注意事项，供 Claude Code 和团队成员参考。

## 一、项目基本信息

| 项目 | 说明 |
|------|------|
| **上游仓库** | Wei-Shaw/sub2api |
| **Fork 仓库** | bayma888/sub2api-bmai |
| **技术栈** | Go 后端 (Ent ORM + Gin) + Vue3 前端 (pnpm) |
| **数据库** | PostgreSQL 16 + Redis |
| **包管理** | 后端: go modules, 前端: **pnpm**（不是 npm） |

## 二、本地环境配置

### PostgreSQL 16 (Windows 服务)

| 配置项 | 值 |
|--------|-----|
| 端口 | 5432 |
| psql 路径 | `C:\Program Files\PostgreSQL\16\bin\psql.exe` |
| pg_hba.conf | `C:\Program Files\PostgreSQL\16\data\pg_hba.conf` |
| 数据库凭据 | user=`sub2api`, password=`sub2api`, dbname=`sub2api` |
| 超级用户 | user=`postgres`, password=`postgres` |

### Redis

| 配置项 | 值 |
|--------|-----|
| 端口 | 6379 |
| 密码 | 无 |

### 开发工具

```bash
# golangci-lint v2.7
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7

# pnpm (前端包管理)
npm install -g pnpm
```

## 三、CI/CD 流水线

### GitHub Actions Workflows

| Workflow | 触发条件 | 检查内容 |
|----------|----------|----------|
| **backend-ci.yml** | push, pull_request | 单元测试 + 集成测试 + golangci-lint v2.7 |
| **security-scan.yml** | push, pull_request, 每周一 | govulncheck + gosec + pnpm audit |
| **release.yml** | tag `v*` | 构建发布（PR 不触发） |

### CI 要求

- Go 版本由 `backend/go.mod` 决定（CI 使用 `go-version-file: backend/go.mod`，当前为 **1.26.5**）
- 前端使用 `pnpm install --frozen-lockfile`，必须提交 `pnpm-lock.yaml`

### 本地测试命令

```bash
# 后端单元测试
cd backend && go test -tags=unit ./...

# 后端集成测试
cd backend && go test -tags=integration ./...

# 代码质量检查
cd backend && golangci-lint run ./...

# 前端依赖安装（必须用 pnpm）
cd frontend && pnpm install
```

## 四、常见坑点 & 解决方案

### 坑 1：pnpm-lock.yaml 必须同步提交

**问题**：`package.json` 新增依赖后，CI 的 `pnpm install --frozen-lockfile` 失败。

**原因**：上游 CI 使用 pnpm，lock 文件不同步会报错。

**解决**：
```bash
cd frontend
pnpm install  # 更新 pnpm-lock.yaml
git add pnpm-lock.yaml
git commit -m "chore: update pnpm-lock.yaml"
```

---

### 坑 2：npm 和 pnpm 的 node_modules 冲突

**问题**：之前用 npm 装过 `node_modules`，pnpm install 报 `EPERM` 错误。

**解决**：
```bash
cd frontend
rm -rf node_modules  # 或 PowerShell: Remove-Item -Recurse -Force node_modules
pnpm install
```

---

### 坑 3：PowerShell 中 bcrypt hash 的 `$` 被转义

**问题**：bcrypt hash 格式如 `$2a$10$xxx...`，PowerShell 把 `$2a` 当变量解析，导致数据丢失。

**解决**：将 SQL 写入文件，用 `psql -f` 执行：
```bash
# 错误示范（PowerShell 会吃掉 $）
psql -c "INSERT INTO users ... VALUES ('$2a$10$...')"

# 正确做法
echo "INSERT INTO users ... VALUES ('\$2a\$10\$...')" > temp.sql
psql -U sub2api -h 127.0.0.1 -d sub2api -f temp.sql
```

---

### 坑 4：psql 不支持中文路径

**问题**：`psql -f "D:\中文路径\file.sql"` 报错找不到文件。

**解决**：复制到纯英文路径再执行：
```bash
cp "D:\中文路径\file.sql" "C:\temp.sql"
psql -f "C:\temp.sql"
```

---

### 坑 5：PostgreSQL 密码重置流程

**场景**：忘记 PostgreSQL 密码。

**步骤**：
1. 修改 `C:\Program Files\PostgreSQL\16\data\pg_hba.conf`
   ```
   # 将 scram-sha-256 改为 trust
   host    all    all    127.0.0.1/32    trust
   ```
2. 重启 PostgreSQL 服务
   ```powershell
   Restart-Service postgresql-x64-16
   ```
3. 无密码登录并重置
   ```bash
   psql -U postgres -h 127.0.0.1
   ALTER USER sub2api WITH PASSWORD 'sub2api';
   ALTER USER postgres WITH PASSWORD 'postgres';
   ```
4. 改回 `scram-sha-256` 并重启

---

### 坑 6：Go interface 新增方法后 test stub 必须补全

**问题**：给 interface 新增方法后，编译报错 `does not implement interface (missing method XXX)`。

**原因**：所有测试文件中实现该 interface 的 stub/mock 都必须补上新方法。

**解决**：
```bash
# 搜索所有实现该 interface 的 struct
cd backend
grep -r "type.*Stub.*struct" internal/
grep -r "type.*Mock.*struct" internal/

# 逐一补全新方法
```

---

### 坑 7：Windows 上 psql 连 localhost 的 IPv6 问题

**问题**：psql 连 `localhost` 先尝试 IPv6 (::1)，可能报错后再回退 IPv4。

**建议**：直接用 `127.0.0.1` 代替 `localhost`。

---

### 坑 8：Windows 没有 make 命令

**问题**：CI 里用 `make test-unit`，本地 Windows 没有 make。

**解决**：直接用 Makefile 里的原始命令：
```bash
# 代替 make test-unit
go test -tags=unit ./...

# 代替 make test-integration
go test -tags=integration ./...
```

---

### 坑 9：Ent Schema 修改后必须重新生成

**问题**：修改 `ent/schema/*.go` 后，代码不生效。

**解决**：
```bash
cd backend
go generate ./ent  # 重新生成 ent 代码
git add ent/       # 生成的文件也要提交
```

---

### 坑 10：前端测试看似正常，但后端调用失败（模型映射被批量误改）

**典型现象**：
- 前端按钮点测看起来正常；
- 实际通过 API/客户端调用时返回 `Service temporarily unavailable` 或提示无可用账号；
- 常见于 OpenAI 账号（例如 Codex 模型）在批量修改后突然不可用。

**根因**：
- OpenAI 账号编辑页默认不显式展示映射规则，容易让人误以为“没映射也没关系”；
- 但在**批量修改同时选中不同平台账号**（OpenAI + Antigravity/Gemini）时，模型白名单/映射可能被跨平台策略覆盖；
- 结果是 OpenAI 账号的关键模型映射丢失或被改坏，后端选不到可用账号。

**修复方案（按优先级）**：
1. **快速修复（推荐）**：在批量修改中补回正确的透传映射（例如 `gpt-5.3-codex -> gpt-5.3-codex-spark`）。
2. **彻底重建**：删除并重新添加全部相关账号（最稳但成本高）。

**关键经验**：
- 如果某模型已被软件内置默认映射覆盖，通常不需要额外再加透传；
- 但当上游模型更新快于本仓库默认映射时，**手动批量添加透传映射**是最简单、最低风险的临时兜底方案；
- 批量操作前尽量按平台分组，不要混选不同平台账号。

---

### 坑 11：PR 提交前检查清单

提交 PR 前务必本地验证：

- [ ] `go test -tags=unit ./...` 通过
- [ ] `go test -tags=integration ./...` 通过
- [ ] `golangci-lint run ./...` 无新增问题
- [ ] `pnpm-lock.yaml` 已同步（如果改了 package.json）
- [ ] 所有 test stub 补全新接口方法（如果改了 interface）
- [ ] Ent 生成的代码已提交（如果改了 schema）

## 五、常用命令速查

### 数据库操作

```bash
# 连接数据库
psql -U sub2api -h 127.0.0.1 -d sub2api

# 查看所有用户
psql -U postgres -h 127.0.0.1 -c "\du"

# 查看所有数据库
psql -U postgres -h 127.0.0.1 -c "\l"

# 执行 SQL 文件
psql -U sub2api -h 127.0.0.1 -d sub2api -f migration.sql
```

### Git 操作

```bash
# 同步上游
git fetch upstream
git checkout main
git merge upstream/main
git push origin main

# 创建功能分支
git checkout -b feature/xxx

# Rebase 到最新 main
git fetch upstream
git rebase upstream/main
```

### 前端操作

```bash
# 安装依赖（必须用 pnpm）
cd frontend
pnpm install

# 开发服务器
pnpm dev

# 构建
pnpm build
```

### 后端操作

```bash
# 运行服务器
cd backend
go run ./cmd/server/

# 生成 Ent 代码
go generate ./ent

# 运行测试
go test -tags=unit ./...
go test -tags=integration ./...

# Lint 检查
golangci-lint run ./...
```

## 六、项目结构速览

```
sub2api/
├── backend/
│   ├── cmd/server/          # 主程序入口（wire DI）
│   ├── ent/                 # Ent ORM 生成代码
│   │   └── schema/          # 数据库 Schema 定义
│   ├── internal/
│   │   ├── handler/         # HTTP 处理器（+ dto/ 子包）
│   │   ├── service/         # 应用服务（用例编排；仍持有未提取实体的 type alias）
│   │   ├── repository/      # 数据访问层（Ent/SQL/Redis 实现 port 接口）
│   │   ├── domain/          # 领域实体 + 错误 + 纯函数（跨 BC 共享内核）
│   │   ├── port/            # BC 仓储端口（接口；repository 实现、service 依赖）
│   │   ├── pkg/             # 通用工具库（不依赖 service/repository/handler）
│   │   ├── securityaudit/   # 提示词审计 BC（半独立）
│   │   ├── payment/         # 支付 provider 内核
│   │   └── server/          # 服务器配置 / 路由 / 中间件
│   ├── migrations/          # 数据库迁移脚本
│   └── config.yaml          # 配置文件
├── frontend/
│   ├── src/
│   │   ├── api/             # API 调用
│   │   ├── components/      # Vue 组件
│   │   ├── views/           # 页面视图
│   │   ├── types/           # TypeScript 类型
│   │   └── i18n/            # 国际化
│   ├── package.json         # 依赖配置
│   └── pnpm-lock.yaml       # pnpm 锁文件（必须提交）
└── .claude/
    └── CLAUDE.md            # 本文档
```

### 6.1 分层与 BC 端口约定

后端正按**有界上下文（BC）**增量拆分 `service` 上帝包。已提取的 BC 遵循以下层级，贡献者改 repository / service 时请遵循同一模式：

```
handler ──► service（应用）──► port/<bc> ──► repository（实现）
                │                  ▲
                └──  domain  ◄─────┘
```

- `internal/domain/`：实体、领域错误、纯函数（无外部依赖，除 `pkg/errors` 等）。
- `internal/port/<bc>/`：每个 BC 一个子包，只放**仓储/缓存端口接口**，签名用 `domain.*` 类型。
- `internal/repository/`：实现 `port/<bc>` 接口，签名用 `domain.*`。目标是**不再 import `internal/service`**，但若该 repo 还依赖其他未提取 BC 的 service 类型（缓存、探针、事件等），可暂留 import——以实体 + 端口落地为准，反转计数下降是最终态而非每一步硬指标。
- `internal/service/`：保留应用服务 + type alias（`type Group = domain.Group`、`type GroupRepository = portgroup.Repository`）+ 错误再导出，保证现有调用点与测试桩渐进迁移。

已提取的 BC（`internal/port/` 子包）：announcement、promo、redeem、tlsfingerprint、errorpassthrough、affiliate、proxy、group、user、apikey、setting、channel、channelmonitor、account、**usagelog**（第 15 个，UsageLog）。`internal/model/` 包已删除（类型迁入 `domain`）。

另有按「基础设施端口」归组的 `port/cache`（RPM/Session/Timeout/LeaderLock/Update/UserMsgQueue/ContentModeration/Internal500/OpenAI403/GeminiToken/Email/RefreshToken/Identity/Totp 等 Redis 缓存契约 + 伴生数据类型）与 `port/oauthclient`（OpenAI/Grok/Claude/Gemini/GeminiCliCodeAssist 五个 OAuth 客户端契约）——它们是跨 BC 的纯 stdlib/`pkg/*` 接口，不属于单一 BC，故按用途归组而非塞进某个 BC 端口包。

**Account BC 备注**（`dd50614`）：
- `domain.Account` + ~150 个纯方法；`domain.AccountGroup` 随 Account 一起迁入（嵌套 `*Account`/`*Group`，不可单独先搬）。
- `port/account` 仓储接口保持原方法集（ISP 拆分留作后续；67 个测试桩靠 type alias 继续满足）。
- 2 个 impure 方法 + 11 个 ctx 级联方法改为 service 自由函数（`AccountSupportsOpenAIEndpointCapability`、`AccountIsSchedulableForModelWithContext` 等），避免把 request-metadata / metrics / gateway helper 拉进 domain。
- `repository/account_repo.go` 实现 port 并返回 `domain.Account`，但仍 import `internal/service`（SchedulerCache、UpstreamBillingProbe*、并发/容量/Grok 凭证等 16 个非 Account 符号，属其他 BC）。

**UsageLog BC 备注**（`5f0bcc9`，Account 解锁后）：
- `domain.UsageLog` + 3 纯方法（`TotalTokens` / `EffectiveRequestType` / `SyncRequestTypeAndLegacyFields`）；嵌套 `*User/*APIKey/*Account/*Group/*UserSubscription` 均已是 domain 类型。
- `port/usagelog.Repository` 38 方法原样；17 个测试桩靠 service alias 继续绿。
- repo：`usage_log_repo{,_query,_dashboard}.go` **已 drop** service import；`_insert` 保留（`MarkUsageLogCreate*`）、`_stats` 保留（`GeminiUsageTotals`）。
- 反转 KPI：**72 → 69**（UsageLog 三文件 drop）。

**提取一个新 BC 的固定步骤**（参考 announcement pilot 与 Account keystone）：
1. 实体 + 错误 + 纯函数 → `internal/domain/<bc>.go`
2. 仓储接口 → `internal/port/<bc>/<bc>.go`（签名用 `domain.*`；大接口可先原样搬、ISP 后拆）
3. 改写 `repository/<bc>_repo.go`：实现 port、返回 domain；能去掉 service import 就去掉，不能则保留并注明剩余符号所属 BC
4. service 改为 type alias + 错误再导出；impure 方法改自由函数
5. 验证：`go build ./...` + `go vet` + 该 BC 测试（含相关 stub）；可选检查 `rg -l 'internal/service' backend/internal/repository --type go -g '!*_test.go' | wc -l`

**注意**：跨 BC 实体依赖会阻塞提取。`Account` 与 `UsageLog` 均已在 domain。后续可优先拆 billing/dashboard 读模型、或 account_repo 残留的 16 个非 Account service 符号所属 BC。先拆被依赖的实体，再拆下游。

**当前反转 KPI**（`rg -l 'internal/service"' backend/internal/repository --type go -g '!*_test.go' | wc -l`）：**32**。本会话从 69 降至此：account_repo domain 换包、RoleAdmin、port/cache + port/oauthclient 两批基础设施端口，以及两个成体 BC——
- **Ops/dashboard 读模型 BC**：`domain/ops.go`（~47 个读模型类型）+ `port/ops`（OpsRepository 38 方法 + OpsIngressRejectRepository）+ `port/dashboard`（DashboardStatsCache + DashboardAggregationRepository）；13 个 ops/dashboard repo 文件 drop service。
- **billing/pricing BC**：`domain/{billing_cache,usage_billing,upstream_billing_probe,user_platform_quota,subscription,scheduler_events}.go` + `port/billing`（BillingCache + PricingRemoteClient + UsageBillingRepository + UserPlatformQuotaRepository）；4 个 billing repo 文件 drop service，account_repo 残留同步缩减（SchedulerOutbox* / UpstreamBillingProbe* 已迁 domain）。

剩余 repo 文件多为未提取 BC 的实体/读模型（BatchImage、scheduler-outbox、gemini-quota、account_repo 残留的 Grok/Group/Scheduler 符号等）以及若干单接口阻塞（SecretEncryptor/ImageStorage/DBDumper，分属 auth/image/backup 等待提取 BC）——这些应随各自 BC 成体拆迁，而非提前单接口提取。

## 七、参考资源

- [上游仓库](https://github.com/Wei-Shaw/sub2api)
- [Ent 文档](https://entgo.io/docs/getting-started)
- [Vue3 文档](https://vuejs.org/)
- [pnpm 文档](https://pnpm.io/)
