# Gateway Extraction · POC-C · GatewayService 主流程依赖图

> 状态：调研报告。
> 关联：GATEWAY-EXTRACTION-PROPOSAL.md §3 §5 §10 Q9 / Q10
> 范围：仅列三条主流程的 hot path（每请求都会调）。错误重试 / 退化分支移到附录 B。

所有行号基于 `feature/plugin-grpc` 分支 HEAD：
- `gateway_service.go` 9845 行
- `openai_gateway_service.go` 6345 行
- `antigravity_gateway_service.go` 4568 行
- `gateway_handler.go` 1979 行
- `openai_gateway_handler.go` 1744 行

---

## 1. Hot Path 依赖图

### 1.1 Anthropic Forward（含 Antigravity 借道）

`/v1/messages` 请求生命周期。入口：`gateway_handler.go:112 Messages`。

```
[1] handler.gateway.Messages (gateway_handler.go:112)
    │ ParseGatewayRequest                       gateway_request.go:130
    │ gatewayService.ResolveChannelMappingAndRestrict  gateway_service.go:8988
    │ concurrencyHelper.IncrementWaitCount       :206 → ConcurrencyService :211
    │ concurrencyHelper.AcquireUserSlotWithWait  :221 → ConcurrencyService :168
    │ billingCacheService.PrepareBillingCheckForRequest :240
    │   └─ BillingCacheService :126 (RPM check / balance / sub / quota plan)
    │ gatewayService.GenerateSessionHash         gw_service.go:665
    │ gatewayService.GetCachedSessionAccountID   gw_service.go:758  (cache.GetSessionAccountID)
    ▼
[2] LOOP: select-account → forward → record
    │ gatewayService.SelectAccountWithLoadAwareness  :1415
    │   ├─ SchedulerSnapshotService.ListSchedulableAccounts (load → DB)
    │   ├─ BillingCacheService.FilterAccountsByServiceQuotaSchedulability :387
    │   ├─ ConcurrencyService.GetAccountsLoadBatch  :288
    │   └─ cache.SetSessionAccountID (sticky bind)   gw_service.go:1810
    │ concurrencyHelper.AcquireAccountSlotWithWaitTimeout
    │ gatewayService.BindStickySession            :749  (cache.SetSessionAccountID)
    │ billingTicket.Consume(channelID, accountID) billing_cache_service_ticket.go:86
    │
    ├─ if account.Platform == antigravity (gateway_handler.go:776):
    │     antigravityGatewayService.Forward       antigravity_gw_svc.go:1357
    │        ├─ tokenProvider.GetAccessToken       (Antigravity-specific)
    │        ├─ antigravity.TransformClaudeToGeminiWithOptions
    │        ├─ httpUpstream.DoWithTLS / SSE 流式回写
    │        └─ rateLimitService.CheckErrorPolicy  :950 (failover decision)
    │ else (gateway_handler.go:778):
    │     gatewayService.Forward                  gw_service.go:4348
    │        ├─ identityService.GetOrCreateFingerprint :4438
    │        ├─ settingService.GetGatewayForwardingSettings  :1607
    │        ├─ gatewayService.GetAccessToken     :3772 → claudeTokenProvider.GetAccessToken
    │        ├─ tlsFPProfileService.ResolveTLSProfile
    │        ├─ httpUpstream.DoWithTLS  (with TLS profile + proxy)
    │        ├─ rateLimitService.UpdateSessionWindow :1222 (response)
    │        └─ rateLimitService.HandleUpstreamError :134 (error path)
    │ gatewayService.IncrementAccountRPM           :2685 (Anthropic OAuth 才调)
    ▼
[3] async submitUsageRecordTask → gatewayService.RecordUsage gw_service.go:8450
    │ recordUsageCore                              :8621
    │   ├─ resolveCacheTTLUsageOverrideTarget
    │   ├─ getUserGroupRateMultiplier
    │   ├─ billingService.CalculateCostUnified / CalculateImageCost
    │   ├─ resolver.Resolve (ModelPricingResolver, channel-level pricing)
    │   ├─ resolveAccountStatsCost  :8558 (PluginHook AccountStatsResolver)
    │   ├─ applyUsageBilling :8231
    │   │    ├─ usageBillingRepo.Apply (DB tx: deduct balance/sub + insert usage_billing)
    │   │    ├─ billingCacheService.QueueDeductBalance / QueueUpdateSubscriptionUsage
    │   │    ├─ billingCacheService.QueueUpdateAPIKeyRateLimitUsage
    │   │    ├─ billingCacheService.RecordServiceQuotaUsage  :370 (RPM/TPM 计数)
    │   │    ├─ deferredService.ScheduleLastUsedUpdate
    │   │    ├─ go notifyBalanceLow → balanceNotifyService.CheckBalanceAfterDeduction :8327
    │   │    └─ go notifyAccountQuota → balanceNotifyService.CheckAccountQuotaAfterIncrement :8368
    │   └─ usageLogRepo.Insert (writeUsageLogBestEffort)
    ▼
[4] billingTicket.Close() (defer at gateway_handler.go:251)
```

