# Design

## Context

动机见 proposal.md，行为契约见 specs/。与方案直接相关的现状（2026-10-16 核实，行号以 main `b0a02a330` 为准，实现时以符号名为准）：

- **上游协议**（Codex CLI 源码 `codex-rs/protocol`、`codex-api`，加实测）：目录 `GET /backend-api/codex/models?client_version=…` 每个模型带 `service_tiers[].id`、`default_service_tier`（实测恒为 null）、`available_access_programs.cyber`。请求体顶层 `service_tier`（Fast 线上值 `priority`、`ultrafast`、`flex`）与 `access_programs:{"cyber":…}`，HTTP 与 WS `response.create` 同构。Codex TUI 只在目录广告某 tier 时发送该 tier（flex 例外，`openai_models.rs` `service_tier_for_request`），只在目录广告任一 program 时发送 `access_programs`（`tui/src/daybreak.rs` `program_for_turn`），Daybreak 关闭时每轮显式发 `standard`；core 层 `cyber_access_program::for_auth` 本身不查目录，所以 app-server / exec 等非 TUI 客户端可能不受目录约束直接发送。访问项目拒绝是 HTTP 200 首帧 `event: error`，`error.param="access_programs.cyber"`，`error.code` 为 `access_program_not_enabled` 或 `invalid_access_program`；无权限账号的 `ultrafast` 被静默接受不报错；`flex` 返回 400 `{"detail":"Unsupported service_tier: flex"}`。能力按模型分布：Blue 账号在 gpt-6-sol / luna / 5.6 系列带 `daybreak_blue` 但 gpt-6.1-sol / astra 不带；`ultrafast` 只在 gpt-6.1-sol / astra 上出现。完整流实测（gpt-6.1-sol，基线 / priority / ultrafast 各两次、两个账号）：`response.completed.service_tier` 恒为 `default`，包括有权限账号的真实 Ultrafast；`usage`、`response.metadata`、全部响应头在变体间无差异。唯一差异是有权限的 Ultrafast 响应缺少 `x-codex-turn-state` 头，原因不明，只能当启发式而非契约。因此响应侧无法回读实际层级，路由过滤是 Ultrafast 的唯一保护。Codex 客户端按 `[daybreak_blue, daybreak_red]` 顺序取目录中第一个命中的等级发送。
- **影子账号**：用户确认 Spark 影子账号已是无操作的过时功能，本设计不为其做任何处理。
- **现用调度路径**（`selectAccountWithSchedulerOnce`，`openai_account_scheduler.go`，`scheduler == nil` 分支）按顺序有四个出口：
  1. `selectLegacyAccountByPreviousResponse`：按 `previous_response_id` 查到账号后直接 `tryAcquireAccountSlot` 抢槽，再用 `isAccountRequestCompatibleReason`（高级调度器的辅助函数）复核，不兼容则 `ReleaseFunc()` 放弃；
  2. guardian-parent 分支：构造临时 `defaultOpenAIAccountScheduler` 调用其 `selectBySessionHash`；
  3. `selectAccountWithLoadAwareness`（`openai_gateway_scheduling.go`）：粘性命中与候选循环；
  4. 回退 `selectAccountForModelWithExclusionsStickyHit` → `tryStickySessionHit` / `selectBestAccount`。
  出口 3、4 及其新鲜度复检 `resolveFreshSchedulableOpenAIAccountBeforeProfit` / `recheckSelectedOpenAIAccountFromDBBeforeProfit` 都汇聚到 `openAICompatibleAccountEligibilityFailureReasonBeforeProfit`，该函数以位置参数接收 `requestedModel`、`requireCompact`、`requiredCapability`，同时服务 OpenAI、Grok 等 OpenAI 兼容平台；出口 1、2 不经过它。`selectBestAccount` 把复检失败统一记为 `ineligible`。选号失败以 `openAINoAvailableSelectionError{message}` 包装 `ErrNoAvailableAccounts`，details 只供日志。
