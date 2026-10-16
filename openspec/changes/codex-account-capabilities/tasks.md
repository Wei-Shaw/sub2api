# Tasks

## 1. 账号能力记录与推导

- [x] 1.1 在 `backend/internal/domain` 新增 `CodexAccountCapabilities` / `CodexModelCapabilities` 类型（JSON 键 `models`、`service_tiers`、`cyber_programs`、`synced_at`），并在 service 层新增 `openai_codex_account_capabilities.go` 提供 `DeriveCodexAccountCapabilities(body) (caps, ok)` 与 `(*Account).CodexCapabilities()`（缺失返回 nil）；单测覆盖实测目录样本（Blue 账号、无项目账号、空列表返回 ok=false、解析失败）。
- [x] 1.2 在 `repository/scheduler_cache.go` 的 `filterSchedulerExtra` 白名单加入 `codex_capabilities`，确认未加入 `schedulerNeutralExtraKeys`；在 `admin_account.go` `UpdateAccount` 的受管键保留列表加入该键，并在创建 / 编辑 / `UpdateAccountExtra` / 批量更新 / 复制账号中丢弃客户端提交的该键；单测验证投影保留该键、`UpdateExtra` 该键会入 outbox、管理端编辑账号其他字段后记录仍在。
- [x] 1.3 从 `FetchCodexModelsManifest` 抽出 `buildCodexModelsManifestRequest(ctx, account, clientVersion)`（凭据解析、认证头、Originator / User-Agent / Version、代理）供两处共用；新增 `openai_codex_capability_probe.go`：`Probe`（用该构造函数直接调用 `fetchCodexModelsManifestUpstream` 并回填该账号的 manifest 缓存条目、用规范客户端版本、15 秒超时、推导、深比较、变化时 `UpdateExtra` + `mergeAccountExtra`）、`ProbeAsync`（后台 ctx、按账号单飞、10 分钟冷却）、`RecordFromManifest`（`clientVersion` 不低于规范版本才生效）；单测覆盖未变化不写、变化写入、空列表不覆盖、冷却窗口内跳过、manifest 缓存新鲜时 `Probe` 仍发出上游请求、旧版本 body 不回写而更新版本 body 回写。
- [ ] 1.4 在 `FetchCodexModelsManifest` 真实上游命中路径调用 `RecordFromManifest`（缓存命中不调用）；放宽其 OAuth 分支到 `IsOpenAIOAuthLike()` 并用 setup_token 账号验证目录接口可调用，不可调用则把 D2/D3 范围同时收窄为 `IsOpenAIOAuth()` 并在 design 中记录；单测验证规范版本的一次上游拉取恰好回写一次。

## 2. 探测触发点

- [x] 2.1 `adminServiceImpl.CreateAccount` 持久化后对 OpenAI OAuth-like 账号调用 `ProbeAsync`；`ApplyOAuthCredentials` 成功后与 `UpdateAccount` 检测到 OAuth-like 账号凭据变更后同样调用；单测用假探测器断言 OAuth 创建、重新授权、凭据变更触发，API Key 创建与仅改名称不触发，创建响应不等待探测。
- [x] 2.2 `TokenRefreshService.postRefreshActions` 对 OpenAI OAuth-like 账号调用 `Probe`（独立 15 秒超时，失败只记日志）；单测断言刷新成功后调用一次。
- [x] 2.3 `testOpenAIAccountConnection` 调用 `Probe`，`SyncUpstreamModelCatalog` 把已拿到的目录 body 交给 `RecordFromManifest`；单测断言账号测试后记录更新、同步后只发生一次上游请求。
- [x] 2.4 实现 `BackfillMissing`，在 service 层 wire 的 Provide 函数中 `Start()` 并在 `cmd/server/wire.go` 的清理列表注册 `Stop()`（与 `OpenAICodexVersionSyncService` 同机制；每秒 2 个、串行、失败记日志、只执行一次）；单测验证只处理无记录的 OpenAI OAuth-like 账号。
- [x] 2.5 新增 `isCodexAccessProgramRejection(payload)` 与 `observeCodexCapabilityRejection(account, payload)`，按 `detectOpenAICyberPolicy` 的调用位置为模板挂到所有检查 OAuth 上游错误事件的地方（`openai_gateway_response_handling.go` 原生 SSE 与非流式聚合、`openai_gateway_passthrough.go` 流式与 SSE→JSON、`openai_gateway_chat_completions.go`、`openai_gateway_messages.go`、`openai_ws_v2_passthrough_adapter.go`、`openai_ws_forwarder_ingress.go`、`openai_ws_http_bridge.go`、`openai_ws_forwarder_v2.go`），不挂 `openai_ws_v2/passthrough_relay.go`；单测用实测错误体（`access_program_not_enabled`、`invalid_access_program`）断言触发，用普通 error 事件断言不触发，并断言事件透传、bare error 改写、failover 判定与账号副作用均不变。