### 1.2 OpenAI ChatCompletions / Responses

`/v1/responses` 请求生命周期。入口：`openai_gateway_handler.go:89 Responses`。

```
[1] handler.openai.Responses (openai_gateway_handler.go:89)
    │ extractOpenAIRequestMetaFromBody (gjson 浅解析)
    │ gatewayService.ResolveChannelMappingAndRestrict  gw_service.go:8988
    │ acquireResponsesUserSlot → ConcurrencyService.AcquireUserSlot  :168
    │ billingCacheService.PrepareBillingCheckForRequest  :126
    │ openaiGatewayService.GenerateSessionHash (header / prompt_cache_key)
    ▼
[2] LOOP: select-account → forward → record
    │ openaiGatewayService.SelectAccountWithScheduler
    │   ├─ openaiScheduler.SelectAccount (top-K with load + sticky-prev)
    │   ├─ schedulerSnapshot.ListSchedulableAccounts
    │   └─ concurrencyService.GetAccountsLoadBatch
    │ acquireResponsesAccountSlot → ConcurrencyService.AcquireAccountSlot  :129
    │ billingTicket.Consume (channelID, accountID)
    │ openaiGatewayService.Forward                openai_gw_svc.go:2080
    │   ├─ openAITokenProvider.GetAccessToken     (OAuth refresh)
    │   ├─ openaiWSResolver.Resolve (HTTP vs WS 上游)
    │   ├─ codexDetector.* (codex_cli_only enforcement)
    │   ├─ httpUpstream.DoWithTLS (or WS pool dial)
    │   ├─ toolCorrector.* (Codex tool schema fix-up)
    │   └─ rateLimitService.HandleUpstreamError    (error path)
    │ openaiGatewayService.UpdateCodexUsageSnapshotFromHeaders
    │ openaiGatewayService.ReportOpenAIAccountScheduleResult
    ▼
[3] async submitUsageRecordTask → openaiGatewayService.RecordUsage  openai_gw_svc.go:5143
    │ 同 §1.1 [3]：recordUsageCore 等价物 → applyUsageBilling 全套写入
    ▼
[4] billingTicket.Close() (defer)
```

注意：OpenAI 路径不调 `IncrementAccountRPM`（OpenAI 计数走 `ReportOpenAIAccountScheduleResult` 内部 stats 池 + `UpdateCodexUsageSnapshotFromHeaders`）。

### 1.3 Antigravity Forward / ForwardGemini

两条入口：
- `/antigravity/v1/messages` (Anthropic 协议) → `antigravityGatewayService.Forward` (`antigravity_gw_svc.go:1357`)
- `/v1beta/models/{model}:generateContent` (Gemini 原生) → `ForwardGemini` (`antigravity_gw_svc.go:2109`)

两者共享 `antigravityRetryLoop` (`:555`)。下面以 Forward 为例：

```
[1] handler.gateway.Messages 进入 antigravity 分支 (gateway_handler.go:776)
    │ 与 §1.1 [1] 完全相同（先经 Anthropic handler 流程）
    ▼
[2] antigravityGatewayService.Forward (antigravity_gw_svc.go:1357)
    │ json.Unmarshal → antigravity.ClaudeRequest
    │ getMappedModel (account.GetMappedModel + Default mapping)  :1012
    │ tokenProvider.GetAccessToken  (AntigravityTokenProvider, gw_svc 不持有)
    │ settingService.* (transform options)
    │ antigravity.TransformClaudeToGeminiWithOptions (request body 转换)
    │ antigravityRetryLoop  :555
    │   ├─ httpUpstream.DoWithTLS (无 TLS profile 解析步骤)
    │   ├─ checkErrorPolicy → rateLimitService.CheckErrorPolicy :950
    │   ├─ accountUsageService.* (积分余额检查)
    │   ├─ schedulerSnapshot.* (single-account-retry 路径)
    │   ├─ cache.DeleteSessionAccountID (model-level rate limit 时清粘性)
    │   └─ internal500Cache.* (INTERNAL 500 渐进惩罚)
    │ stream → antigravity.TransformGeminiToClaude (响应转换)
    ▼
[3] handler 异步 RecordUsageWithLongContext (Gemini 长上下文双倍计费)
    │ → recordUsageCore 同 §1.1 [3]
    ▼
[4] billingTicket.Close()
```

