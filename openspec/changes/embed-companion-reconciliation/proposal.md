# 把「经营对账」做成 Sub2API 自带功能

> 状态：**待主人确认 1 个汇率问题**（其余全部定稿）
> 日期：2026-09-29 ｜ 工作区：`.worktrees/dev`（分支 `dev`）
> 版本：v3 —— 按主人 2026-09-29 的新信息大幅简化（去掉 Subarx、去掉汇率换算）

---

## 〇、主人本轮给的 6 条信息（全部已纳入）

| # | 主人的话 | 对方案的影响 |
| --- | --- | --- |
| 1 | 上游**只有 A6**，Subarx 已取消 | **删掉整个 Subarx 子系统**（分摊算法、日级校验、倍率、3 张表） |
| 2 | 汇率统一了，记账按实际汇率 6.71，**不需要汇率换算了** | **删掉整个汇率子系统**（抓取、冻结、兜底链、1 张表）；改为**面板上一个可改的换算数字**（默认 6.71）+ 账单行上的历史快照 |
| 3 | 价格接口「先别删」，是**给第三方获取**的，模型价格先不用更新 | **保留**，搬进主程序，只修域名 |
| 4 | 旧代码目录 + 旧数据库**先归档** | 见第五节 |
| 5 | 新账本**从切换那一刻**开始记账 | 确认原方案 A |
| 6 | 账号规则**我自己在对账面板里调**，你让它能调就行 | **不预置规则**，保证面板可编辑即可（现有面板已支持） |

---

## 一、一句话目标

把「经营对账」从旁边那个单独的 companion 容器搬进 Sub2API 主程序，变成主程序自带的功能。
数据存在**同一个 PostgreSQL**（新建独立表，不动任何现有表），后台「经营对账」页面照旧，从此**只需要一个容器**。

---

## 二、主人的 6 条信息砍掉了多少工作量

这轮信息让方案**从 9 张表减到 5 张**，还砍掉两套定时采集器：

| 原计划 | 现在 | 原因 |
| --- | --- | --- |
| Subarx 采集器 + 分摊算法 + 日级一致性校验 | **全部删除** | 上游只剩 A6 |
| `reconciliation_subarx_daily_usage` | **删除** | 同上 |
| `reconciliation_subarx_scope_status` | **删除** | 同上 |
| `reconciliation_subarx_billing_snapshots` | **删除** | 同上 |
| 汇率采集器（Coinbase / er-api 双源） | **全部删除** | 汇率已统一 |
| `reconciliation_fx_rates` | **删除** | 同上 |
| 汇率冻结 / 陈旧判定 / 四级兜底链 | **全部删除** | 同上 |
| 「汇率数字从哪来」 | 改为**面板上一个可改的输入框**（默认 6.71），A6 账单导入时把这个数字**抄一份到账单行上** | 不抓取、不联网、不判定陈旧；但历史仍然冻结 |
| 账号规则支持两种 provider | **只保留 A6** | 同上 |
| `cost_source` 11 种状态 | **减到 6 种** | 去掉 5 种 Subarx 状态 |

**保留下来的核心复杂度只有一件事：把 A6 的逐笔账单和我们自己的调用一笔笔对上。** 这也正是对账真正的价值所在。

---

## 三、保留下来的 6 种对账状态

| 状态 | 中文 | 含义 |
| --- | --- | --- |
| `billed` | 账单实扣 | 已拿到 A6 真实账单并匹配上 → **已对账** |
| `a6_waiting` | 等待上游账单 | 这笔调用的 A6 账单还没拉回来 |
| `a6_pending` | 上游账单待匹配 | 账单拉回来了，但没匹配到具体是哪笔调用 |
| `upstream_unmatched` | 上游待匹配 | 账单在，但确认匹配不上（孤儿账单） |
| `rule_unconfigured` | 规则待配置 | 这个账号还没配 A6 令牌，无法对账 |
| `pending` | 待对账 | 兜底状态 |

（原方案里 `subarx_billed_allocation` / `subarx_pending` / `subarx_waiting` / `subarx_rule` / `subarx_unallocated` 五种全部废弃。前端对这几种的文案会自然落空到我们给的 `cost_source_label`，**不需要改前端**。）

### ⚠️ 旧实现有一个已知 Bug，新实现必须修掉