- **候选集**来自 Redis 快照投影，`Extra` 只有 `repository/scheduler_cache.go` `filterSchedulerExtra` 白名单内的键可见；`schedulerNeutralExtraKeys` 内的键更新不触发快照重建。管理端 `UpdateAccount`（`admin_account.go`）用前端提交的 `extra` 整体替换，只保留一份硬编码受管键列表。高级调度器默认关闭且计划清理，本设计不修改其内部实现。
- **入口**：`handler/openai_gateway_handler.go` Responses（校验 `service_tier`，再选号）、WS（首帧模型白名单、后续帧 `response.create` 校验、首帧选号）、Messages；`handler/openai_chat_completions.go`。Chat Completions 入口已接受 Responses 形态请求体并原样转发。OpenAI Messages 兼容路径的 Fast 来自 `anthropic-beta: fast-mode` 头（`openai_gateway_messages.go` 3b 注入 `priority`），转换层 `AnthropicRequest → ResponsesRequest` 不携带请求体 `service_tier`（2026-10-16 实现时核实并修正本句）。`SelectAccountWithSchedulerForCapability` 是唯一公开选号入口，签名以变参 `platformOverride ...string` 结尾。
- **tier 改写在选号之后**：`openai_gateway_forward.go`、`applyOpenAIFastPolicyToBody`、`applyOpenAIFastPolicyToWSResponseCreate` 在拿到账号后执行分组强制 Fast（无条件改写为 `priority`，含客户端发 `ultrafast` 的情况）→ 全局 Fast Policy（按账号类型 scope）。
- **错误呈现**：`handler/no_account_error.go` 的 `classifyNoAccountError` 不消费选号错误，靠 `DiagnoseModelAvailabilityForPlatform` 重查池子判定 404 `model_not_found`（配置类）或 503；`classifySelectionFailureError` 只解析 `rate_limited=N`。Messages 入口不经过后者，WS 入口不经过分类器。
- **Manifest**：`handler/openai_codex_models_handler.go` 接受 OpenAI 与 Composite 分组，三条路径：固定账号（`FetchPinnedCodexModelsManifest` → `MergeGroupConfiguredCodexModels`）、本地生成（`BuildGroupConfiguredCodexModelsManifest` 自行计算 ETag 后直接写响应，不经 Merge）、调度拉取（`FetchCodexModelsManifest` → `ApplyPinnedCodexModelsMapping` → Merge）。`MergeGroupConfiguredCodexModels` 对非 OpenAI 分组直接返回，且只在 `changed || retired` 时写回 body。本地模板 `configuredCodexServiceTiersForModel` 为 gpt-5.6-sol 硬编码 `ultrafast`。`FetchCodexModelsManifest` 按账号 60 秒新鲜、5 分钟乐观返回旧值的单飞内存缓存；真正上游请求是 `fetchCodexModelsManifestUpstream`。公开别名映射在 Merge 之前执行，body 中的 slug 可能是别名而非上游 slug。
- **账号元数据**：`accounts.extra` JSONB，`accountRepo.UpdateExtra` 原子合并并按键决定是否入 outbox。现有 `upstream_model_metadata`（`UpstreamModelMetadataExtraKey`）由管理端「同步上游模型」手动写入，对 OAuth 账号同样来自 Codex 目录并按模型保留 `service_tiers`，但不含 cyber 项目、无自动刷新、manifest 侧跨账号取交集（`intersectUpstreamModelMetadata`）。
- **分组配置样板**：`CodexModelsManifestConfig` 使用 domain 类型直通 DTO，链路为 ent → 迁移 → `group_repo` → `api_key_repo` 投影 → 认证快照（本变更前为版本 25） → DTO/mapper → `admin_group.go` → 复制。`UpdateGroup` 的输入字段为可选指针。`GroupsView.vue` 6900 行，`components/admin/group/` 已有抽出的字段组件；账号详情内联在 `views/admin/AccountsView.vue`。
- **后台服务启动**：`cmd/server/wire.go` 维护一个按名启动的后台服务列表（含 `OpenAICodexVersionSyncService`），是一次性与周期任务的统一挂载点。

## Goals / Non-Goals

**Goals:**
- 能力推导、需求解析、白名单判定、manifest 投影都是纯函数，可独立单测。
- 能力谓词只实现一次，并覆盖现用调度路径的全部四个出口。
- 需求传递是显式的、编译期可见的，不依赖隐式上下文；对不经谓词的出口做显式后置校验。
- 新增的持久化键在候选阶段即可见，不重蹈 `openai_capabilities` 只能在抢槽后拦截的覆辄。
- 分组配置复用 `CodexModelsManifestConfig` 的单类型直通链路，不复制 `ModelAllowlist` 的三份同形类型。

