# Proposal

## Why

ChatGPT Login (Codex) 的 OAuth 账号在上游拥有不同的权益：Cyber 访问项目（Daybreak，分 Standard / Blue / Red）和模型服务层级（Fast 即线上值 `priority`、Ultrafast）。sub2api 目前完全不识别也不持久化这些能力，带 `access_programs.cyber` 或 `service_tier` 的请求会被调度到任意账号。实测结果：Daybreak 请求落到无权限账号时上游在 HTTP 200 的首帧 SSE 返回 `error`（`param=access_programs.cyber`）；Ultrafast 请求落到无权限账号时上游**不报错而是静默降级**，而本项目按请求 tier 计费，用户付 6 倍价格却拿不到 Ultrafast。运营者也没有任何手段限制某个分组可以使用哪些能力。

## What Changes

- **账号能力记录**：新增按账号持久化的 Codex 能力记录（按模型 slug 记录 `service_tiers` 与 `available_access_programs.cyber`，并派生账号级能力集合），由上游 `GET /backend-api/codex/models` 响应经单一纯函数推导。触发点：OAuth 账号创建后立即探测、后台 token 刷新后、账号测试、管理端「同步上游模型」、以及任何已拿到 manifest body 的路径顺手回写。值未变化不写库。
- **能力需求解析与路由**：在账号选择之前，从请求体解析出能力需求（Daybreak program、service tier），归一化规则与现有 `service_tier` 校验一致。`standard`、`flex`、`auto`、`default`、`scale` 不构成需求。需求随请求进入现用调度路径的全部出口（`previous_response_id` 命中、guardian 父账号命中、`selectAccountWithLoadAwareness`、粘性命中、`selectBestAccount` 及其新鲜度复检），OpenAI OAuth 类账号的能力记录缺失或不含对应能力即被排除，并给出命名过滤原因；API Key 账号与其他平台账号无探测来源，不受能力需求过滤。Blue 与 Red 独立精确匹配。分组开启强制 Fast 时选号需求的 tier 折叠为 Fast，与转发层改写一致。
- **错误触发的重新探测**：上游首帧 `error` 且 `param=access_programs.cyber` 时，对该账号异步触发一次能力重探测（按账号冷却窗口）。Service tier 没有可识别的拒绝信号，路由过滤是唯一保护，本提案明确记录这一点。
- **分组能力白名单**：OpenAI / Composite 分组新增能力白名单配置：允许的 Daybreak 等级（Standard / Blue / Red）与允许的 Service Tier（Fast / Ultrafast）。请求携带被禁用的能力时按「没有可用账号」类错误拒绝（404 配置类分类、独立错误码 `capability_not_allowed`，避免 503 诱发无意义重试）。分组内（忽略临时状态的持久池）没有任何具备所需能力的账号时同样返回 404（`capability_not_available`），WS 首帧则以错误事件加策略违规关闭表达；有能力账号但暂时不可用时沿用既有临时不可用分类。HTTP 入口与 WebSocket 每个 `response.create` 帧都执行，与分组模型白名单同位置。
- **Manifest 能力投影**：下发给 Codex 客户端的 manifest 中，每个模型的 `service_tiers` 与 `available_access_programs` 替换为分组内可调度 OAuth 账号能力记录的按 slug 并集（无记录的 slug 保留原值），再按分组白名单裁掉禁用值（裁剪不依赖记录是否存在）；三条 manifest 生成路径都经过同一后处理步骤。Composite 分组没有直属账号，只裁剪不并集。Codex TUI 只在 manifest 广告能力时才会发送对应字段（`/daybreak` 与 tier 选择都以目录为依据），没有投影则这些客户端的路由永远不会被触发；非 TUI 客户端可能直接发送，此时由路由与准入兜底。
- **管理端**：分组创建与编辑对话框新增能力白名单独立组件；账号详情展示已检测能力徽标。
- **BREAKING**：带 Daybreak / Ultrafast / Fast 需求的请求在分组内没有具备对应能力的账号时返回「没有可用账号」，不再被调度到任意账号。
- **BREAKING**（行为层面）：固定账号模式的 manifest 合并原先按 slug 首账号优先取整条记录，现在能力字段改为按 slug 并集。