历史排查记录（mem0 `bb26134f`）确认：旧代码在 `profitRowsPage` 里写了

```go
else if snapshot.Provider != "subarx" { row.CostSource = "rule_unconfigured" }
```

**把「已配好 A6 规则、只是上游账单还没到」的调用，错误地标成了「规则待配置」**，导致页面上出现大量误导性的红色标签（实测一次影响 1768 条）。

正确判定：

- **只有**当规则快照的 provider **为空** 且该账号当前也**没有**规则时 → 才显示「规则待配置」
- 已配置 A6 但账单未到/未匹配 → 应显示「等待上游账单」或「上游账单待匹配」

**这是新实现必须刻意修掉的**，不能照搬旧逻辑。

### ⚠️ 旧实现第二个已知 Bug：令牌改名后历史账单变孤儿

历史排查记录（mem0 `0752f301`）确认：匹配算法**只读当前**的 令牌↔账号 映射表，**完全不读**调用发生时记录的规则快照 key。于是管理员把令牌从 `0.12` 改名成 `0.15-claude` 之后，**所有旧账单立刻变成孤儿**，无法回配（实测一次影响 1157 条）。

正确做法：**未对账的记录，必须用它自己的规则快照 key 去做历史匹配**；已对账的记录保持不变。

**这两个 Bug 是新实现相对旧实现的实质改进，也是「为什么值得重写」的最好理由。**

---

## 四、要做的四件事

### 第 1 件：建账本

主程序本就记录了每次调用（`usage_logs`：谁、哪个账号、哪个模型、多少 token、收了多少钱）。这件事：

- 新建对账专用的表（不动现有表）
- 把调用接进账本
- **额外记下"当时这个账号归哪个 A6 令牌管"** —— 因为令牌↔账号的对应关系以后会改，历史必须按当时的对应算，不能事后重算

**做完这件，管理员能看到「下游收入」和调用明细。**

### 第 2 件：接 A6 账单，一笔笔对上

- 定时去 A6 拉每一笔的真实扣费（A6 系统令牌鉴权，分页拉取）
- 把 A6 账单和我们自己的调用**一笔笔对上**（匹配算法见 6.6）
- 提供**历史回填**：可以从任意时间段补拉 A6 账单

**做完这件，就能算出真实毛利。**

### 第 3 件：账号规则（主人自己调）

页面上列出**所有账号**（不只最近有调用的），每个账号可以选：
- provider：`A6`
- 值：A6 的**令牌名**（token_name）

保存后立即生效。**不预置任何规则，全部由主人在面板上手工配置。**

⚠️ 需要保证：**账号没有任何调用记录时也能出现在列表里并配置**（旧实现是列出全部未删除账号 + 它的用量统计，这一点要继承）。

### 第 4 件：公开价格接口 + 下线旧容器

- `GET /api/provider/pricing` 搬进主程序（**内容与价格一字不改**，只把 `site_domain` 从 `xzgc.asia` 改成 `chenshuapi.com`）
- 删除 `/ops/profit` HTML 看板
- 归档旧代码目录、归档旧数据库文件、停用 companion 容器
- 更新交接文档与拓扑图

---

## 五、归档方案（主人第 4 条）

### 5.1 旧代码目录 —— 注意它是 **git-tracked** 的

`extensions/sub2api-companion/` **有 26 个文件被 git 跟踪**，不是忽略目录。所以：

- **方案 A（推荐）**：移到 `extensions/archive/sub2api-companion/`。git 会识别为「重命名」，**历史一条不丢**，同时目录名明确表达"已归档、勿改"。
- 方案 B：直接删除。git 历史本身就是最完整的归档，需要时 `git show <旧提交>:路径` 就能取回。
- 配套：在归档目录的 `README.md` 顶部加一段「本服务已收编进主程序，此处仅作历史参考，不再维护」。

**我的建议：方案 A**（主人说"归档"，那就留个可见的存档位置，比只剩 git 历史更符合"归档"的直觉）。

### 5.2 旧数据库文件 —— 本地没有，在生产 VPS 上

我查过了：**本工作区没有任何 `.db` / `.sqlite` 文件**。生产的 SQLite 在 VPS 上：
```
/opt/sub2api/extensions/sub2api-companion/data/companion.db
```