`ForwardGemini`（`:2109`）省略 Claude→Gemini 转换，其余步骤完全相同。

---

## 2. Stable Surface 推荐

> 行：每一种 plugin owner 在 hot path 必然要"调到"的能力。
> 列：必须 host RPC（跨进程）/ plugin 内（封装在自家 SDK）/ 完全本地（纯 CPU 工作）。

| 能力 | 现有调用 | host RPC（必须） | plugin 内 | 完全本地 |
|---|---|---|---|---|
| 账号选择 | `SelectAccountWithLoadAwareness` :1415 / `SelectAccountWithScheduler` | **`SelectAccount`** | – | – |
| 调度池过滤 | `SchedulerSnapshotService.ListSchedulableAccounts` | （SelectAccount 内联）| – | – |
| 用户并发槽 | `ConcurrencyService.AcquireUserSlot` :168 | **`AcquireUserSlot`/`Release`**（owner 才调）| – | – |
| 账号并发槽 | `ConcurrencyService.AcquireAccountSlot` :129 | **`AcquireAccountSlot`/`Release`** | – | – |
| Wait 计数器 | `IncrementWaitCount` / `IncrementAccountWaitCount` | **`IncrementWait`/`Decrement`** | – | – |
| 计费两阶段票据 | `BillingCacheService.PrepareBillingCheckForRequest` :126 / `Ticket.Consume` :86 / `Ticket.Close` | **`AcquireBillingTicket` / `ConsumeBillingTicket` / `CloseBillingTicket`** | – | – |
| RPM 增量 | `gatewayService.IncrementAccountRPM` :2685 | **`IncrementAccountRPM`** | – | – |
| 凭据获取 | `claudeTokenProvider.GetAccessToken` :3792 / `openAITokenProvider` / `antigravityTokenProvider` | **`GetUpstreamCredentials(account_id, scope)`**（POC-B 推荐细化）| 凭据缓存 | OAuth 刷新算法 |
| 凭据回写 | `tokenProvider.RefreshAndPersist` 类调用（写 `account.Credentials`）| **`UpdateAccountCredentials(CAS)`** | – | – |
| 模型定价 | `ModelPricingResolver.Resolve` / `BillingService.CalculateCostUnified` | **`ResolveModelPricing` + `CalculateCost`**（已有 ResolveModelPricing；CalculateCost 新增）| – | – |
| 渠道映射 | `gatewayService.ResolveChannelMappingAndRestrict` :8988 | **`ResolveChannelMapping`** | – | – |
| 粘性会话查/绑/清 | `cache.GetSessionAccountID` :758 / `BindStickySession` :749 / `cache.DeleteSessionAccountID` | **`LookupStickySession` / `BindStickySession` / `ClearStickySession`** | – | – |
| TLS 指纹 profile | `tlsFPProfileService.ResolveTLSProfile` :4524 | **`ResolveTLSProfile(account)`**（轻量字段，可考虑随 Account 一起返回）| – | – |
| 客户端身份指纹 | `identityService.GetOrCreateFingerprint` :4438 | **`GetOrCreateFingerprint`** | – | – |
| 上游 HTTP 调用 | `httpUpstream.DoWithTLS` | – | **plugin 自带 http.Client** | – |
| 上游 WS 池 | `openaiWSPool.*` | – | **plugin 内私有** | – |
| 错误策略命中 | `rateLimitService.CheckErrorPolicy` :950 / `HandleUpstreamError` :134 | **`HandleUpstreamError`**（host 维护账号 disable / 限流计数）| – | – |
| 会话窗口更新 | `rateLimitService.UpdateSessionWindow` :1222 | **`UpdateSessionWindow`** | – | – |
| 错误透传规则 | `errorPassthroughService` (BindErrorPassthroughService) | **`ResolveErrorPassthroughRule`** | – | – |
| 设置读取 | `settingService.GetGatewayForwardingSettings` :1607 等 | **`GetGatewayForwardingSettings`**（已有 HostSettings 可复用）| – | – |
| 协议解析 / 转换 | `ParseGatewayRequest` / `antigravity.TransformClaudeToGeminiWithOptions` / `claudeOAuthNormalize` | – | – | **plugin 内纯函数** |
| 模型映射应用 | `applyThinkingModelSuffix` / `replaceModelInBody` / `claude.NormalizeModelID` | – | – | **plugin 内纯函数** |
| Cache breakpoint | `addMessageCacheBreakpoints` / `enforceCacheControlLimit` | – | – | **plugin 内纯函数** |
| Tool name rewrite | `buildToolNameRewriteFromBody` / `applyToolNameRewriteToBody` | – | – | **plugin 内纯函数** |
| 摘要会话 | `digestStore.Find` / `Save` :775-800 | **`LookupDigestSession` / `SaveDigestSession`** | – | – |
| Usage 记录 | `RecordUsage` / `recordUsageCore` :8621（含 applyUsageBilling 5+ 个写入）| **`RecordUsage`**（必须留 host，详见 §4）| – | – |
| 账号统计成本钩子 | `resolveAccountStatsCost` :8538 | – | **AccountStatsResolver 反向 hook**（已有）| – |
| 账户写入限速 | `accountWriteThrottle.*` (codex snapshot) | – | **plugin 内私有** | – |
| 退化更新 | `deferredService.ScheduleLastUsedUpdate` | （RecordUsage 内调）| – | – |
| INTERNAL 500 计数 | `internal500Cache.*`（仅 antigravity）| **新增 `IncrementAccountInternal500`** 或留 plugin 内 redis（看 POC-A）| – | – |