**Non-Goals:**
- 不改动计费；不基于响应的 `service_tier` 回显或任何响应头做层级回读（实测 `response.completed` 恒为 `default`，无法区分）。
- 不为 service tier 构造上游错误识别（上游不报错）。
- 不在调度路径 manifest 中并入其他账号独有的模型（如 `gpt-daybreak-blue-latest`）；需要时用固定账号模式。
- 不修改高级调度器内部实现（`defaultOpenAIAccountScheduler` 的方法与 `OpenAIAccountScheduleRequest`）；不把全局 Fast Policy 前移到选号前。
- 不做定时探测。

## Decisions

### D1：能力记录存于 `accounts.extra["codex_capabilities"]`，按 slug 保存并派生账号级集合
类型 `domain.CodexAccountCapabilities{Models map[string]CodexModelCapabilities{ServiceTiers []string; CyberPrograms []string}; ServiceTiers []string; CyberPrograms []string; SyncedAt time.Time}`，JSON 键 `models`、`service_tiers`、`cyber_programs`、`synced_at`。推导函数 `DeriveCodexAccountCapabilities(models []byte) (CodexAccountCapabilities, bool)` 以上游目录 body 为输入，返回 ok=false 表示空列表或解析失败。
- 按 slug 保存是为 manifest 投影（D7）：若按账号级标志投影，客户端会在 gpt-6.1-sol 上发送 Blue，上游直接拒绝。路由只用派生的账号级集合，符合用户取舍 1、2。
- 独立新键而非扩展 `upstream_model_metadata`：后者是手动触发、无自动刷新、不含 cyber、跨账号取交集的模型工具能力快照；本键自动刷新、含 cyber、按并集使用。两者语义不同，合并会让现有 `intersectUpstreamModelMetadata` 的消费者行为漂移。
- 新键加入 `filterSchedulerExtra` 白名单，不进 `schedulerNeutralExtraKeys`；同时加入 `admin_account.go` `UpdateAccount` 的受管键保留列表，否则管理员编辑账号名称、并发等字段时会被前端提交的旧 `extra` 整体覆盖抹掉。该键只能由探测写入：创建、编辑、`UpdateAccountExtra`、批量更新与复制账号五条管理端写路径都丢弃客户端提交的值（伪造记录可绕过能力路由，并让部署后补齐把账号当作已探测而跳过）。
- 读取 getter `(*Account).CodexCapabilities() *CodexAccountCapabilities`，缺失返回 nil，调用方以 nil 为「无能力」。
- 备选：新增 `accounts` 专用列。否决：现有账号学习型元数据全部在 `extra`，专用列需要迁移且无查询需求。