## 3. 请求需求解析与调度过滤

- [x] 3.1 新增 `openai_capability_requirement.go`：`OpenAIAccountRequirement` 类型（`IsZero`、`AppliesTo` 仅 OAuth-like、`SatisfiedBy` 返回 `daybreak_not_supported:<program>` / `service_tier_not_supported:<tier>`）与 `ParseOpenAIAccountRequirement(body, group) (requested, routing)`；表驱动单测覆盖 standard、daybreak_blue、daybreak_red、fast/priority、ultrafast、flex/auto/default/scale、缺省、ForceOpenAIFast 把 routing tier 折叠为 priority 而 requested 保留原值、Blue 与 Red 互不满足、nil 记录的 OAuth 账号不满足任何需求、API Key 账号与 Grok 账号始终满足。
- [x] 3.2 `openAICompatibleAccountEligibilityFailureReasonBeforeProfit` 增加需求谓词，并把 `OpenAIAccountRequirement` 作为显式参数穿过 `SelectAccountWithSchedulerForCapability` → `selectAccountWithScheduler` → `selectAccountWithSchedulerOnce` → `selectLegacyAccountByPreviousResponse` / `selectAccountWithLoadAwareness` / `selectAccountForModelWithExclusionsStickyHit` → `tryStickySessionHit` / `selectBestAccount` / `resolveFreshSchedulableOpenAIAccountBeforeProfit` / `recheckSelectedOpenAIAccountFromDBBeforeProfit`；`selectLegacyAccountByPreviousResponse` 与 guardian-parent 分支对返回的 selection 做后置校验，不满足则 `ReleaseFunc()` 并落入后续出口；`selectBestAccount` 的复检失败改记谓词 reason；其他调用方传零值；`go build ./...` 与现有调度测试通过。
- [x] 3.3 为四个出口分别补调度单测：`previous_response_id` 命中但账号不满足需求时释放槽位并继续选号、guardian-parent 命中同理、候选循环与粘性命中只选出具备能力的账号、`selectBestAccount` 回退同理、无需求时候选集不变、过滤统计出现带能力名的原因、API Key 账号不受影响。
- [x] 3.4 把 `access_programs` 加入 compact 请求体字段白名单（`openai_gateway_request_body.go` compact 白名单），并审计 OAuth 路径上所有重建请求体的位置（Chat Completions / Messages → Responses 转换、WS payload 重建）确保该字段存活；单测断言各路径转发体仍含 `access_programs`。
- [x] 3.5 新增类型化错误 `OpenAICapabilityUnavailableError{Requirement}`（`Unwrap` 到 `ErrNoAvailableAccounts`）：选号层在候选循环统计具备能力的账号数为零时，再用 `accountRepo.ListModelAvailabilityCandidates`（忽略临时状态）对池内每个账号套用同一 `requirement.SatisfiedBy`（含 API Key 等豁免类型天然满足），确认没有任何账号满足完整需求后才返回它；Responses HTTP、Chat Completions、Messages 用 `errors.As` 识别后返回 404 `capability_not_available` 并生成指明能力的消息，WS 首帧先写 `error` 事件再以 `StatusPolicyViolation` 关闭，其余沿用既有分类；单测覆盖「持久池无具备能力账号 → 404」「具备能力账号处于账号级限流未进候选集、另一无能力账号在候选集 → 既有临时不可用分类」「failover 排除了唯一能力账号 → 既有分类」「唯一满足需求的 API Key 账号被限流、池内另有无能力 OAuth 账号 → 既有分类」「Daybreak 与 Ultrafast 两个需求必须由同一账号满足」与四个入口。

## 4. 分组能力白名单

