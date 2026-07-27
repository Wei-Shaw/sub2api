# 设计：代理池（代理分组）

本文档记录实测得出的硬约束、核心架构决策、被否决的方案，以及实现时必须守住的不变量。所有约束均附代码证据（文件:行号），实现与评审时应逐条核对。

---

## 1. 现状基线

| 事实 | 证据 |
| --- | --- |
| 不存在代理池/分组实体 | `backend/ent/schema/proxy.go:32-66` 无 group/pool 字段；全仓库 `proxy_group\|proxy_pool` 仅命中 ent 自动生成的 `ProxyGroupBy`（SQL GROUP BY 构建器，`backend/ent/proxy_query.go:635`） |
| 账号 1:1 绑定单代理 | `backend/ent/schema/account.go:91` `field.Int64("proxy_id")`；edge `:217-218` Unique |
| 代理过期回退是写路径批量作业 | `backend/internal/service/proxy_fallback.go:13` 纯函数，唯一调用方 `backend/internal/repository/proxy_repo.go:634`（`SweepExpiredProxies` 内） |
| 账号 hydration 只有一个入口 | 全仓库 `WithProxy()` 仅 `backend/internal/repository/account_repo.go:264` 一处 |
| 上游消费点 58 处 | `grep -rn "\.Proxy\.URL()" --include=*.go internal \| grep -v _test.go \| wc -l` = 58，分布于 43 个文件 |
| grok 代理链路已完整 | 中继 `openai_gateway_grok.go:100,904`；媒体 `grok_media.go:400,491`；配额 `grok_quota_service.go:580`；OAuth `grok_oauth_service.go:331` |

**结论**：代理池是全平台缺失的能力，不是 grok 特有的开关问题。

---

## 2. 四条硬约束（实测，不可违反）

### C1 — 禁止把选中的代理写回 `account.ProxyID`

`backend/internal/service/grok_token_provider.go` 在 OAuth token 刷新流程中对 `account.ProxyID` 做 CAS 校验：

```go
selectedProxyID := cloneGrokProxyID(account.ProxyID)   // :97  getAccessToken
selectedProxyID := cloneGrokProxyID(account.ProxyID)   // :275 waitForRefreshedToken
```

三处断言，任一不等即返回 `errOAuthRefreshAccountStateChanged`：

- `:145` 刷新协程返回后比对 `result.Account.ProxyID`
- `:167` token 版本检查后比对 `latestAccount.ProxyID`
- `:300` 等锁轮询中比对 `latest.ProxyID`

该 CAS 的语义是「刷新 token 期间，管理员没有在后台改这个账号绑定的代理」——它保护的是 OAuth 刷新的 IP 一致性（xAI 会因刷新 IP 与签发 IP 不符而拒绝），**不是**保护本次请求走哪条出站链路。

因此：

- 若代理池每次请求把选中的 proxy 写回 `account.ProxyID` 并持久化 → 三处断言大面积误报，grok OAuth 账号 token 刷新持续失败。
- 若仅在内存里改 `account.ProxyID`（不落库）→ `:97` 抓取的快照与 `:167` 从 repo 读回的 `latestAccount.ProxyID` 不一致，同样误报。

**红线：`account.ProxyID` 在代理池链路中全程只读。** 满足此约束后 CAS 逻辑零改动。

### C2 — grok OAuth 刷新会退化成直连

`backend/internal/service/grok_oauth_service.go:202`：

```go
proxyURL, err := s.proxyURL(ctx, account.ProxyID)
```

`proxyURL(ctx, proxyID *int64)`（`:315-332`）在 `proxyID == nil` 时返回空串（直连）。

池账号的 `proxy_id IS NULL`，因此：**中继请求走池内代理，OAuth 刷新走直连 → 出口 IP 不一致 → xAI 大概率拒绝刷新。**

这是 grok 特有的、必须专门处理的点。处理方式见 §4 决策 D4。

注意 `proxyURL()` 的其余四个调用点（`:55`、`:141`、`:175`、`:183`）参数来自管理端显式传入的 proxyID（OAuth 导入 / 授权流程），**保持单代理语义不变**。

