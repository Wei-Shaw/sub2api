# 设计文档：经营对账收编进 Sub2API 主进程

> 对应提案：`proposal.md` ｜ 日期：2026-09-29 ｜ 迁移编号：**242**

> ⚠️ **本文是设计阶段的推演稿，不是最终实现的说明书。** 实施过程中有若干决策被推翻或收敛，
> **权威记录是 `tasks.md` 的「实施偏差」一节与代码本身**。已知本文与代码不一致之处：
>
> | 本文的写法 | 代码的实际做法 |
> |---|---|
> | `reconciliation.enabled` 默认 `false` | 默认 **`true`**（`viper.SetDefault`） |
> | 嵌套配置键 `reconciliation.a6.base_url` | **扁平键** `reconciliation.a6_base_url`（以 `mapstructure` 标签为准） |
> | 汇率覆盖写 `settings` 表 | 写 `reconciliation_sync_state` 的 `fx_usd_cny_rate_override` 键 |
> | `reconciliation_usage_extras` 主键是 `usage_log_id` | `id BIGSERIAL PRIMARY KEY` + `usage_log_id` 唯一索引 |
> | 4 张表之外还有第 5 张 | 实际就是 **4 张**（`request_maps` 因 1.1 的发现被取消） |
> | `reconciliation.historical_token_lookback_hours` | **该配置项已被删除**（装配了但无人消费，属死配置） |
>
> 另外：`counterparty`/`Subarx` 相关设计已整体作废（上游只剩 A6）。

---

## 1. 三个决定性发现（直接改变了设计）

### 1.1 `usage_logs.upstream_request_id` 已存在 → 不需要 `request_maps` 表

`backend/migrations/232_add_usage_log_upstream_request_id.sql`：

```sql
-- usage_logs.upstream_request_id 记录直接上游在响应头中声明的请求标识，
-- 头名由账户 extra.upstream_request_id_header 指定，未指定时按默认识别链取值。
ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(128);
```

`backend/migrations/233_add_usage_log_upstream_request_id_index_notx.sql` 建了部分索引：

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_upstream_request_id
    ON usage_logs (upstream_request_id)
    WHERE upstream_request_id IS NOT NULL;