- [x] 4.1 新增 `domain.GroupCapabilityAllowlist`，ent schema `groups.capability_allowlist` JSON 字段，迁移 `249_group_capability_allowlist.sql`（`ADD COLUMN IF NOT EXISTS ... JSONB NOT NULL DEFAULT '{}'`），运行 ent 生成；迁移测试验证幂等。
- [x] 4.2 `service.Group` 新增字段；`group_repo` 创建与更新 setter、`api_key_repo` 字段投影（含 `GetByKeyForAuth` 的显式 Select 列表）与映射、认证快照结构体与 impl 两处映射并把版本升到 26、`admin_group_duplicate.go` 复制；单测覆盖快照往返与复制。
- [x] 4.3 新增 `group_capability_allowlist.go`：`normalizeGroupCapabilityAllowlist`、`validateGroupCapabilityAllowlist`（未知值 400 `INVALID_CAPABILITY_ALLOWLIST`；开启且不含 `priority` 却 `ForceOpenAIFast` 时 400）、`(*Group).CapabilityAllows(req) (blocked string)`；单测覆盖关闭放行一切、空集合全禁、standard 始终放行、非 openai/composite 归零、冲突校验。
- [x] 4.4 管理端 DTO：`CreateGroupRequest` / `UpdateGroupRequest`（指针）与 `AdminGroup` 响应新增 `capability_allowlist`，mapper 直通 domain 类型；`admin_group.go` 创建路径与更新路径接入归一化，更新路径在把可选输入合并到 `group` 之后对合并态无条件执行冲突校验；handler 测试覆盖创建、更新、未知值 400、先设白名单再单独开启强制 Fast 被拒绝。
- [x] 4.5 准入接入：Responses HTTP（紧随 `ValidateOpenAIServiceTierField`）、WS 首帧与后续帧（紧随模型白名单校验，后续帧只准入不重选）、Chat Completions、Messages 调用 `ParseOpenAIAccountRequirement` + `CapabilityAllows(requested)`，并把 `routing` 传入选号；HTTP 拒绝返回 404 `capability_not_allowed` 并标记 `LocalPolicyDenied` 与 ingress 原因 `capability_not_allowed`，WS 以 `StatusPolicyViolation` 关闭；handler 测试覆盖 HTTP 拒绝不发上游、WS 首帧与后续帧关闭、Chat Completions 的 Responses 形态 Daybreak 被同样准入、允许时正常选号并把需求传入。

## 5. Manifest 能力投影

- [x] 5.1 新增 `openai_codex_manifest_capability_projection.go`：`CodexCapabilitySource{Caps, UpstreamSlugOf}` 与 `ProjectCodexManifestCapabilities(body, sources, allowlist)`；单测覆盖并集替换（不保留 body 原值）、按 slug 隔离（gpt-6.1-sol 的 ultrafast 不扩散）、无记录 slug 保留原值、sources 为空时仍执行白名单裁剪、缓存旧能力不复活、本地模板硬编码 ultrafast 被记录覆盖、同一公开别名在两个账号映射到不同上游 slug 时按各自映射取并集、standard 保留、条目名称描述回填。
- [x] 5.2 抽出共用的 manifest 后处理函数（投影 + ETag 重算 + 304 判断），从 `BuildGroupConfiguredCodexModelsManifest` 与 `MergeGroupConfiguredCodexModels` 内部移除既有 ETag/304 判定，在 `openai_codex_models_handler.go` 的固定账号、本地生成、调度拉取三条路径写响应前显式调用；OpenAI 分组逐账号用 `ListSchedulableByGroupID` 过滤 OAuth-like 并以各自 `ResolveMappedModel` 构造 `CodexCapabilitySource`，Composite 分组只裁剪不并集；handler 测试覆盖三条路径、Composite 分组裁剪、固定账号路径（首账号无 Daybreak、第二账号有 Blue）、本地生成路径模板 ultrafast 被记录覆盖，并断言 ETag 随能力变化。

## 6. 前端

- [x] 6.1 `types/index.ts` 新增 `CapabilityAllowlist`、`CodexAccountCapabilities`，`AdminGroup` / `CreateGroupRequest` / `UpdateGroupRequest` 加 `capability_allowlist`；`tsc` 通过。
- [x] 6.2 新增 `components/admin/group/CapabilityAllowlistField.vue`（总开关 + Daybreak 多选 Blue/Red + Service Tier 多选 Fast/Ultrafast，附 Blue/Red 并存时客户端只发 Blue 的提示），接入 `GroupsView.vue` 创建与编辑表单，平台切换到非 openai/composite 时隐藏并清空；组件单测覆盖勾选产出配置与清空。
- [x] 6.3 `views/admin/AccountsView.vue` 账号详情展示 `extra.codex_capabilities` 徽标与同步时间，缺失显示未检测；单测覆盖有记录与无记录。
- [x] 6.4 中英文 i18n 新增 `admin.groups.capabilityAllowlist.*` 与 `admin.accounts.codexCapabilities.*`，并在「OpenAI 实验调度策略」设置文案注明该策略不支持能力路由；i18n 一致性测试通过。

## 7. 集成验证

- [x] 7.1 后端 `go build ./... && go test ./internal/service/... ./internal/handler/... ./internal/repository/...` 通过；前端 `pnpm test` 与 `pnpm build` 通过。
- [ ] 7.2 端到端手动验证（使用两份测试登录态）：无项目账号 + Blue 账号同组时，Codex CLI 从 sub2api 拿到的 manifest 在 gpt-6-sol 上显示 `daybreak_blue`、在 gpt-6.1-sol 上显示 `ultrafast`；开启 `/daybreak` 的请求只落到 Blue 账号；多轮会话携带 `previous_response_id` 中途开启 Daybreak 时切换到 Blue 账号；白名单禁用 Ultrafast 后 manifest 不再广告且直发 `ultrafast` 的请求返回 404；账号详情显示徽标。