### C3 — WebSocket 长连接的代理必须在连接生命周期内固定

`backend/internal/service/openai_ws_forwarder_v2.go:189` 与 `backend/internal/service/openai_ws_forwarder_ingress.go:597` 在闭包内取代理。若实现为「每帧/每轮重选」，同一条 WS 连接会出现多个出口 IP。

本设计的 hydration 时选择天然满足该约束：一次账号 hydration = 一个连接周期 = 一个固定代理。

### C4 — 过期代理不会被自动踢出池

`backend/internal/repository/proxy_repo.go:723-748` 的改投 SQL 形如：

```sql
UPDATE accounts SET proxy_id = ... WHERE proxy_id = $1 AND proxy_fallback_origin_id IS NULL
```

对 `proxy_id IS NULL` 的池账号**完全不命中**。

**因此过期/停用剔除只能在选择器的候选过滤阶段做**（运行时过滤），不能依赖既有 sweep 作业。这也避免了写放大。

---

## 3. 核心架构决策

### D1 — 在 hydration 点完成选择，而非在消费点

全部 58 处消费点是同一模式：

```go
// backend/internal/service/openai_gateway_grok.go:98-101
proxyURL := ""
if account.ProxyID != nil && account.Proxy != nil {
    proxyURL = account.Proxy.URL()
}
```

而 `WithProxy()` 预加载全仓库只有 `account_repo.go:264` 一处，且是「按 ID 批量 hydrate」的每请求路径。

**决策：把「池内选一个」做在 hydration 里，填入 `account.Proxy`。**

收益：

- 58 个消费点无需感知代理池的存在；
- 轮询粒度天然是「每请求」；
- 自动满足 C3（一次 hydrate = 一个固定代理）；
- 自动满足 C1（只写 `Proxy`，不碰 `ProxyID`）。

### D2 — 先做 Phase 0 重构，把改造面从 58 处压缩到 1 处

在 `backend/internal/service/account.go` 新增：

```go
// ProxyURL 返回账号当前生效的出站代理 URL，无代理时返回空串。
// 该方法是代理取值的唯一入口：账号可能通过 proxy_id 直接绑定单个代理，
// 也可能通过 proxy_group_id 由服务端在 hydration 时从组内选出一个代理，
// 两种情况下结果都体现在 Proxy 字段上。
func (a *Account) ProxyURL() string {
    if a == nil || a.Proxy == nil {
        return ""
    }
    return a.Proxy.URL()
}
```

把 58 处替换为 `proxyURL := account.ProxyURL()`。

**不变量证明（Phase 0 为零行为变化）：**

现有守卫有两种写法——多数是 `account.ProxyID != nil && account.Proxy != nil`，少数只判 `account.Proxy != nil`（如 `openai_gateway_chat_completions.go:270`、`openai_gateway_messages.go:334`）。新方法只判 `a.Proxy != nil`，两者等价的前提是不变量：

> **`account.Proxy != nil` ⟹ `account.ProxyID != nil`**

穷举 `account.Proxy` 的全部赋值点（`grep -rn "\.Proxy = " internal | grep -v _test.go`，排除 `transport.Proxy` 等同名字段）：

| 赋值点 | 守卫 |
| --- | --- |
| `repository/account_repo.go:294` | `if entAcc.Edges.Proxy != nil` —— ent edge 存在蕴含 `proxy_id` 非空 |
| `repository/account_repo.go:3170` | 外层 `if acc.ProxyID != nil`（`:3168`） |
| `service/grok_quota_service.go:583` | 外层 `if account.ProxyID == nil { return "" }`（`:576`） |
| `service/admin_account.go:738` | 赋值为 `nil` |

三处非空赋值全部受 `ProxyID != nil` 保护，不变量成立，故 Phase 0 可证明为零行为变化。

Phase 2 引入池后，该不变量被**有意打破**（池账号 `Proxy != nil` 而 `ProxyID == nil`），这正是 `ProxyURL()` 采用宽松守卫的原因，也是 Phase 0 必须先行的原因。