### D2：探测服务单一实现，直连上游绕过 manifest 缓存
新文件 `openai_codex_capability_probe.go`，类型 `CodexCapabilityProbe`，方法：
- `Probe(ctx, account) error`：用规范客户端版本（`CodexCanonicalClientVersion()`）直接调用 `fetchCodexModelsManifestUpstream`（其入参 `openAIModelsRequest` 的构造目前内嵌在 `FetchCodexModelsManifest` 中，需先抽成 `buildCodexModelsManifestRequest(ctx, account, clientVersion)` 供两处共用）（不经 `fetchCachedOpenAIModels`，因为后者 60 秒新鲜直接返回、5 分钟内乐观返回旧值，权益刚变化时探测会读到旧目录），成功后回填该账号对应的 manifest 缓存条目；调用 D1 推导，与现有记录深比较（模型级与账号级），变化时 `accountRepo.UpdateExtra` 并 `mergeAccountExtra` 更新内存副本。带 15 秒上游超时。
- `ProbeAsync(accountID int64, reason string)`：`context.Background()` 加超时，按账号单飞，供创建后、流内拒绝等不可阻塞路径使用。
- `RecordFromManifest(ctx, account, clientVersion string, body []byte)`：供 `FetchCodexModelsManifest` 真实上游命中（非缓存命中）与 `SyncUpstreamModelCatalog` 已拿到 body 时顺手回写，避免重复请求。只在 `clientVersion` 不低于规范版本时生效：目录按 `minimal_client_version` 过滤模型，旧版本客户端拉到的目录模型更少，直接回写会把按 slug 记录缩水；更新的客户端拿到的是超集，可以回写。
- 重探测冷却：进程内 `map[accountID]time.Time`，默认 10 分钟，只记录流内拒绝触发的重探测；常规探测（创建 / 刷新 / 测试 / 补齐）不参与冷却，否则一次常规探测会把随后十分钟的拒绝重探测压掉。重探测失败不回滚冷却：上游不可达时拒绝持续出现，逐请求重试只会放大请求量。因为 `Probe` 直连上游，冷却只防重复请求，不会出现「读旧缓存后锁十分钟」。
- 触发点：`adminServiceImpl.CreateAccount` 持久化后对 OpenAI OAuth-like 账号 `ProbeAsync`（覆盖 OAuth 流程与 Codex 导入）；`ApplyOAuthCredentials` 与 `UpdateAccount` 检测到 OAuth-like 账号凭据变更后 `ProbeAsync`（重新授权可能换了底层 ChatGPT 账号）；`TokenRefreshService.postRefreshActions` 与 `ensureOpenAIPrivacy` 并列同步调用 `Probe`；`testOpenAIAccountConnection` 调用 `Probe`；`SyncUpstreamModelCatalog` 调用 `RecordFromManifest`；启动后一次性补齐（D9）。
- 账号类型范围：探测与谓词（D3）同为 `IsOpenAIOAuthLike()`（oauth 与 setup_token），`FetchCodexModelsManifest` 的 OAuth 分支相应放宽到 OAuth-like。若实现时验证 setup_token 凭据无法调用目录接口，则两处同时收窄为 `IsOpenAIOAuth()`，setup_token 账号不参与过滤。
  - 实现记录（2026-10-16）：范围以单一谓词 `isCodexCapabilityProbeAccount` 收口（探测、`FetchCodexModelsManifest` OAuth 分支、调度谓词 `AppliesTo`、manifest 投影来源、以及 `upstream_models.go` 「同步上游模型」的 OpenAI OAuth 分支共用；后者原按 `IsOpenAIOAuth()` 分流，setup_token 会落到 API Key 的 `/v1/models` 路径而拿不到 Codex 目录，已对齐），收窄只需改这一处。setup_token 凭据能否调用目录接口**未验证**，保持 OAuth-like 默认；验证为不可调用时按本节回退。
- 备选：在 `OpenAITokenProvider.GetAccessToken` 的按需刷新后挂钩。否决：热路径，且后台刷新器已覆盖所有活跃账号。

### D3：需求以显式结构体参数穿过现用调度链，并对不经谓词的出口做显式后置校验
类型 `OpenAIAccountRequirement{CyberProgram string; ServiceTier string}`（零值表示无需求），方法 `IsZero()`、`AppliesTo(account) bool`（`IsOpenAIOAuthLike()`）、`SatisfiedBy(account) (ok bool, reason string)`，reason 为 `daybreak_not_supported:<program>` / `service_tier_not_supported:<tier>`。
- 谓词只对 OAuth-like 账号生效：API Key 账号（`priority` 是其原生合法参数、计费按响应 tier 回读）、Grok 等共用该谓词的平台账号没有探测来源，若按「无记录即无能力」会被永久排除。它们直接放行，由上游处理。
- 在 `openAICompatibleAccountEligibilityFailureReasonBeforeProfit` 的 `capability_mismatch` 之后增加一段判断，覆盖出口 3、4 及新鲜度复检。
- 出口 1（`selectLegacyAccountByPreviousResponse`）：在既有 `isAccountRequestCompatibleReason` 复核旁增加 `requirement.SatisfiedBy(account)`，不满足则同样 `ReleaseFunc()` 并返回未命中，落入后续出口。
- 出口 2（guardian-parent）：对 `selectBySessionHash` 返回的 selection 做同样后置校验，不满足则调用其 `ReleaseFunc` 并继续后续出口。不修改 `OpenAIAccountScheduleRequest` 与高级调度器方法。
- 新参数加在 `SelectAccountWithSchedulerForCapability` 的 `useUpstreamTokenCost` 之后、变参之前，并沿 `selectAccountWithScheduler` → `selectAccountWithSchedulerOnce` → `selectLegacyAccountByPreviousResponse` / `selectAccountWithLoadAwareness` / `selectAccountForModelWithExclusionsStickyHit` → `tryStickySessionHit` / `selectBestAccount` / `resolveFreshSchedulableOpenAIAccountBeforeProfit` / `recheckSelectedOpenAIAccountFromDBBeforeProfit` → 谓词显式传递。图片、token 计数、manifest 拉取等其他调用方传零值。编译期暴露只覆盖被改签名的函数，出口 1、2 靠本节的显式后置校验与回归测试覆盖。
- `selectBestAccount` 的复检失败改为记录谓词返回的 reason 而非统一 `ineligible`，以便过滤统计与 D6 的分类能看到能力原因。
- 备选：像 composite route / privacy / profit gate 一样放进 `context`。否决：漏注入时静默等于「无需求」，会把 Ultrafast 请求静默路由到无权限账号并按 6 倍计费，正是本变更要消灭的故障。
- 备选：把整条位置参数链重构成请求结构体。否决：属于旧引擎清理的范围，不混入本变更。