```

**结论**：旧 Companion 用「解析 Caddy JSON 日志」来构建 `client_request_id → upstream_request_id` 映射的努力，是在解决一个**主程序已经原生解决**的问题。新实现直接读这一列即可。

**连带删除**：
- ❌ `reconciliation_request_maps` 表（原计划第 4 张表）
- ❌ `internal/server/client_request_id.go` 的改动
- ❌ Caddy 日志只读卷依赖（`../../caddy_logs:/var/log/caddy:ro`）

### 1.2 ⚠️ 前置条件：`upstream_request_id` 默认是空的

`backend/internal/service/upstream_request_id.go:13-14, 31-40`：

```go
// AccountExtraUpstreamRequestIDHeader 是账户 extra 中的键，值为直接上游声明请求标识的响应头名。
// 未指定时不记录上游请求标识。
const AccountExtraUpstreamRequestIDHeader = "upstream_request_id_header"
...
// UpstreamRequestIDFromHeaders 从直接上游的响应头解析请求标识。
// 只读账户指定的头；账户未指定头名时恒为空串。
func UpstreamRequestIDFromHeaders(account *Account, h http.Header) string {
	if len(h) == 0 { return "" }
	name := UpstreamRequestIDHeaderName(account)
	if name == "" { return "" }
	return strings.TrimSpace(h.Get(name))
}
```

**所以直连匹配（Level 1）能否生效，取决于每个 A6 账号有没有配 `extra.upstream_request_id_header`。**

- 配了 → Level 1 直连匹配命中，准确率最高
- 没配 → 该列为 NULL，自动落到 Level 2/3/4 组合匹配，**功能不缺失、只是匹配率略低**

**运维前置动作**（写入交付文档）：给 A6 账号补 `extra.upstream_request_id_header`，值取 A6 实际回传的请求标识响应头名。**这是配置动作，不是代码改动。**

### 1.3 旧实现对账有两个已确认的线上 Bug（新实现必须修）

| Bug | 旧代码 | 线上影响 | 新实现 |
| --- | --- | --- | --- |
| 状态误标 | `profitRowsPage` 里 `else if snapshot.Provider != "subarx" { row.CostSource = "rule_unconfigured" }` | 一次误标 **1768 条** | 「规则待配置」**仅当**规则快照 provider 为空**且**当前无规则 |
| 历史孤儿 | 匹配只读**当前**令牌映射，不读调用时的规则快照 | 令牌改名后 **1157 条**账单变孤儿 | 未对账记录**必须用自身的规则快照 key** 做历史匹配 |

---

## 2. 表设计（4 张，全部 `reconciliation_` 前缀）

迁移文件：`backend/migrations/242_reconciliation_tables.sql`

> 全部新建，**不修改任何现有表**。所有表带 `created_at` / `updated_at`（`mixins.TimeMixin{}`）。

### 2.1 `reconciliation_usage_extras` —— 调用侧扩展快照

主键 `usage_log_id`。**薄表**：只存主库没有、且必须冻结的东西。下游收入原值仍在 `usage_logs.actual_cost`，本表只冻结「汇率」与「当时的规则」。

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `usage_log_id` | bigint PK | 对应 `usage_logs.id`（不加外键约束，避免影响主表写入性能） |
| `account_id` | bigint NOT NULL | 冗余便于按账号聚合 |
| `rule_provider` | varchar(16) NOT NULL DEFAULT '' | `''` 或 `'a6'` |
| `rule_external_key` | varchar(128) NOT NULL DEFAULT '' | 采集时的 A6 令牌名（`token_name`） |
| `rule_version` | bigint NOT NULL DEFAULT 0 | 采集时的规则版本 |
| `revenue_original` | decimal(20,10) NOT NULL DEFAULT 0 | `usage_logs.actual_cost` 原值 |
| `fx_rate_to_cny` | decimal(20,10) NOT NULL DEFAULT 1 | **采集时的汇率快照** |
| `revenue_cny` | decimal(20,10) NOT NULL DEFAULT 0 | `revenue_original × fx_rate_to_cny`（冻结） |
| `collected_at` | timestamptz NOT NULL | 采集时间 |
| `created_at` / `updated_at` | timestamptz | TimeMixin |

索引：`(collected_at)`、`(account_id, collected_at)`、`(rule_external_key)`（供令牌改名后的历史回补查询）。

### 2.2 `reconciliation_upstream_bills` —— A6 逐笔账单

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint PK | |
| `provider` | varchar(16) NOT NULL DEFAULT 'a6' | 目前恒为 `a6` |
| `upstream_request_id` | varchar(128) NOT NULL | A6 账单的 `request_id` |
| `occurred_at` | timestamptz NOT NULL | A6 账单时间 |
| `billing_date` | date NULL | A6 若带日期则存，便于按日核对 |
| `model` | varchar(128) NOT NULL DEFAULT '' | |
| `token_name` | varchar(128) NOT NULL DEFAULT '' | A6 令牌名，用于关联账号 |
| `input_tokens` / `output_tokens` / `cache_read_tokens` / `cache_creation_tokens` | int NOT NULL DEFAULT 0 | 匹配依据 |
| `cost_original` | decimal(20,10) NOT NULL DEFAULT 0 | A6 原币金额（美元） |
| `currency` | varchar(8) NOT NULL DEFAULT 'USD' | |
| `fx_rate_to_cny` | decimal(20,10) NOT NULL DEFAULT 1 | **导入时的汇率快照** |
| `cost_cny` | decimal(20,10) NOT NULL DEFAULT 0 | `cost_original × fx_rate_to_cny`（冻结） |
| `source` | varchar(32) NOT NULL DEFAULT 'a6' | `a6` / `manual_import` |
| `match_state` | varchar(16) NOT NULL DEFAULT 'staging' | `staging` / `matched` / `unmatched` |
| `match_method` | varchar(48) NOT NULL DEFAULT '' | 见 §5.2 的匹配方法取值 |
| `matched_usage_log_id` | bigint NULL | 匹配到的调用 |
| `matched_account_id` | bigint NULL | |
| `raw` | jsonb NULL | 原始账单留存，便于追溯与重放 |
| `imported_at` | timestamptz NOT NULL | |
| `created_at` / `updated_at` | timestamptz | TimeMixin |

索引：
- **唯一** `(provider, upstream_request_id)` —— 幂等导入
- **唯一部分** `(matched_usage_log_id) WHERE matched_usage_log_id IS NOT NULL` —— **「一笔调用最多挂一笔上游账单」的硬保证**，必须保留
- `(occurred_at)`、`(token_name, occurred_at)`、`(match_state)`

### 2.3 `reconciliation_account_rules` —— 账号规则

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `id` | bigint PK | |
| `account_id` | bigint NOT NULL **UNIQUE** | 一个账号一条规则 |
| `provider` | varchar(16) NOT NULL DEFAULT 'a6' | 目前只支持 `a6` |
| `external_key` | varchar(128) NOT NULL DEFAULT '' | **A6 令牌名** |
| `multiplier` | decimal(20,10) NULL | 预留（Subarx 已取消，暂不使用） |
| `version` | bigint NOT NULL DEFAULT 1 | 每次保存 +1，供快照比对 |
| `enabled` | bool NOT NULL DEFAULT true | |
| `created_at` / `updated_at` | timestamptz | TimeMixin |

> ⚠️ **不加** `(provider, external_key)` 唯一索引 —— A6 令牌**允许多个账号共用**（旧实现特意删掉过该约束）。

### 2.4 `reconciliation_sync_state` —— 同步状态 KV

沿用官方 `settings` 表的形态（`key` 唯一 + `value` text + `updated_at`）。

| 列 | 类型 | 说明 |
| --- | --- | --- |
| `key` | varchar(100) PK | 见下表 |
| `value` | text NOT NULL DEFAULT '' | |
| `updated_at` | timestamptz | |

约定键：

| 键 | 含义 |
| --- | --- |
| `usage_last_collected_at` | 下游用量采集游标（RFC3339Nano UTC） |
| `a6_last_sync_unix` | A6 上次成功同步时间（Unix 秒） |
| `a6_last_sync_error` | A6 上次同步错误摘要 |
| `a6_last_sync_error_at` | 上述错误发生时间 |
| `a6_bootstrap_done:<token>` | 某 A6 令牌是否完成过首次同步（决定回看窗口） |
| `a6_backfill_status` / `_from` / `_to` / `_cursor` / `_processed` / `_error` | 历史回填任务进度（`status` 取 `running` / `completed` / `failed`，**与前端契约一致**） |

---

## 3. 下游收入口径（与官方一致）

沿用仓库既有口径，与 `dashboard_aggregation_repo.go:439`、`usage_log_repo_stats.go`、`usage_log_repo_trend.go` 完全一致：

```sql
-- 账号侧成本（官方口径，仅作参考展示，不参与对账）
COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)