### D3 — Proxy ↔ Group 采用一对多

`proxies` 表加 `group_id`。一个代理只属于一个组。

理由：满足当前需求，改动最小，无需中间表和额外的 N:M 加载逻辑。若未来需要一个代理复用于多组，可再引入 `proxy_group_members` 中间表，届时 `group_id` 作为兼容视图保留。

### D4 — grok 出口一致性通过「sticky 策略 + OAuth 复用 hydrate 结果」解决

两步：

1. 组新增 `sticky_by_account` 布尔开关。开启时策略退化为 `hash(accountID) % len(candidates)`，同一账号在候选集不变时恒定选中同一代理。grok 平台账号建议默认开启。
2. 把 `grok_oauth_service.go:202` 从 `s.proxyURL(ctx, account.ProxyID)` 改为复用已 hydrate 的 `account.Proxy`（即 `account.ProxyURL()`），使刷新与中继同出口。

注意 sticky 仅保证「候选集不变时恒定」。候选集变化（成员增删、某代理过期）会导致重新映射，这是可接受的——此时原出口本就不可用。

### D5 — 选择器写成无 I/O 纯函数

`backend/internal/service/proxy_selector.go`：

```go
func SelectProxyFromGroup(candidates []Proxy, strategy string, now time.Time, seed uint64) (*Proxy, bool)
```

照 `proxy_fallback.go:13` `ResolveProxyFallbackTarget` 已验证的风格：纯函数、无 I/O、可穷举单测。轮询游标状态由调用方持有（原子计数器）。

候选过滤：`p.IsActive() && !p.IsExpired(now)`，复用 `service/proxy.go:33,38`。这是 C4 的解法。

全部候选不可用时返回 `(nil, false)`，由调用方决定降级策略（默认直连并告警，可配置为拒绝请求）。

---

## 4. 被否决的方案

| 方案 | 否决理由 |
| --- | --- |
| 在每个消费点调用池选择器 | 58 处散落改造，评审不可行，且极易遗漏导致部分链路不走池 |
| 每次请求把选中代理写回 `account.ProxyID` | 违反 C1，grok OAuth 刷新全面失败 |
| 复用 `fallback_mode`/`backup_proxy_id` 链实现池 | 回退链是「过期触发的持久化改写」（写路径），池是「每请求选择」（读路径），两者正交。混用会破坏 `SweepExpiredProxies` 语义，且 `proxy_fallback.go:18` 的 `visited` 防环集无法区分 proxy id 与 group id 命名空间 |
| 扩展 `AdminService` 承载组管理 | 该接口已 500+ 行；且 `ProxyService`（`service/proxy_service.go:60`）实际是死代码，管理端走 `AdminService`。新建独立 `ProxyGroupService` 更清晰 |
| 路由挂 `/api/v1/admin/proxies/groups` | gin 中同层 wildcard 与静态段冲突：`routes/admin.go:482` 已有 `GET /proxies/:id`。必须用独立顶层组 `/proxy-groups` |

---

## 5. 实现时的次级约束

### 5.1 连接池隔离模式

`backend/internal/repository/http_upstream.go:924` `buildCacheKey`：

| isolation | cacheKey |
| --- | --- |
| `account` | `account:{accountID}` |
| `account_proxy` | `account:{accountID}\|proxy:{proxyKey}` |
| `proxy`（默认） | `proxy:{proxyKey}` |

- `proxy` / `account_proxy` 模式：不同代理天然落不同 cacheKey，多客户端并存复用，**最优**。
- `account` 模式：`shouldReuseEntry`（`:714`）发现 `entry.proxyKey != proxyKey` 即判定失效 → `removeClientLocked`（`:670`）销毁并重建 Transport。池化后每请求重建连接池，**必须禁止「池 + account 隔离」组合**（启动校验或配置文档明确）。
- `maxUpstreamClients()`（`:672-681`）上限需按池大小上调，否则触发 `errUpstreamClientLimitReached`。

### 5.2 热路径缓存