**结论**：必须新增的 host RPC（在 GATEWAY-EXTRACTION-PROPOSAL §5.2 列表上的增量）：
- `IncrementAccountRPM`、`IncrementWait/DecrementWait`、`UpdateSessionWindow`、`HandleUpstreamError`、`ResolveErrorPassthroughRule`、`ResolveChannelMapping`、`ResolveTLSProfile`、`GetOrCreateFingerprint`、`LookupDigestSession/SaveDigestSession`、`CalculateCost`、`IncrementAccountInternal500`

**总数**：在 PROPOSAL §5.2 已有 11 条之上 +11 → **共 22 条 host RPC**。其中 5 条（CalculateCost / ResolveChannelMapping / GetOrCreateFingerprint / ResolveTLSProfile / DigestSession）属于"参数小、返回小、调用频率每请求 1-2 次"，跨进程代价可控；剩下的 6 条（HandleUpstreamError 等）在错误路径才命中，频率低。

---

## 3. God Object 内部状态分类

### 3.1 GatewayService 字段（gateway_service.go:538-583，24 个字段）

**必须进 SDK / 跨进程暴露**（plugin owner 需要等价能力）：

| 字段 | 用途 | 暴露方式 |
|---|---|---|
| `accountRepo` `groupRepo` `userRepo` `userSubRepo` `userGroupRateRepo` | 实体读取 | 走 host RPC `GetAccount/GetGroup/GetUser` |
| `usageLogRepo` `usageBillingRepo` | 计费写入 | 走 host RPC `RecordUsage`（不直接暴露 repo）|
| `cache` (`GatewayCache`) | 粘性会话 + 模型 metadata 缓存 | host RPC `LookupSticky/BindSticky` |
| `digestStore` | OpenAI digest session | host RPC `LookupDigestSession/SaveDigestSession` |
| `schedulerSnapshot` | 候选账号列表 | 走 `SelectAccount` RPC，不直接暴露 |
| `billingService` `rateLimitService` `billingCacheService` `concurrencyService` | 横切关注点 | 走 §2 表中对应 RPC |
| `httpUpstream` | 上游 HTTP 调用 | **plugin 自带 client，不需要** |
| `claudeTokenProvider` | Anthropic Service Account JWT | host RPC `GetUpstreamCredentials`（POC-B 详细方案）|
| `identityService` | Client fingerprint | host RPC `GetOrCreateFingerprint` |
| `tlsFPProfileService` | TLS 指纹 | host RPC `ResolveTLSProfile`（小对象，可随 Account 返回）|
| `resolver` (`ModelPricingResolver`) | 渠道定价解析 | 走 host RPC `ResolveModelPricing`（已有）|
| `responseHeaderFilter` | 响应头过滤规则 | host RPC `GetResponseHeaderFilter` 或 plugin 启动时下发 |
| `settingService` | 全局设置 | host RPC `GetSettings`（已有 HostSettings）|
| `balanceNotifyService` | 余额低 / 配额到点通知 | （RecordUsage 内调，plugin 不直接看见）|
| `accountStatsResolver` | 渠道管理插件 hook | **保留现有反向 hook**（AccountStatsResolver）|

**host 留着、不外暴**（实现细节）：