-- 下游收入（用户实付）：取 actual_cost，再乘汇率冻结成 CNY
revenue_cny = usage_logs.actual_cost * fx_rate_to_cny
```

**对账口径 = 只实扣口径**：毛利只统计拿到 A6 真实账单的调用（`profit_scope = matched_only`）。不做估算、不做规则回算（`calculated_count` 恒为 0）。

---

## 4. 服务与文件拆分

```
handler 层（gin，禁 import repository）
  internal/handler/admin/reconciliation_handler.go      11 个管理端路由（替换 companion.go 的反代实现）
  internal/handler/provider_pricing_handler.go          公开价格接口（无鉴权）

service 层（端口 interface 声明在这里）
  internal/service/reconciliation_types.go             领域类型 + 端口 interface
  internal/service/reconciliation_ledger_service.go    账本：summary / timeseries / requests
  internal/service/reconciliation_rule_service.go      账号规则 CRUD + 账号列表视图
  internal/service/reconciliation_a6_client.go         A6 HTTP 客户端（鉴权/分页/重试/限流）
  internal/service/reconciliation_sync_service.go      用量增量采集 + A6 账单导入 + 匹配 + 回填
  internal/service/reconciliation_collector.go         周期调度（Provide + Start/Stop + leader lock）
  internal/service/provider_pricing_service.go         读取价格文档