hydration 位于每请求路径，组成员列表不能每次查库。复用仓库既有的 outbox 失效模式（`proxy_repo.go:246-259` 的 `SchedulerOutboxEventAccountBulkChanged` 分片投递，500/批见 `:26`），在组成员变更时投递失效事件。

`account_repo.go:3170` 那条批量路径必须同步改造，否则每账号一次组查询，产生 N+1。

### 5.3 探测缓存不得因组变更失效

`proxy_repo.go:126-134` 的 `proxyProbeIdentity` 只包含 `{protocol,host,port,username,password,status}`，语义是「网络出口是否变了」。**不要把 `group_id` 加进去**，否则组调整会触发无谓的探测缓存失效风暴（`invalidateProxyProbeSnapshots`，`:207`）。

### 5.4 迁移与 schema 双写

`backend/migrations/` 与 `backend/ent/schema/` 是双写的（例：`149_proxy_expiry_fallback.sql` 的 4 个列在 `ent/schema/proxy.go:55-66` 各有一份）。两边必须同步，然后 `cd backend && make generate`（`backend/Makefile:9-11`，同时重生成 ent 客户端与 `wire_gen.go`）。

迁移文件名规范：`{3位序号}_{snake_case}.sql`，当前最大为 `190_add_users_email_alias_dedup_index_notx.sql`，故新增用 `191_`。写法全程幂等（`IF NOT EXISTS`），索引命名 `{表}_{列}_idx`。

### 5.5 前端哨兵值

`frontend/src/components/account/EditAccountModal.vue:4040-4042` 有明确注释：后端期望 `proxy_id: 0` 表示清除代理，而非 `null`。`proxy_group_id` **必须沿用同一约定**，否则清空代理组失效。

`ProxySelector.vue` 的 `:72`、`:75`、`:83`、`:153` 四处 `modelValue === x` 严格相等判断，在扩展为组模式时需一并调整。

### 5.6 删除保护

`service/proxy_service.go:14` 现有 `ErrProxyInUse` 只检查 `CountAccountsByProxyID`。删除组时需检查：组内是否仍有代理成员、是否有账号绑定该组。新增 `ErrProxyGroupInUse` 而非复用。

### 5.7 导入导出的自然键约定

若为组支持迁移导出，必须遵循 `handler/admin/proxy_data.go` 既有约定：跨实例迁移用**自然键而非 ID**（参考 `BackupProxyID` 导出为 `BackupProxyName`，`:70-73`；导入反查 `:194-207`，查不到则降级并记 warning）。组同理导出 `GroupName`。

---

## 6. 开放问题决策（已确认）

| # | 问题 | 决策 | 理由 |
| --- | --- | --- | --- |
| 1 | 组内全部代理不可用 | **降级直连**（`ResolveProxy`/`SelectProxyFromGroup` 返回未命中 → 消费侧 `ProxyURL()` 空串） | 与存量「无代理」语义一致；拒绝请求会放大故障面。组级配置项延后。 |
| 2 | grok 是否强制 sticky | **不强制**；管理端/UI 推荐开启 `sticky_by_account`（文案提示），管理员可关 | 非 grok 账号可能需要轮询；强制会限制运维。 |
| 3 | 轮询游标 | **进程内原子计数器**（`DefaultProxyGroupResolver.rrCounters`） | 无额外 Redis RTT；多实例各自轮询整体仍均衡。 |
| 4 | 组级过期/告警 | **不做**；继续用 per-proxy 的 `ExpiryWarnDays` / 选择器运行时过滤过期成员 | 组级 SQL 与产品语义收益不足，避免范围膨胀。 |

### 6.1 实施期补充决策

- **连接池隔离**：`connection_pool_isolation=account` 与代理组不兼容（出口变化会重建 Transport）。配置校验在该模式下 **slog.Warn**，推荐 `proxy` 或默认 `account_proxy`。
- **max_upstream_clients**：默认 **5000** 且可配置（`gateway.max_upstream_clients`）；灰度前按「账号数 × 组内健康成员上界」复核，不足再上调。
- **导入导出自然键**（tasks 4.10/4.11）：明确延后到独立 change。