## Capabilities

### New Capabilities
- `codex-account-capabilities`：账号能力记录的数据形态、推导规则、持久化与探测触发点，以及上游拒绝后的重新探测。
- `codex-capability-routing`：从请求解析能力需求、按账号能力过滤候选、命名过滤原因与无可用账号时的错误。
- `group-capability-allowlist`：分组能力白名单的数据模型、管理端校验与 UI、请求准入拒绝。
- `codex-manifest-capability-projection`：Codex manifest 中能力字段按分组账号并集与白名单投影。

### Modified Capabilities
<!-- openspec/specs 目前为空；codex-manifest-pinned-accounts 尚未归档，其 specs 不在主规格中，因此不以 Modified 形式引用。 -->

## Impact

- **数据库**：`groups` 新增 JSONB 列 `capability_allowlist`（迁移 `246_group_capability_allowlist.sql`）；账号能力记录存于 `accounts.extra` 新键，无迁移。
- **后端**：
  - 新文件：能力记录类型与推导（`openai_codex_account_capabilities.go`）、探测服务（`openai_codex_capability_probe.go`）、请求需求解析（`openai_capability_requirement.go`）、分组白名单（`group_capability_allowlist.go`）、manifest 投影（`openai_codex_manifest_capability_projection.go`）。
  - 调度：`openai_gateway_scheduling.go` 的 `openAICompatibleAccountEligibilityFailureReasonBeforeProfit` 增加能力谓词；`SelectAccountWithSchedulerForCapability` 调用链显式传递需求；`repository/scheduler_cache.go` 的 `filterSchedulerExtra` 白名单加入新键。
  - 入口：`handler/openai_gateway_handler.go`（Responses HTTP、WS 首帧与后续帧）、`handler/openai_chat_completions.go`、Messages 入口解析需求并执行分组白名单准入。
  - 流处理：所有检查 OAuth 上游错误事件的位置（原生 SSE、透传 SSE 与 SSE→JSON、Chat Completions / Messages 转换、WS passthrough adapter、WS ingress 各模式、HTTP→WS bridge、WS forwarder v2）挂重探测，清单见 design D8。
  - 探测触发：`admin_account.go` 的 `CreateAccount`、`token_refresh_service.go` 的 `postRefreshActions`、`account_test_service.go`、`upstream_models.go` 的 `SyncUpstreamModelCatalog`、`openai_codex_models_service.go` 的 `FetchCodexModelsManifest`。
  - 分组配置链路：ent schema → 迁移 → `group_repo` → `api_key_repo` 投影 → 认证快照版本 +1 → 管理端 DTO / mapper → `admin_group.go` 校验 → 分组复制。
- **管理端 API**：`POST/PUT /admin/groups` 请求与响应新增 `capability_allowlist`；`GET /admin/accounts/:id` 的 `extra` 已含新键，前端直接读取。
- **前端**：`components/admin/group/` 新增能力白名单组件并接入 `GroupsView.vue` 创建与编辑表单；账号详情徽标；`types/index.ts`；中英文 i18n。
- **依赖**：manifest 投影建立在 `codex-manifest-pinned-accounts` 变更引入的缓存与合并函数之上，该变更需先于本变更合并。
- **上游事实**（2026-10-16 实测）：无权限账号 catalog 的每个模型都带 `cyber:["standard"]`，Blue 账号在部分模型上带 `daybreak_blue` 并多出 `gpt-daybreak-blue-latest`；Ultrafast 只出现在特定模型的 `service_tiers`；`default_service_tier` 恒为 null；旧 `client_version` 返回空列表。
- **非目标**：不改动计费逻辑；不为 Service Tier 构造上游错误识别；不在调度路径的 manifest 中合并其他账号独有的模型（如 `gpt-daybreak-blue-latest`），需要该模型时使用固定账号模式；不触碰高级调度器（计划清理）。