| 字段 | 原因 |
|---|---|
| `userGroupRateResolver` `userGroupRateCache` `userGroupRateSF` (3 个 LRU + singleflight) | 内部缓存优化，由 host CalculateCost 内部使用 |
| `modelsListCache` `modelsListCacheTTL` | `/v1/models` 端点的内部 cache |
| `sessionLimitCache` `rpmCache` | Anthropic OAuth 5h 会话限制 / RPM 缓存 — host 通过 RPM RPC 暴露语义而非缓存对象 |
| `debugModelRouting` `debugClaudeMimic` `debugGatewayBodyFile` | 进程级 debug switch，plugin 自己管 |
| `channelCacheReader` | 渠道配置只读快照（host 内部写时拷贝结构）|
| `cfg` | 进程级配置树，plugin 走 SettingService 拉子集 |
| `deferredService` | host 内部异步任务 worker 池，由 RecordUsage 内调 |
| `accountStatsMu` | 锁是字段，不暴露 |

### 3.2 OpenAIGatewayService 字段（openai_gateway_service.go:314-359，22 个字段 + 7 个 once）

**进 SDK**：与 §3.1 同名字段 11 个（accountRepo / usageLog / cache / cfg / scheduler / concurrency / billing / rateLimit / billingCache / httpUpstream / settings / resolver / channelCacheReader / balanceNotify / responseHeaderFilter / accountStatsResolver）。

**OpenAI 专属、必须进 plugin**（不跨进程，plugin 自带）：
- `openaiWSPool` / `openaiWSStateStore` / `openaiWSPassthroughDialer` / `openaiAccountStats` / `openaiWSFallbackUntil` / `openaiWSRetryMetrics` / `codexSnapshotThrottle`
- `openaiScheduler`（OpenAI 私有调度器，区分 sticky-prev / load-aware top-K — 拆 plugin 时整个搬过去）
- `openAITokenProvider`（POC-B 推荐 plugin 进程内运行，host 仅做 CAS 持久化）
- `codexDetector` / `toolCorrector` / `openaiWSResolver`（纯函数 / 配置读取，plugin 内）

**host 留着**：`userSubRepo` `userRepo` `userGroupRateResolver` `deferredService`（同 §3.1）。

### 3.3 AntigravityGatewayService 字段（antigravity_gateway_service.go:879-890，11 个字段）

字段最少，几乎都是"现有 GatewayService 字段子集"：

| 字段 | 处理 |
|---|---|
| `accountRepo` `cache` `schedulerSnapshot` `rateLimitService` `httpUpstream` `settingService` | 同 §3.1，走 host RPC |
| `tokenProvider` (`*AntigravityTokenProvider`) | POC-B：plugin 进程内运行，host 仅 CAS 持久化 |
| `internal500Cache` | host RPC `IncrementAccountInternal500`（细粒度 redis 计数）|
| `accountUsageService` | host RPC `GetAccountUsage`（积分余额）|
| `eventPublisher` | 反向 hook（已有 PluginEventPublisher）|

---

## 4. RecordUsage 扇出分析

`recordUsageCore` (`gateway_service.go:8621`) 单次调用扇出：

| 写入目标 | 表 / 资源 | 调用栈 | 同步/异步 | 可能延迟 |
|---|---|---|---|---|
| `usage_logs` | DB INSERT | `usageLogRepo.Insert` (writeUsageLogBestEffort) | 异步 best-effort | 1-5 ms |
| `usage_billing` | DB tx + balance / sub deduction（带 RETURNING）| `usageBillingRepo.Apply` (applyUsageBilling :8231) | **同步**（同一事务） | 5-30 ms（含主键冲突重试）|
| `users.balance` | balance 列扣减 | 同上 tx 内 UPDATE RETURNING | 同步 | 包含在 5-30 ms |
| `user_subscriptions.balance_used` | sub 配额扣减 | 同上 tx 内 | 同步 | 包含在 5-30 ms |
| `api_keys.rate_limit_*` | redis 增量 | `billingCacheService.QueueUpdateAPIKeyRateLimitUsage` | 异步 worker | < 1 ms |
| Service quota 计数 | redis（RPM / TPM / TPD）| `billingCacheService.RecordServiceQuotaUsage` :370 | 异步（detached ctx + 2 min timeout）| < 1 ms |
| `accounts.last_used_at` | DB | `deferredService.ScheduleLastUsedUpdate` | 异步合批 | 0 ms（仅入队）|
| 余额低通知 | smtp / webhook | `notifyBalanceLow` → `balanceNotifyService.CheckBalanceAfterDeduction` :8327 | 异步 goroutine | 0 ms（goroutine fire-and-forget）|
| 账号配额通知 | smtp / webhook | `notifyAccountQuota` → `CheckAccountQuotaAfterIncrement` :8368 | 异步 goroutine | 0 ms |
| API Key 缓存失效 | in-memory | `apiKeyAuthCacheInvalidator.InvalidateAuthCacheByKey` | 同步 | < 1 ms |
| Plugin AccountStats hook | 反向 RPC（plugin → host → plugin）| `resolveAccountStatsCost` :8538 | 同步 | 1-3 ms（gRPC）|