**归档 = 运维动作**（我在本工作区无法执行，也不该执行）：
1. 停掉 companion 容器
2. 把整个 `data/` 目录打包成 `companion-db-backup-20260929.tar.gz`
3. 留存到安全位置（VPS 本地 + 建议再拉一份到本地）
4. 确认备份可解压、文件大小正常，再动容器

⚠️ 主人的决定是**不迁数据**，所以这份备份纯粹是"万一要回滚"的保险，**不需要导入任何数据**。

---

## 六、技术实施细则

> 这一节是给写代码的人（和后续会话）看的，主人可以跳过。

### 6.1 为什么前端可以一行不改

前端 `CompanionView.vue`（780 行）+ `api/admin/companion.ts`（508 行）调的一直是**主程序自己**的 `/api/v1/admin/companion/*`，由 `backend/internal/handler/admin/companion.go` 反向代理到 companion 的 `/ops/api/*`。

**只把 `CompanionHandler` 从「HTTP 反代」改成「调用进程内 service」，11 个路由的路径/方法/请求体/响应结构全部不变。**

```
现状:  Vue → /api/v1/admin/companion/* → [反代] → companion:8090/ops/api/*
目标:  Vue → /api/v1/admin/companion/* → [进程内 service] → PostgreSQL
              ↑ 这一整段原封不动                  ↑ 只换这里
```

### 6.2 前端契约硬约束（违一条就白屏或静默出错）

1. **所有 GET 会被前端拦截器无条件注入 `?timezone=Asia/Shanghai`** → 新 handler **绝不能严格校验 query**，否则直接 400。
2. **错误分类只认 `message` 子串**：`classifyCompanionError` 按 `COMPANION_NOT_CONFIGURED` / `COMPANION_UNREACHABLE` / `COMPANION_AUTH_FAILED` / `COMPANION_INVALID_RESPONSE` / `COMPANION_BAD_REQUEST` / `COMPANION_UPSTREAM_` 前缀顺序匹配，**数字 `code` 完全不参与分类**。
   ⚠️ 与官方规范冲突（官方要求用 `reason`）。**两全**：`reason` 用官方 UPPER_SNAKE，**同时**在 `message` 里保留 `COMPANION_*` 前缀。
3. **6 个计数字段必须是 JSON number**（`downstream_matched` / `downstream_unmatched` / `upstream_unmatched` / `record_total` / `billed_count` / `calculated_count`）→ 字符串会让卡片**静默**显示「—」，无报错、无日志。
4. **`margin_percent` 不能是 null** → 会渲染出 `undefined%`。
5. **绝不返回 401/403** → 前端遇 401 会清登录态跳 `/login`。鉴权类失败必须映射成 502/503 + `COMPANION_*` 标记。
6. **`collect` 必须「触发即返回」**：前端 HTTP 超时 30s，而 A6 超时 90s，同步跑完整采集**必然超时**。改为触发后台任务立即返回。
7. **`GET /status` 的 `enabled:false` 是唯一会让整页不加载业务数据的开关** → 收编后恒为 `true`。
8. **明细行 key = `record_type:source_id:request_id:upstream_request_id` 必须唯一**（下游行与上游待匹配行共用 `source_id=0` 是已知情况）。
9. `timeseries.points` 的 **`start` 必须能被 `new Date()` 解析且严格升序**（图表用前两个点的时间差推断桶宽）。
10. `cost_source_label` **必须非空**（它是未知 `cost_source` 枚举值的回落文案）。
11. **`upstream_cost` / `gross_profit` 在未对账时必须给空字符串 `""` 而不是 `0`**（前端据此显示「—」）。
12. **账号规则列表不得为空**：主人要靠它配置规则，所以必须列出全部账号（含零调用账号）。

### 6.3 后端规范硬约束（违反会被 CI / 守卫测试拦）

1. **迁移唯一权威** = `migrations/NNN_desc.sql`，**没有 ent auto-migrate**。下一个号 **242**。前向不可变 + SHA256 checksum 记账。
2. **handler / service 禁止 import `internal/repository`、`gorm.io/gorm`、`redis/go-redis/v9`**（depguard 硬拦）。
3. **端口 interface 声明在 service 包**，repository 构造函数**返回 service 侧接口**。
4. **ent 与 `wire_gen.go` 禁止手改**，改完必须 `go generate ./ent` + `go generate ./cmd/server`。
   ⚠️ **上一轮 Companion 集成手改了 `wire_gen.go`，这是违规**，本次必须用生成覆盖回来。