### D4：需求解析与白名单准入在 handler 一次完成，位于选号之前
新文件 `openai_capability_requirement.go`：
- `ParseOpenAIAccountRequirement(body []byte, group *Group) (requested, routing OpenAIAccountRequirement)`：读取顶层 `access_programs.cyber` 与 `service_tier`，归一化复用 `normalizeOpenAIServiceTier`（`fast`→`priority`；`flex`/`auto`/`default`/`scale`→无需求）；`standard` 与未知 program 不构成需求（未知值交给上游报错）。`requested` 是客户端原始需求，用于白名单准入；`routing` 是分组改写后的选号需求：`group.ForceOpenAIFast` 时 tier 折叠为 `priority`（与转发层无条件改写为 `priority` 的行为一致，不保留 `ultrafast`），否则等于 `requested`。
- 准入判定使用 `(*Group).CapabilityAllows(requested) (blocked string)`（定义在 D5 的 `group_capability_allowlist.go`）：白名单开启时返回被禁用的能力名，空串表示放行。先准入再折叠，符合用户确认的「白名单 → force → policy」顺序。
- 调用点：Responses HTTP 在 `ValidateOpenAIServiceTierField` 之后；WS 首帧与后续帧紧随模型白名单校验之后（后续帧只做准入，不重选）；Chat Completions 与 Messages 同样解析两个字段（Chat Completions 已接受 Responses 形态请求体，Anthropic 形态请求体没有 `access_programs` 时自然为空）。凡允许向 OAuth 上游转发 `access_programs` 的入口都必须解析它，避免准入与路由被绕过。
  - 实现修正（2026-10-16）：Messages 入口的转换层（`AnthropicRequest → ResponsesRequest`）不携带请求体 `service_tier`，该路径上唯一到达上游的 tier 是 `anthropic-beta: fast-mode` 头注入的 `priority`（`openai_gateway_messages.go` 3b）。上文 Context 中「Messages 兼容路径的 Fast 以请求体 `service_tier` 为准、不读 anthropic-beta 头」的描述与代码不符。为使准入与路由与实际转发一致，Messages 入口使用 `ParseOpenAIMessagesAccountRequirement(body, anthropicBeta, group)`：请求体 `service_tier` 不构成需求，fast-mode 头构成 Fast 需求，`access_programs` 照常解析（并经 `AnthropicRequest.AccessPrograms` 原样转发）。Ultrafast 在该路径无法到达上游，因此也不构成需求。
  - 实现记录：强制 Fast 分组下选号需求无条件折叠为 `priority`（含客户端未带 tier 的请求），与转发层无条件改写一致；后果是强制 Fast 分组内所有请求都只落到能力记录含 `priority` 的 OAuth-like 账号（API Key 账号不受影响），部署后依赖 D9 补齐；如需收窄为「仅客户端带 tier 时折叠」，改 `foldOpenAIRoutingRequirement` 一处即可。
- 全局 Fast Policy 保留在选号之后。接受的后果：`filter` 动作可能剥掉已据以路由的 tier（无害，只是多筛了一轮）；`force_priority` 规则会给未按 Fast 路由的请求加上 `priority`。后者只影响 Fast，而 Fast 在 ChatGPT 订阅账号上普遍存在，暂不处理，记入 Risks。
- 备选：把 Fast Policy 前移。否决：policy 按账号类型 scope，账号未定时无法求值。