整条 recordUsage 同步部分总耗时约 **6-35 ms**。

**跨进程把 RecordUsage 搬到 plugin 是否可行？**

**结论：必须留 host 进程内**。原因：

1. **8 个写入目标里 5 个是 DB / Redis 直连**（usage_logs / usage_billing tx / api_keys redis / quota redis / accounts deferred）。每搬一个都得新增 host RPC。
2. **`usage_billing.Apply` 是包含 RETURNING 的事务**：返回 NewBalance / QuotaState 用于通知判断。跨进程后 plugin 还得把这两个字段透传回去触发通知。
3. **写入扇出有依赖关系**：notify 必须在 tx 提交后；redis quota / rate_limit 必须在 tx 失败时跳过。这种逻辑放进 plugin 等于把整个 billing 子系统搬过去。
4. **plugin 拆分目标是"代码量收敛"**：recordUsage 主体 ~370 行，迁移后省下来的代码不到 god object 的 4%；而新增 host RPC 数量却要 4-5 个。
5. **现有反向 hook 已经够了**：`AccountStatsResolver` 让插件在 host 算完成本后改写 account_stats_cost，这是 90% 渠道管理插件的需求。其他需求走 `EventsExtension.gateway.usage.recorded` 异步消费即可。

**推荐架构**：plugin Forward 完成后，**单次 `RecordUsage(host_rpc)` 调用** 把 `ForwardResult + ChannelUsageFields + ServiceQuotaRequest` 送进 host，host 自己跑 recordUsageCore。RPC 总尺寸 < 4 KB，频率 = 每请求 1 次，跨进程开销 < 1 ms。已有 PROPOSAL §5.2 `RecordUsage` RPC 即对应该方案。

---

## 5. 横切关注点接入点

### 5.1 ConcurrencyService（gateway hot path）

| 方法 | hot path 调用点 | 频率 |
|---|---|---|
| `IncrementWaitCount` :211 | `gateway_handler.go:206` | 每请求 1（等待入队前）|
| `DecrementWaitCount` :228 | `gateway_handler.go:227` `:243` | 每请求 1（拿到 user slot 后）|
| `AcquireUserSlot` :168 | via `concurrencyHelper.AcquireUserSlotWithWait` :221 | 每请求 1 |
| `IncrementAccountWaitCount` :243 / `DecrementAccountWaitCount` :257 | `gateway_handler.go:381` `:393` | 当 `selection.WaitPlan != nil` 时 1 次 |
| `AcquireAccountSlot` :129 | `gateway_handler.go:402` | 每请求 0-1（snapshot select 时优先在 SelectAccount 内 acquire；fallback 才走这条）|
| `GetAccountWaitingCount` :271 | `gateway_service.go:1513` `:1709` `:1912` | 在 SelectAccount 排序阶段 1 次 |
| `GetAccountsLoadBatch` :288 | `gateway_service.go:1763` `:2015` | 每次 SelectAccount 1 次 |

**plugin 接入策略**：plugin 通过 host RPC 调（推荐 `AcquireUserSlot` / `AcquireAccountSlot` / `Release`）。每次 RPC 携带 `lease_id` 让 host 兜底释放（client crash 时 deferred cleanup）。

### 5.2 RateLimitService

| 方法 | hot path | 频率 |
|---|---|---|
| `UpdateSessionWindow` :1222 | `gateway_service.go:5348` `:5638` `:7273` | 每请求成功后 1 次（解析响应头）|
| `HandleUpstreamError` :134 | `gateway_service.go:7045` `:7153` `:7163` `:7636` | 错误路径 1 次（401 / 403 / 429 / 529 → 设置封禁）|
| `CheckErrorPolicy` :950（antigravity）| `antigravity_gw_svc.go:950` | 错误路径 1 次 |
| `PreCheckUsage` :302 / `PreCheckUsageBatch` :415 | SelectAccount 内 batch | 每次 SelectAccount 1 次（batch 形式）|
| `GeminiCooldown` :660 | gemini schedule | 每 gemini 请求 1 次 |

**plugin 接入策略**：plugin 在 `Forward` 拿到响应后调 `host.UpdateSessionWindow(account_id, headers)` 和（若错）`host.HandleUpstreamError(account_id, status, body)`。**留 host 主因**：状态机（账号 disable / model rate-limit / cooldown）所有读者（其它 plugin、admin handler、scheduler）都依赖同一份 host state，跨进程会话级缓存无法共享。

### 5.3 BillingCacheService（票据三段式）