5. **新增 `Config` 字段必须同时加 `viper.SetDefault`**，否则 `TestConfigKeysAreEnvReachable` 直接红。
6. **后台循环必须成对**：`Provide*` 里 `Start()`，`cmd/server/wire.go` 的 `provideCleanup` 里 `Stop()`。新增参数必须同步 `cmd/server/wire_gen_test.go`。
7. **`internal/server/api_contract_test.go`（2962 行）表驱动比对完整 JSON 字符串** → 改动已有 admin 接口响应字段会被它拦。
8. **测试带 build tag**：`go test ./...` 在本仓库≈空跑。必须 `go test -tags=unit ./...` / `-tags=integration ./internal/repository/`。
9. **审计日志全自动**：挂在 `/api/v1/admin` 组上，新 handler 默认零改动就有记录。
10. `gofmt` rewrite：`interface{}` → `any`；`errcheck` 开了 `check-type-assertions: true`。
11. 注释用**中文**；错误 reason 用 UPPER_SNAKE；日志用 `logger.LegacyPrintf("service.<name>", "snake_case_event: err=%v", err)`；handler 上方两行注释（方法说明 + `METHOD /path`）。
12. 事务：`ent.NewTxContext(ctx, tx)` + `error_translate.go` 的 `clientFromContext` 让 repo 自动参与同一事务。

### 6.4 新建的表（5 张，全部 `reconciliation_` 前缀）

**关键设计改进：不再需要整份复制调用记录。**

旧 companion 之所以要把每次调用复制一份到自己的 `usage_snapshots` 表，是因为它是**独立进程**，只能只读地远远看着主库。现在我们就住在同一个数据库里，**可以直接查 `usage_logs` 原表**。只需额外记主库没有的东西：

| 表 | 装什么 |
| --- | --- |
| `reconciliation_usage_extras` | 每次调用的**令牌对应关系快照**（薄表，按 usage_log id）。记录"这笔调用发生时，该账号归哪个 A6 令牌管"，保证改规则不重算历史 |
| `reconciliation_upstream_bills` | A6 逐笔账单 + 人工导入的账单；含匹配结果（匹配到哪笔调用、用什么方法匹配的） |
| `reconciliation_account_rules` | 账号规则：账号 ID ↔ A6 令牌名，带版本号与启用开关 |
| `reconciliation_request_maps` | 请求 ID 关联（我们这边 ID ↔ 上游 ID），用于"直连匹配"（见 6.7 设计问题） |
| `reconciliation_sync_state` | A6 同步游标与状态（拉到哪了、每个令牌的首次同步是否完成、上次成功/失败时间与错误）、历史回填任务进度 |

> 原方案里的 `reconciliation_fx_rates` 与 3 张 Subarx 表已按主人信息删除。

### 6.5 需要动的文件

**新增**

| 文件 | 说明 |
| --- | --- |
| `backend/ent/schema/reconciliation_*.go` | 5 张新表的实体定义 |
| `backend/migrations/242_reconciliation_*.sql` | 建表 DDL |
| `backend/internal/service/reconciliation_*.go` | 端口 interface + service（账本 / A6 同步 / 规则 / 报表） |
| `backend/internal/repository/reconciliation_*.go` | ent 实现 |
| `backend/internal/handler/admin/reconciliation_*.go` | 业务 handler |
| `backend/internal/handler/reconciliation_*_test.go` | `//go:build unit` |
| `backend/internal/repository/reconciliation_*_integration_test.go` | `//go:build integration` |

**改动（一处都不能漏）**