### D5：白名单取值与「standard 始终允许」
`domain.GroupCapabilityAllowlist{Enabled bool; CyberPrograms []string; ServiceTiers []string}`，JSON 键 `enabled`、`cyber_programs`、`service_tiers`，列 `groups.capability_allowlist` JSONB NOT NULL DEFAULT '{}'，迁移 `249_group_capability_allowlist.sql`，ent `field.JSON(...).Default(domain.GroupCapabilityAllowlist{})`。
- 合法值：`cyber_programs ⊆ {daybreak_blue, daybreak_red}`，`service_tiers ⊆ {priority, ultrafast}`。`standard` 不入配置：它表示「没有项目」，禁用它等于禁用所有已入项目用户的普通请求，没有合理场景。
- 校验在 `admin_group.go` 创建与更新路径：新文件 `group_capability_allowlist.go` 提供 `normalizeGroupCapabilityAllowlist(platform, cfg)`（非 openai/composite 归零、去重、小写）与 `validateGroupCapabilityAllowlist(cfg, forceFast bool)`（未知值 400 `INVALID_CAPABILITY_ALLOWLIST`；开启且不含 `priority` 却 `ForceOpenAIFast` 时 400 并说明冲突）。更新路径先把可选输入合并到 `group` 再对合并后的 `CapabilityAllowlist` 与 `ForceOpenAIFast` 无条件校验，避免两次独立请求绕过冲突检查。
- domain 类型直通 DTO 与认证快照（版本 25 → 26），不建 service 侧同形类型。
- 备选：把 Fast 相关控制并入现有全局 Fast Policy 规则。否决：那是全局按账号类型的策略，不是分组维度，且不覆盖 Daybreak。

### D6：拒绝与无可用账号以类型化错误表达，统一为 404 配置类错误
- 白名单拒绝：handler 直接返回 404，OpenAI 格式 `type: invalid_request_error`、`code: capability_not_allowed`，消息形如 `Capability "ultrafast" is not allowed for this group`；标记 `MarkOpsClientBusinessLimited(LocalPolicyDenied)` 与 ingress reject 原因 `capability_not_allowed`。WS 以 `StatusPolicyViolation` 关闭并用同一消息。
- 路由无账号：候选循环看到的集合在 SQL / 快照阶段已经排除了账号级限流、过载、临时不可调度的账号，还会再去掉 failover 的 `excludedIDs`，所以循环内「具备能力的账号数为零」不能证明分组没有能力账号。判定分两步：候选循环统计满足需求的账号数为零时，再用忽略临时状态的持久池查询（现有 `accountRepo.ListModelAvailabilityCandidates`，与 `classifyNoAccountError` 的诊断同源）对池内每个账号套用与选号完全相同的 `requirement.SatisfiedBy(account)`（含 `AppliesTo` 豁免：API Key 等不受过滤的账号天然满足），确认没有任何账号满足完整需求后才返回类型化错误 `OpenAICapabilityUnavailableError{Requirement}`（`Unwrap()` 到 `ErrNoAvailableAccounts` 以保持既有 failover 与日志语义）；持久池里存在具备能力的账号则按既有临时不可用分类（429 / 503）返回。该查询只在选号失败路径执行，不进入正常调度热路径。HTTP 入口（Responses、Chat Completions、Messages）用 `errors.As` 识别后返回 404，`code: capability_not_available`，消息 `No account in this group supports capability "daybreak_blue"`；不命中则沿用既有 `classifyNoAccountError` 路径。WS 首帧选号发生在连接升级之后，无法再返回 HTTP 状态码：识别到该错误时先向客户端写一条 Responses 风格的 `error` 事件（`code: capability_not_available`，同一消息），再以 `StatusPolicyViolation` 关闭连接；临时不可用沿用既有 WS 关闭路径。
- 选 404 而非 503：能力缺失是权益或配置问题，重试不会成功；503 会触发客户端退避重试与反向代理健康告警。沿用「没有可用账号」的 404 配置类分类，但用独立 `code` 而非 `model_not_found`，避免客户端提示「模型不可用」。
- 备选：在 `classifyNoAccountError` 解析原因字符串。否决：该函数刻意不消费选号错误，details 只供日志；`selectBestAccount` 也会把原因折叠。