| 阶段 | 方法 | 行号 | 调用点 |
|---|---|---|---|
| Acquire | `PrepareBillingCheckForRequest` | :126 | handler `:135` `:145`（user slot 拿到后）|
| Consume | `Ticket.Consume` | ticket.go:86 | handler 选定 account 后 `:332` |
| Close | `Ticket.Close` | ticket.go:123 | defer 在 handler `:147` |
| Filter | `FilterAccountsByServiceQuotaSchedulability` | :387 | gateway_service.go:1560（SelectAccount 内）|
| Record | `RecordServiceQuotaUsage` | :370 | applyUsageBilling :8285 |

**plugin 接入策略**：plugin 通过 4 个 host RPC（`Acquire/Consume/Close/Filter`）拿到 `lease_id`。**Filter 必须留 host**：SelectAccount 在 host 跑（因为账号池本就在 host），filter 直接内联调用即可，plugin 不应该看到。

**Close 死锁风险**：plugin crash 后 host 必须能基于 `lease_id` 超时清理（推荐 ticket 自带 5 min TTL + plugin 端 keepalive）。

---

## 6. 跨边界 DTO 字段清单

### 6.1 ParsedRequest（gateway_request.go:66）

```
Body []byte                  // 原始请求体（保留用于转发）
Model string
Stream bool
MetadataUserID string        // metadata.user_id（会话亲和）
System any                   // anthropic system / gemini systemInstruction
Messages []any               // 原始 messages 数组
HasSystem bool
ThinkingEnabled bool
OutputEffort string          // output_config.effort
MaxTokens int
SessionContext *SessionContext  // {ClientIP, UserAgent, APIKeyID}
GroupID *int64
OnUpstreamAccepted func()    // ⚠️ 闭包，跨进程不可序列化
```

跨进程注意：`OnUpstreamAccepted` 是回调闭包（用户消息串行队列释放锁），跨进程必须改为 `host.NotifyUpstreamAccepted(request_id)` 信号。

### 6.2 ForwardResult / ClaudeUsage（gateway_service.go:480 / :491）

**ClaudeUsage**（7 字段，全列）：
```
InputTokens int
OutputTokens int
CacheCreationInputTokens int
CacheReadInputTokens int
CacheCreation5mTokens int
CacheCreation1hTokens int
ImageOutputTokens int
```

**ForwardResult**（13 字段，全列）：
```
RequestID string
Usage ClaudeUsage
Model string
UpstreamModel string
Stream bool
Duration time.Duration
FirstTokenMs *int
ClientDisconnect bool
ReasoningEffort *string
ImageCount int
ImageSize string
```

### 6.3 OpenAIUsage / OpenAIForwardResult（openai_gateway_service.go:204 / :213）

**OpenAIUsage**（5 字段，全列）：
```
InputTokens, OutputTokens,
CacheCreationInputTokens, CacheReadInputTokens,
ImageOutputTokens int
```

**OpenAIForwardResult**（13 字段，全列）：
```
RequestID string
Usage OpenAIUsage
Model string
BillingModel string
UpstreamModel string
ServiceTier *string
ReasoningEffort *string
Stream bool
OpenAIWSMode bool
ResponseHeaders http.Header   // ⚠️ map[string][]string，proto 用 repeated KV
Duration time.Duration
FirstTokenMs *int
ImageCount int
ImageSize string
```

`ResponseHeaders` 跨进程时需序列化为 `map<string, repeated string>` 或保留全部头给 host 做 codex snapshot 解析（`UpdateCodexUsageSnapshotFromHeaders`）— 由 plugin 单独通过 `host.UpdateCodexSnapshot(account_id, headers)` RPC 上送即可，避免每个 ForwardResult 都拖一个完整 header map 跨进程。

---

## 7. 拆分期双写灰度推荐

**现状**：仓库**没有**通用 feature flag 系统（既无 `cfg.Features` 也无 `setting.IsEnabled` 这种通用入口）。`SettingService` 存在一些零散的 `IsEmailVerifyEnabled / IsBackendModeEnabled / GetGatewayForwardingSettings` 读取，都走 DB-backed `system_settings` 表，没有"按 group / 按 user 灰度"的能力。

**推荐切换粒度（从粗到细）**：

| 粒度 | 实现成本 | 灰度灵敏度 | 推荐 |
|---|---|---|---|
| 全局 1 bit | 最低（环境变量 / system_settings 加一行）| 无（要么全切要么全不切）| ❌ 风险大 |
| **per-platform** | 低（manifest 注册 ProviderRegistry，host 优先走 plugin，否则 fallback 内置实现）| 中（OpenAI 先切，Antigravity 后切）| **✅ 推荐主路径** |
| per-group | 中（DB schema：`groups.gateway_plugin_id` 列）| 高 | 第二阶段补充 |
| per-account | 高（账号级开关 + 调度池过滤）| 最高 | 不做（粒度太碎，scheduling 不一致风险）|
| per-request hash | 中（按 request_id 取模）| 高（A/B test）| 暂不做 |