| 文件 | 改动 |
| --- | --- |
| `backend/internal/handler/admin/companion.go` | 反代 → service 委托；删掉 `os.Getenv` |
| `backend/internal/handler/handler.go` | `AdminHandlers` 结构体字段 |
| `backend/internal/handler/wire.go` | ProviderSet 注册 + `ProvideAdminHandlers(...)` 参数表末尾追加 |
| `backend/internal/repository/wire.go` | ProviderSet 注册 |
| `backend/internal/service/wire.go` | ProviderSet 注册 + `Provide*` 启动包装 |
| `backend/cmd/server/wire.go` | `provideCleanup` 参数表 + `parallelSteps` 加一段 `Stop()` |
| `backend/cmd/server/wire_gen_test.go` | **必须同步**（漏改直接编译失败） |
| `backend/internal/server/routes/admin.go` | `registerCompanionRoutes` 路径不变；新增公开价格路由 |
| `backend/internal/config/config.go` | 新配置字段 + `viper.SetDefault`（A6 地址/令牌/用户 ID/超时等） |
| `backend/internal/server/client_request_id.go` | 视 6.7 的设计结论决定是否落 `request_maps` |
| `extensions/sub2api-companion/` | 归档移动 + README 标注 |

**生成物（禁止手改）**：`backend/ent/**`、`backend/cmd/server/wire_gen.go` → 跑生成命令覆盖。

### 6.6 必须逐字保住的业务语义

1. **A6 成本换算**：`costUSD = quota / quota_per_unit`（`quota_per_unit` 来自 A6 的 `GET /api/status`）。
2. **A6 鉴权头**：`Authorization: Bearer <A6_ACCESS_TOKEN>` + `New-API-User: <A6_USER_ID>`，另带 `Cache-Control: no-store`。响应体限 16MB、用 `UseNumber()` 解析。
3. **A6 分页参数**：`GET /api/log/self?p=&page_size=&type=2&token_name=&model_name=&start_timestamp=&end_timestamp=`（时间戳为 Unix 秒，`end = now + 1分钟`）。
4. **`other` 字段可能是被双重编码的 JSON 字符串**，解析要兼容对象与字符串两种形态。
5. **上游账单匹配四级降级**（顺序不能改）：
   - ① 直连 Request ID 匹配 → `direct_request_id`
   - ② 组合匹配：模型 + 输出 token + 缓存 token + 输入 token（两种等价写法）+ 时间窗 ±2 分钟，**要求唯一**，并列最小值时**拒绝认定**（宁可不对账也不错配）
   - ③ 放宽缓存 token 到「只比 cache_read」
   - ④ 再放宽到「缓存 token 差 1 以内」且时间窗收紧到 ±2 秒
6. **暂存态**：新导入的账单在完成首次匹配前处于内部暂存状态，**不与下游调用在看板里分裂成两行**；确认匹配不上后才显示为「上游待匹配」。
7. **`cost_source` 状态机的分支顺序**（默认 `pending`）—— 顺序决定结果，**不能重排**。
8. **统计口径与明细列表必须同源**：旧实现的计数走 SQL 条件、明细走 Go 分支，是手工镜像，容易不一致。**新实现让两者共用同一份判定逻辑**（重写应顺手修掉的隐患）。
9. **A6 历史回填的块级游标语义**：整块成功后**才**推进游标（所以块内进度数字不变是正常的，不是卡死）；重启后从持久化游标续跑。
10. **A6 单页重试**：单页 3 次重试，退避 0.5s / 1.0s。
11. **A6 令牌映射支持"一个令牌对多个账号"**（旧实现特意删掉了唯一索引来允许这种情况）。
12. **令牌改名后的历史补偿**：从历史快照里收集旧令牌名，用一个独立的、较短的回头窗口单独补拉（默认 48 小时），不从全量历史第一页重新扫。
13. **单个令牌失败要隔离**：记下具体是哪个令牌失败并继续处理其他令牌；同步失败的标记只表示"本轮有部分失败"，不代表其他令牌没采到。
14. **两种"未对账"方向完全相反，绝不能强行互相匹配**（mem0 `020e2005` / `974ed8d5` 的核心结论）：
    - **下游待对账** = 本站有调用，但缺上游成本
    - **上游待匹配** = 共享上游账单在，但缺本站调用
    - 共享同一个 A6 令牌时，上游账单数量天然可能**少于**本站调用数（别的站点也在用同一令牌），所以「同令牌有账单」**不等于**「该账单属于这条请求」。**绝不能为了消灭"未对账"而复制/摊派上游成本。**