### D7：Manifest 投影是写响应前、ETag 前的一步纯函数，三条路径显式调用
新文件 `openai_codex_manifest_capability_projection.go`：
- `ProjectCodexManifestCapabilities(body []byte, sources []CodexCapabilitySource, allowlist GroupCapabilityAllowlist) (out []byte, changed bool)`，其中 `CodexCapabilitySource{Caps CodexAccountCapabilities; UpstreamSlugOf func(publicSlug string) string}` 每个账号一个，`UpstreamSlugOf` 用该账号自身的模型映射（`ResolveMappedModel`）把 body 中的公开别名解析回上游 slug，因为同一公开别名在不同账号上可以映射到不同上游模型：
  1. 并集：对 body 中每个模型，逐账号用其 `UpstreamSlugOf` 解析上游 slug 并查该账号记录，若任一账号命中，则 `service_tiers` 与 `available_access_programs.cyber` **替换**为命中账号记录的并集；条目名称与描述优先沿用 body 原有条目，缺失时用 `configuredCodexServiceTier` 内置描述。所有记录都不含该 slug 时保留原值。不把 body 原值并入：body 可能来自 5 分钟内的旧缓存或硬编码模板，并入会复活已撤销能力；拉取账号刚获得的能力由同一次拉取的 `RecordFromManifest` 写入记录，无需靠 body 兜底。
  2. 裁剪：白名单开启时无条件执行，与 records 是否为空无关；剔除被禁用的 Daybreak 等级与 tier，`standard` 保留。
- 来源：OpenAI 分组用 `accountRepo.ListSchedulableByGroupID` 过滤 OAuth-like，逐账号构造 `CodexCapabilitySource`，一次分组查询；Composite 分组没有直属账号，只执行步骤 2 裁剪，不做并集，spec 据此表述。
- 接入点：handler 在三条路径写响应之前各自显式调用一个共用的后处理函数（投影 + ETag 重算 + 304 判断），而不是依赖 `MergeGroupConfiguredCodexModels`：本地生成路径不经 Merge，Merge 对非 OpenAI 分组提前返回且只在 `changed || retired` 时写回。同时从 `BuildGroupConfiguredCodexModelsManifest` 与 Merge 内部移除既有的 ETag/304 判定，避免它们在投影前按旧 ETag 返回 `NotModified` 且 `Body` 为空。
- 备选：在 `FetchCodexModelsManifest` 缓存层投影。否决：缓存键不含分组，投影是分组维度操作，必须在缓存体克隆上进行。

### D8：流内拒绝的识别与挂钩
- 检测函数 `isCodexAccessProgramRejection(payload []byte) bool`：事件 `error`（或 OAuth 路径改写后的 `response.failed`）且 `error.param == "access_programs.cyber"`（兼容顶层 `param`）。不匹配 `code`，两种 code 都表示不允许，且上游可能新增。
- 统一触发函数 `(*OpenAIGatewayService).observeCodexCapabilityRejection(account, payload)`，内部判定并 `ProbeAsync`。挂钩点以现有 `detectOpenAICyberPolicy` 的调用位置为模板，覆盖所有检查 OAuth 上游错误事件的地方：`openai_gateway_response_handling.go` 原生 SSE 与非流式聚合、`openai_gateway_passthrough.go` 流式与 SSE→JSON、`openai_gateway_chat_completions.go`、`openai_gateway_messages.go`、`openai_ws_v2_passthrough_adapter.go` 的 `BeforeWriteClient` 分支、`openai_ws_forwarder_ingress.go` 各模式、`openai_ws_http_bridge.go`、`openai_ws_forwarder_v2.go`。不在 `openai_ws_v2/passthrough_relay.go` 挂钩：它是独立传输子包，不持有账号或服务，且 adapter 已在同一事件上回调。
- 不改变事件透传、既有 bare error → `response.failed` 改写、failover 判定与账号副作用；测试用实测完整错误体断言「不 failover、不进账号惩罚副作用、账号状态不变」。

### D8b：`access_programs` 必须在所有 OAuth 转发路径存活
`/responses` 原始透传走黑名单变换，`access_programs` 可通过；但 compact 请求体按字段白名单重建（`openai_gateway_request_body.go` compact 白名单），chat → responses、messages → responses 等类型化重建也会丢字段。本变更把 `access_programs` 加入 compact 白名单，并审计 OAuth 路径上所有重建请求体的位置，确保已据以路由的字段到达上游；与 D4「所有入口都解析该字段」配套，避免「按 Blue 选号、转发时丢掉 Blue」或「转发了 Blue 却没按 Blue 准入」。