**具体方案**：

1. **阶段一（host 内重构）期间**：完全不需要灰度。`GatewayProvider` 接口是单一实现的纯重构，做完就 100% 切。
2. **阶段二（SDK / Mediator）期间**：引入 manifest-driven 切换 — `ProviderRegistry` 没注册某 `(platform, protocol)` 时 fallback 到 host 内置实现。这本质上就是 "per-platform" 灰度。先把 OpenAI plugin 装上、看 1 周；再把 Antigravity plugin 装上；最后才是 Anthropic 主体。
3. **阶段三（per-group）补充**：在 `groups` 表加 `gateway_plugin_override` (nullable text) 列。SelectAccountForEndpoint 时优先匹配 group override → 全局 ProviderRegistry。这给运维一个"测试环境某个 group 用 plugin、生产环境其他 group 用内置"的开关。
4. **回滚机制**：plugin 加载失败 / Mediator dial 失败 → host 自动 fallback 到内置实现。不需要单独的"开关"，故障即降级。

**反例 ❌（避免）**：在 `gateway_handler.go` 里散落 `if cfg.UseGatewayPlugin { ... } else { ... }` 双写代码 — 违反 §解耦原则，等于 god object 上再加一层 god if。

---

## 附录 A：边缘观察

1. **god object 双 RecordUsage 重复**：`GatewayService.RecordUsage / RecordUsageWithLongContext` 与 `OpenAIGatewayService.RecordUsage` 两份近乎平行的实现 — 阶段一重构应当合并到 `recordUsageCore` 单点（已经是入口，但 OpenAI 走的是另一份 `OpenAIRecordUsageInput → openai_gw_svc.go:5143`）。
2. **`OnUpstreamAccepted` 闭包**：跨进程不可序列化，必须改为信号 RPC。仅在 Anthropic Forward 用；OpenAI / Antigravity 不用。
3. **antigravity 的 single-account-retry**：`gateway_handler.go:312-319` 在 `apiKey.GroupID` 是单 antigravity 账号时设 ctx 标记 — plugin 化后 ctx 标记必须随 ParsedRequest 显式传 plugin。
4. **`forcePlatform` middleware**：现在通过 `request.Context` 传值（`middleware.ForcePlatform`），plugin 化后改用 `EndpointDecl.RequiredAccountPlatform` 显式声明，避免 plugin 读 ctx 私有 key。
5. **`useMixed` 计算**：`gateway_service.go:2279` 在 host SelectAccount 内决定，plugin 不感知；这是正确的解耦。
6. **`PluginEventPublisher`**：已有 hook（`AntigravityGatewayService.eventPublisher`），plugin 化后整体替换为 `EventsExtension.Subscribe(gateway.model.invoked)`。

## 附录 B：error path 调用图（不在主表里）

仅列代价高 / 跨进程必须考虑的 error path 调用，hot path 不会触发：

- **Failover loop**（`gateway_handler.go:776-796` SwitchCount 累加）：每次失败重新进 SelectAccount。host 端 SelectAccount 已支持 `excludedIDs` 过滤集合。
- **Same-account retry**（`openai_gateway_handler.go:347-368`）：池模式同账号重试 N 次后才切换；plugin 化后由 plugin 自己控制 `RetryableOnSameAccount` 状态。
- **Signature error retry**（`gateway_service.go:4574-4700`）：thinking block 签名错误后剥离 thinking 重发；plugin 内部行为，不跨进程。
- **`shouldRectifySignatureError`**：调 `settingService.IsSignatureRectifierEnabled` — 走 host SettingService RPC（已有 `HostSettings`）。
- **Beta policy block**（`gateway_service.go:4521-4534`）：由 plugin 实现，host 不感知。
- **PromptTooLongError fallback group**（`gateway_handler.go:801-878`）：跨 group 重试 + ticket 替换 — 这是 handler 层逻辑，plugin 化后留在 owner plugin 内（不跨进程）。
- **Gemini single-account exhausted**（`gateway_handler.go:329-341`）：handler 层 failover state 机；plugin 化后留 owner plugin。
- **`ClearRateLimit` / `ResetOpenAI403Counter`**：admin 路径，与 gateway hot path 无关；plugin 化后走 admin RPC（已有 `accounts.refresh` 类）。