15. **缓存 token 差 1 的窄兜底有严格门禁**（mem0 `10eefec6`）：实测 A6 账单 `cache_tokens=62` 而本站 `cache_read+cache_creation=63`，旧算法要求缓存精确相等，导致同一次调用被拆成两行。修法是**仅当精确候选为 0 时**才放宽，且必须同时满足：缓存**恰好相差 1** + 时间差 **≤2 秒** + 候选数**严格等于 1** + 该 usage **未被占用** + 缓存 **>0**。**不能直接把窗口放宽到 2 分钟。**
16. **历史账单窗口的客观边界**（mem0 `e693f57e`）：A6 接口能查到的**最早账单有时间下限**，早于本站首次上线时刻的真实账单**不会自动补齐**；另外汇总超过 10 万条会被截断。页面上应能看出来这个边界，不能把"历史查不到"误判成"对账失败"。
17. **A6 鉴权优先用「系统访问令牌」**（mem0 `279f399b`），不要长期保存网页登录密码。

### 6.7 一个待定的设计问题（不阻塞，实施前定即可）

`usage_logs` 表**没有上游请求 ID**（只有 `request_id`、`upstream_model`、`channel_id`）。所以「① 直连 Request ID 匹配」需要额外的映射来源。两个选项：

- **选项 A**：在请求链路上把「我们这边 ID ↔ 上游 ID」写进 `reconciliation_request_maps`（新增一张表；需要在 `client_request_id.go` 附近加一处落库）。
- **选项 B**：只做组合匹配（②③④），不做直连匹配。实现更简单，但**匹配率会下降**，部分本来能对上的账会落到「待匹配」。

**我倾向选项 A**（对账准确率是核心价值），但会在 `design.md` 里给出两种方案的匹配率对比后再定。

### 6.8 架构净收益：彻底删掉 Caddy 日志依赖

旧 companion 要解析 Caddy 的 JSON 日志，把 `X-Client-Request-Id → X-Request-Id` 的映射写进 `request_maps`。**但 Sub2API 自己就在请求链路上**，直接在中间件里落库即可。于是：

- ① **删掉 `../../caddy_logs:/var/log/caddy:ro` 只读卷依赖**
- ② 顺带消灭一个既有缺陷：`bufio.Scanner` 读到无换行的半行 JSON 会解析失败丢弃，而字节偏移已推进到文件末尾 → **该请求的映射永久丢失**。改为中间件落库后不可能发生。

> 这条与 6.7 选项 A 是同一处改动，可一并完成。

### 6.9 采集服务要照抄的官方范式

照 `internal/service/upstream_billing_probe.go`（1179 行，仓库里最新的权威样板）：

- `ProvideReconciliationCollectorService(...)`：函数内 `svc.SetLeaderLock(...)` + `svc.Start()`，返回 `*Service`
- `Start()` / `Stop()` 幂等（`mu` + `started` / `stopped` 标志）；`Stop()` 走 `parentCancel()` + `wg.Wait()`
- `runLoop()`：`time.Ticker` + `select { case <-ctx.Done(): return; case <-ticker.C: ... }`
- 多实例去重：`LeaderLockCache.TryAcquireLeaderLock`（Redis 锁 → pg advisory lock → 单实例三级退化）
- 在 `cmd/server/wire.go` 的 `provideCleanup` 注册 `Stop()`

### 6.10 采集器的既有缺陷（重写时顺手修）

1. **`collect()` 全程持大锁且含网络调用**（A6 90s），叠加 SQLite 单连接 → 极端情况看板查询撞锁。新实现换 PG 连接池 + 分阶段短事务。
2. **完全没有优雅退出**（无 `signal` / `context` / `WaitGroup`）。新实现按 6.9 范式。
3. **A6 与汇率配置耦合**：旧实现里汇率相关配置只在 `A6_ACCESS_TOKEN` 非空时解析。既然汇率已删，这个耦合自然消失。
4. **`usage_logs` 时间列比较**：旧 companion 把时间存成 TEXT 靠字典序比较，存在「同一秒内小数位数不同则不严格正确」的隐患（`...T00:00:00Z` 会排在 `...T00:00:00.5Z` **之后**）。新实现直接查 `usage_logs` 的原生 `timestamptz`，**这个隐患自动消失**。

### 6.11 验证链（交付前必须全绿）

```powershell
# backend/
gofmt -l .                                   # 必须无输出
go build ./...
go test -tags=unit ./...
go test -tags=integration ./internal/repository/
# frontend/
pnpm run check:i18n
pnpm exec vue-tsc --noEmit                   # 注意：不是 vue-tsc -b
pnpm run build
```

---