### D9：启动后一次性补齐
`CodexCapabilityProbe.BackfillMissing(ctx)` 沿用 `OpenAICodexVersionSyncService` 的机制：在 service 层 wire 的 Provide 函数中 `Start()`，并在 `cmd/server/wire.go` 的按名清理列表注册 `Stop()`；启动后执行一次：列出无记录的 OpenAI OAuth-like 账号，按每秒 2 个的节奏串行 `Probe`，失败只记日志。原因：部署后若等 token 刷新周期，期间所有 Daybreak / Ultrafast 请求都会报无可用账号。

### D10：前端拆组件
- `components/admin/group/CapabilityAllowlistField.vue`：props `config`、`platform`，emit `update:config`；总开关 + 两组多选（Daybreak：Blue、Red；Service Tier：Fast、Ultrafast）。创建与编辑表单都接入，平台切换到非 openai/composite 时清空。文案提示：同一模型同时广告 Blue 与 Red 时 Codex 客户端只会发 Blue。
- 账号详情：`views/admin/AccountsView.vue` 详情区块读取 `extra.codex_capabilities` 渲染徽标与同步时间，缺失显示「未检测」；若该区块需要拆出组件，先抽再加。
- 类型 `CapabilityAllowlist`、`CodexAccountCapabilities` 加入 `types/index.ts`；i18n 键 `admin.groups.capabilityAllowlist.*`、`admin.accounts.codexCapabilities.*`。

## Risks / Trade-offs

- [创建后探测失败，账号在下次 token 刷新前无记录，Daybreak / Ultrafast 请求报无可用账号] → 创建响应中不阻塞，但管理端账号详情显示「未检测」并可用「同步上游模型」手动触发；D9 补齐任务也会覆盖。
- [强制 Fast 分组下选号需求无条件折叠为 `priority`（含客户端未带 tier 的请求），纯 OAuth 分组在 D9 补齐完成前所有请求都要求能力记录含 `priority`] → 与转发层无条件改写一致，是有意为之；发布说明注明；分组内 API Key 账号不受影响；如需收窄为「仅客户端带 tier 时折叠」改 `foldOpenAIRoutingRequirement` 一处。
- [全局 Fast Policy `force_priority` 在选号后加 `priority`，未按 Fast 路由] → Fast 在订阅账号上普遍存在，影响极小；记录为已知限制，如出现实际问题再把 `force_priority` 规则存在性并入需求解析。
- [快照投影新增键使 `UpdateExtra` 触发快照重建] → 仅变化时写入，稳态零写入。
- [固定账号模式能力字段从首账号原值改为并集，运营者可能依赖旧行为] → proposal 标注 BREAKING；并集只会增加客户端可见能力，路由侧有过滤兜底。
- [同一 slug 同时广告 Blue 与 Red 时，Codex 客户端按顺序只发 Blue，Red 账号的 Daybreak 不可达] → UI 文案提示；需要 Red 时用白名单禁用 Blue。
- [WS 连接中途开启 Daybreak 时上游拒绝直达客户端] → 用户明确选择透传；Codex CLI 自身对该错误有提示文案。
- [能力谓词只接入现用调度链，若运营者开启「OpenAI 实验调度策略」，需求不会被评估] → 该路径已计划清理，不接入；在该设置的界面文案中注明不支持能力路由。
- [重探测冷却是进程内状态，多实例各自冷却] → 代价是最坏每实例每窗口一次目录请求，可接受。
- [Composite 分组的 manifest 只裁剪不并集] → 准入与路由仍兜底；Composite 下需要广告 Daybreak / Ultrafast 的场景由子分组直连解决。
- [`gpt-daybreak-blue-latest` 等项目独有模型在调度路径 manifest 中不出现] → Non-Goal，文档建议需要时使用固定账号模式并选入 Daybreak 账号。
- [setup_token 账号若无法调用目录接口] → D2 已定义回退：两处同时收窄为 `IsOpenAIOAuth()`。

## Migration Plan

1. 合并 `codex-manifest-pinned-accounts` 后再合并本变更。
2. 迁移 `249` 新增列，默认 `{}`，旧行为不变（白名单关闭）。
3. 部署后 D9 补齐任务填充能力记录；在补齐完成前带能力需求的请求可能报无可用账号，发布说明中注明。
4. 回滚：回退代码即可，列与 `extra` 新键无害残留。