repository 层（构造函数返回 service 侧接口）
  internal/repository/reconciliation_usage_extra_repo.go
  internal/repository/reconciliation_upstream_bill_repo.go
  internal/repository/reconciliation_account_rule_repo.go
  internal/repository/reconciliation_sync_state_repo.go

ent schema（4 个）
  internal/../ent/schema/reconciliation_usage_extra.go
  ent/schema/reconciliation_upstream_bill.go
  ent/schema/reconciliation_account_rule.go
  ent/schema/reconciliation_sync_state.go
```

---

## 5. A6 采集与匹配

### 5.1 A6 客户端

| 项 | 值 |
| --- | --- |
| 鉴权 | `Authorization: Bearer <A6_ACCESS_TOKEN>` + `New-API-User: <A6_USER_ID>` |
| 额外头 | `Cache-Control: no-store` |
| 账单接口 | `GET {base}/api/log/self?p=&page_size=&type=2&token_name=&model_name=&start_timestamp=&end_timestamp=` |
| 时间参数 | Unix **秒**；`end = now + 60`（吸收时钟偏差） |
| 响应上限 | 16 MB；用 `json.Decoder.UseNumber()` 避免浮点精度丢失 |
| 单页重试 | 3 次，退避 0.5s / 1.0s |
| 超时 | 90s |
| 成本换算 | `costUSD = quota / quota_per_unit`，`quota_per_unit` 来自 `GET {base}/api/status` |
| 兼容 | `other` 字段可能是**被双重编码的 JSON 字符串**，对象与字符串两种形态都要能解析 |
| 单令牌隔离 | 某个令牌失败时记下具体令牌并继续处理其他令牌；失败标记只表示「本轮有部分失败」 |

### 5.2 四级匹配算法（顺序不可改）

对每条 `match_state = staging` 的账单，按顺序尝试：

| 级别 | `match_method` | 条件 |
| --- | --- | --- |
| ① 直连 | `direct_request_id` | `usage_logs.upstream_request_id = bills.upstream_request_id`，且该 usage 未被占用 |
| ② 组合 | `composite_account_model_tokens_time` | 模型一致 + 输出 token 一致 + 缓存 token 一致 + 输入 token 一致（两种等价写法）+ 时间差 ≤ **2 分钟**；**候选必须唯一**，并列最小值时**拒绝认定** |
| ③ 放宽缓存 | `composite_account_model_tokens_time_cache_read` | 同上，但缓存 token 只比 `cache_read` |
| ④ 窄兜底 | `composite_account_model_tokens_time_cache_tolerance` | 缓存 token **恰好相差 1** + 时间差 ≤ **2 秒** + 候选数**严格 = 1** + 该 usage **未被占用** + 缓存 **> 0** |

候选账号范围 = 账单的 `token_name` 对应的账号（见 §5.3）。

**限制**：不复制、不摊派。若同一 A6 令牌被多个站点共用，账单数天然可能少于本站调用数 —— 「同令牌有账单」**不等于**「账单属于这条请求」。宁可留「上游待匹配」也不错配。

### 5.3 令牌 → 账号的映射（修 Bug 2）

**不能只读当前规则表。** 规则变更会产生三种情况：

1. 账单的 `token_name` 命中**当前**规则的账号 → 直接用
2. 未命中当前规则，但**历史** `reconciliation_usage_extras.rule_external_key` 里有 → 用这些历史账号（**这是旧实现缺失、导致令牌改名后 1157 条账单变孤儿的地方**）
3. 都没有 → 该账单保持 `unmatched`

另外：令牌改名后，对**历史快照里出现过的旧令牌名**，用一个**独立的、较短的回头窗口**（默认 48 小时）单独补拉，而不是从全量历史第一页重新扫。

### 5.4 采集时序

```
每 30s  ──► collectUsage()      扫 usage_logs 新行 → 写 reconciliation_usage_extras（含规则+汇率快照）
每 5min ──► syncA6Bills()       拉 A6 账单 → 幂等 upsert → 匹配 → 更新 match_state
手动    ──► POST /collect       触发一次完整采集，**立即返回**，后台跑（前端 30s 超时不能阻塞）
手动    ──► POST /a6/backfill   启动历史回填任务，立即返回进度对象
```

**并发安全**：多实例用 `LeaderLockCache`（Redis 锁 → pg advisory lock → 单实例三级退化），照抄 `internal/service/upstream_billing_probe.go`。

**退出**：`Provide*` 里 `Start()`；`cmd/server/wire.go` 的 `provideCleanup` 里 `Stop()`。`Stop()` 走 `parentCancel()` + `wg.Wait()`，**不再像旧实现那样没有优雅退出**。

**锁粒度**：不照搬旧实现「全程持一把大锁跨网络调用」的做法。改为：网络请求在锁外、DB 写入用短事务。

---

## 6. `cost_source` 状态机（6 种，含 Bug 1 修正）

判定顺序（顺序即优先级，默认 `pending`）：

### 6.1 下游调用行的状态（判定顺序即优先级）

| 顺序 | `cost_source` | 中文 | 条件 |
| --- | --- | --- | --- |
| 1 | `billed` | 账单实扣 | 该调用已匹配到 A6 账单 → **已对账** |
| 2 | `rule_unconfigured` | 规则待配置 | **规则快照 provider 为空 AND 该账号当前也无规则**（← Bug 1 修正点） |
| 3 | `a6_pending` | 上游账单待匹配 | 有规则，且对应令牌在窗口内**已出现账单**（说明匹配跑过、这一笔没对上） |
| 4 | `a6_waiting` | 等待上游账单 | 有规则，但对应令牌在窗口内**一笔账单都没有** |
| 5 | `pending` | 待对账 | 兜底（正常不应出现） |

### 6.2 孤儿账单行的状态

`record_type = upstream_unmatched` 的行恒为 `upstream_unmatched`「上游待匹配」。

> ⚠️ **修正记录**：设计初稿曾把 `upstream_unmatched` 也列为下游行的一种状态，与 `a6_pending` 语义重叠。已收敛为：**`upstream_unmatched` 只用于孤儿账单行**，下游行不再产生该取值。这样 5 + 1 = 6 种状态互不重叠。

**关键**：第 4 与第 5 的区别就是 Bug 1。旧实现用 `provider != "subarx"` 兜底，把第 4 种情况误判成第 5 种，线上一次误标 1768 条。

**另外**：统计口径（`summary` 的计数）与明细列表（`requests`）**必须共用同一份判定函数**。旧实现一处走 SQL 条件、一处走 Go 分支，是手工镜像，容易互相矛盾 —— 新实现统一。

---

## 7. 前端契约映射（逐字段 → 数据来源）

### 7.1 `GET /status`

```json
{ "enabled": true, "healthy": true, "status": 200, "detail": "" }
```
`enabled` **恒为 true**（不再有"未配置"态）。`healthy` = 采集服务在运行。

### 7.2 `GET /summary`

| 字段 | 来源 |
| --- | --- |
| `from` / `to` | 入参回显（缺省 = 当前时间往前 24 小时，`[from, to)` 半开） |
| `revenue` | `SUM(revenue_cny)` 全部调用 |
| `matched_revenue` | `SUM(revenue_cny)` 限已对账 |
| `upstream_cost` = `billed_upstream_cost` | `SUM(cost_cny)` 已匹配账单（两字段同值） |
| `gross_profit` | `matched_revenue − upstream_cost` |
| `margin_percent` | `gross_profit / matched_revenue × 100`，**`StringFixed(2)`，永不 null**（分母为 0 时给 `"0.00"`） |
| `matched` = `downstream_matched` | 已对账调用数（**JSON number**） |
| `unmatched` = `downstream_unmatched` | 待对账调用数（**JSON number**） |
| `upstream_unmatched` | 上游账单未匹配数（**JSON number**） |
| `record_total` | `matched + unmatched + upstream_unmatched`（**JSON number**） |
| `billed_count` | 已取得账单笔数（**JSON number**） |
| `calculated_count` | 恒 `0`（**JSON number**） |
| `cost_policy` | 恒 `"billed_or_subarx_api_or_rule"`（**保持契约原字符串**） |
| `subarx_unallocated_cost` | 恒 `"0.00000000"`（Subarx 已取消） |
| `subarx_unallocated_count` | 恒 `0` |
| `profit_scope` | 恒 `"matched_only"` |
| `currency` | 恒 `"CNY"` |
| `fx_usd_cny` | 当前生效汇率，`StringFixed(8)` |
| `fx_source` | `"config"` 或 `"settings"` |
| `fx_effective_at` | 配置时间（RFC3339，可为空串） |
| `fx_stale` | 恒 `false`（不再有陈旧兜底） |

### 7.3 `GET /timeseries`

`points[].start` 必须能被 `new Date()` 解析且**严格升序**（RFC3339Nano UTC）。`bucket` 仅作标题：窗口 ≤ 48h → `1小时`；≤ 14d → `6小时`；≤ 90d → `1天`；否则 `1周`。

### 7.4 `GET /requests`

- `page` 默认 1；`page_size` 默认 50、上限 100（超限按 100）
- `status` 仅接受 `all` / `matched` / `unmatched` / `upstream_unmatched`，并**原样回显**
- 行 key = `record_type:source_id:request_id:upstream_request_id`，**必须唯一**
- `record_type`：`downstream`（调用）/ `upstream_unmatched`（孤儿账单，`source_id = 0`）
- 金额字段用 `StringFixed(8)`；**未对账时 `upstream_cost` / `gross_profit` 给空字符串 `""`（不是 `0`）**，前端据此显示「—」
- `cost_source_label` **必须非空**（未知枚举的回落文案）
- `rows` 与 `total` 由**同一份判定函数**产出，保证自洽

### 7.5 规则接口

- `GET /account-rules`：列出**全部未删除账号**（含零调用账号！主人要靠它配置规则），附带账号名、分组、在该窗口的调用数、当前规则、`configured`
- `PUT /account-rules/:account_id`：`{ provider, external_key }` → upsert，`version += 1`
- `DELETE /account-rules/:account_id`
- `provider` 只接受 `a6`；收到 `subarx` 返回 `COMPANION_BAD_REQUEST: ...` 明确报错（Subarx 已下线）

### 7.6 错误契约（不可破坏）

- **绝不返回 401/403**（前端会清登录态跳 `/login`）。鉴权类失败统一 502/503 + `COMPANION_*` 标记。
- `classifyCompanionError` **只认 `message` 子串**，数字 `code` 不参与分类。因此：
  - `reason` 用官方 UPPER_SNAKE 风格（符合仓库规范）
  - **同时**在 `message` 里保留 `COMPANION_*` 前缀（满足前端）
- 响应信封：`{code, message, data}`，成功 `code = 0`。

---

## 8. 配置项

放进 `internal/config/config.go`（`mapstructure` + **必须配 `viper.SetDefault`**，否则 `TestConfigKeysAreEnvReachable` 红）：

| 键 | 环境变量 | 默认 | 说明 |
| --- | --- | --- | --- |
| `reconciliation.enabled` | `RECONCILIATION_ENABLED` | `false` | 关闭时不启动采集，但**页面仍可用**（只读本地已有的对账数据） |
| `reconciliation.fx_usd_cny_rate` | `RECONCILIATION_FX_USD_CNY_RATE` | `6.71` | 美元→人民币换算数字；如需"不换算"填 `1` |
| `reconciliation.a6.base_url` | `RECONCILIATION_A6_BASE_URL` | `""` | A6 站点地址 |
| `reconciliation.a6.access_token` | `RECONCILIATION_A6_ACCESS_TOKEN` | `""` | **系统访问令牌**（不要用网页登录密码） |
| `reconciliation.a6.user_id` | `RECONCILIATION_A6_USER_ID` | `""` | `New-API-User` 头 |
| `reconciliation.a6.lookback_hours` | `RECONCILIATION_A6_LOOKBACK_HOURS` | `24` | 首次同步回看窗口 |
| `reconciliation.a6.historical_token_lookback_hours` | 同名 | `48` | 旧令牌名回头补拉窗口 |
| `reconciliation.a6.timeout_seconds` | 同名 | `90` | |
| `reconciliation.usage_interval_seconds` | 同名 | `30` | 用量采集周期 |
| `reconciliation.a6_sync_interval_seconds` | 同名 | `300` | A6 同步周期 |
| `reconciliation.pricing_file` | `RECONCILIATION_PRICING_FILE` | `config/pricing.json` | 公开价格文档路径 |

**汇率可运行时覆盖**：若 `settings` 表存在键 `reconciliation_fx_usd_cny_rate`，优先取它（免重启）。**不修改 `SystemSettings` DTO**，避免触发 `api_contract_test.go` 的全 JSON 字符串比对。

**凭据纪律**：`access_token` 走环境变量注入，**绝不写入 git / 文档 / 日志 / 输出**。

---

## 9. 公开价格接口

`GET /api/provider/pricing` —— **注册在 `/api/v1` 之外**（否则会被鉴权中间件拦）。

- 在 `internal/server/routes/common.go` 加路由（与 `GET /health` 同级），**不经任何鉴权中间件**
- 数据源：静态 JSON 文件，**内容一字不改**，仅把 `site_domain` 从 `xzgc.asia` 改为 `chenshuapi.com`
- 响应结构完全保持 Hvoy schema 1.0：`{schema_version, success, message, data:{currency, price_unit, site_name, site_domain, updated_at, models[]}}`
- `updated_at` = **请求时刻**的 RFC3339（与旧实现一致，不是文件时间）
- `cache_create_price` / `cache_create_price_1h` **刻意不带 omitempty → 恒存在，可为 `null`**（有测试断言，不能改）
- 文件缺失 → `503 {"error":"pricing unavailable"}`；JSON 非法 → `500 {"error":"invalid pricing configuration"}`

> ⚠️ 与 `internal/config/config.go` 的 `PricingConfig`（LiteLLM 内部定价）**是两个完全不同的东西**，不要混。

---

## 10. 归档

| 对象 | 做法 |
| --- | --- |
| `extensions/sub2api-companion/`（**git-tracked，26 个文件**） | 移到 `extensions/archive/sub2api-companion/`（git 识别 rename，历史不丢），README 顶部加「已收编进主程序，仅作历史参考，不再维护」 |
| 生产 SQLite（VPS `/opt/sub2api/extensions/sub2api-companion/data/companion.db`） | **运维动作**：停容器 → 打包 `companion-db-backup-20260929.tar.gz` → 留存 → 校验可解压。**不导数据** |

---

## 11. 验证清单

```powershell
# backend/
gofmt -l .                                   # 必须无输出
go build ./...
go test -tags=unit ./...
go test -tags=integration ./internal/repository/
# frontend/
pnpm run check:i18n
pnpm exec vue-tsc --noEmit
pnpm run build
```

外加**契约自查**（逐条对照 §7）：

- [ ] 6 个计数字段都是 JSON number（不是字符串）
- [ ] `margin_percent` 永不为 null
- [ ] 全链路无 401/403
- [ ] `collect` 立即返回、不阻塞 30s
- [ ] `GET /status` 的 `enabled` 恒 true
- [ ] 明细行 key 唯一
- [ ] `timeseries.points[].start` 可解析且升序
- [ ] 未对账时 `upstream_cost` / `gross_profit` 是 `""`
- [ ] `cost_source_label` 非空
- [ ] 账号规则列表含零调用账号
- [ ] `reason` 与 `message` 里的 `COMPANION_*` 同时存在