## 七、分阶段实施计划

| 阶段 | 内容 | 验收 |
| --- | --- | --- |
| **1. 建表 + 账本** | 迁移 242 + 5 张表 + service + `/summary` `/requests` `/timeseries` 走新实现 | 管理页 8 个卡片、趋势图、明细表数字正确 |
| **2. A6 同步与匹配** | A6 采集器（含历史回填）+ 四级匹配 + `cost_source` 状态机 + 账号规则 CRUD | 手动「立即同步」后数字与状态符合预期；**账号规则能列出全部账号并保存生效** |
| **3. 请求 ID 关联** | 按 6.7 结论落地；删掉 Caddy 日志依赖 | 匹配率不低于旧实现 |
| **4. 切接口 + 归档 + 下线** | 价格接口搬进主程序（改域名）、删 HTML 看板、归档旧代码、停容器 | `curl /api/provider/pricing` 与切换前逐字节等价（除 `updated_at`）；4 个容器健康 |

---

## 八、Non-goals（本方案不做）

- **不改前端**（面板已可用，只换发动机；Subarx 那几种状态自然落空到 `cost_source_label`）
- **不迁移任何历史数据**
- **不修改任何现有官方表**（新表全部 `reconciliation_` 前缀）
- **不实现 Subarx 相关任何功能**（上游已取消）
- **不实现汇率换算 / 抓取 / 冻结**（已统一，待主人确认）
- **不更新价格表的模型与价格**（主人明确说先不维护）
- **不合并 Ops 子系统与对账模块**（Ops 是平台用量指标，对账是金额对账，口径不同）
- **不在本工作区操作生产**（生产归档、Caddy 改路由留给运维会话）

---

## 九、汇率：我改成了「不用你回答也能对」的设计

我原本要问主人一个问题，后来发现**可以设计成无论哪种理解都是对的**，所以不问了。

### 我查到的代码证据

- `api_key` 表的 `quota_used` 字段注释是 **「Used quota amount in USD」**
- `batch_image_job` 表的币种默认值是 `USD`
- 历史生产实测数据（2026-07 记录）：下游收 **¥0.00004338**，A6 原币 **$0.00002400**，折算后 **¥0.00017280**，毛利 **−¥0.00012942**
  → 那次的换算是 `$0.000024 × 7.2 = ¥0.0001728`，所以**历史上汇率是 7.2**，而主人现在说统一成 **6.71**

### 我的设计（不需要主人回答任何问题）

> **把「A6 美元 → 人民币」的换算做成面板上的一个输入框，默认 6.71。**
> **A6 账单导入时，把这个数字抄一份到那条账单上。**

这样一来：

| 情况 | 结果 |
| --- | --- |
| 如果确实需要换算 | 填 6.71（或任何当时的真实汇率），照常算 |
| 如果两边币种其实已经一致、压根不用换算 | 把那个框填 **1**，就等价于不换算 |
| 汇率以后又变了 | 改框里的数字。**已导入的历史账单不会被改动**（它们各自抄了当时的数字），历史永久冻结 |
| 主人想把两个币种的展示口径调一调 | 改框里数字即可，不需要改代码、不需要重新部署 |

**这比旧实现好在哪：** 旧实现要每小时联网抓 Coinbase 和 er-api 两个汇率源、要判定数据是否陈旧、要有四级兜底链、还要建一张专门的汇率表 —— 而这一切的**唯一目的**只是得到"一个数字"。现在这个数字由主人直接给，**整套机器全删，历史冻结能力一分不减**。

**唯一请主人留意的**：上线后打开面板的对账设置，看看那个框里的数字是不是 6.71。如果实际的换算关系不是这个数，改成正确的即可。

---

## 九之二、还需要主人确认的（只剩 0 个）

没有了。**方案已可开工。**（账号规则主人说自己调，价格表主人说先不动，历史数据主人说不迁 —— 全部闭环。）

---

## 十、待补充产物

方向确认后补齐（按本仓库 OpenSpec 先例格式）：

- `design.md` — 5 张表的字段设计、service 拆分、A6 采集器时序、匹配算法细节、6.7 两个选项的匹配率对比
- `tasks.md` — 编号任务清单（精确到文件与函数）
- `specs/<capability>/spec.md` — 能力行为规格
- `verification.md` — 验证计划与证据
