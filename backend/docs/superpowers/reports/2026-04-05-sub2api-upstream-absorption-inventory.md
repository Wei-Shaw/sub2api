# sub2api Upstream Absorption Inventory

## Baseline

- HEAD: `883c9355793eb2d5c19d27b948d174f694037464`
- origin/main: `f585a15eff28a36d86262ab67009a8502690040c`
- merge-base: `b384570de3545f036f250e68e9ca31362142dadf`
- branch status: `main...origin/main [ahead 23, behind 70]`

## Overlap

```text
backend/cmd/server/wire_gen.go
backend/ent/migrate/schema.go
backend/ent/mutation.go
backend/ent/runtime/runtime.go
backend/ent/schema/usage_log.go
backend/ent/usagelog.go
backend/ent/usagelog/usagelog.go
backend/ent/usagelog/where.go
backend/ent/usagelog_create.go
backend/ent/usagelog_update.go
backend/internal/handler/admin/usage_handler.go
backend/internal/handler/dto/mappers.go
backend/internal/handler/dto/types.go
backend/internal/handler/gateway_handler.go
backend/internal/handler/openai_chat_completions.go
backend/internal/handler/openai_gateway_handler.go
backend/internal/pkg/usagestats/usage_log_types.go
backend/internal/repository/usage_log_repo.go
backend/internal/repository/usage_log_repo_request_type_test.go
backend/internal/server/routes/admin.go
backend/internal/service/gateway_service.go
backend/internal/service/openai_gateway_record_usage_test.go
backend/internal/service/openai_gateway_service.go
backend/internal/service/openai_model_mapping.go
backend/internal/service/openai_model_mapping_test.go
backend/internal/service/openai_ws_forwarder.go
backend/internal/service/openai_ws_protocol_forward_test.go
backend/internal/service/usage_log.go
backend/internal/service/usage_log_helpers.go
backend/internal/service/wire.go
frontend/src/components/admin/usage/UsageFilters.vue
frontend/src/components/admin/usage/UsageTable.vue
frontend/src/i18n/locales/en.ts
frontend/src/i18n/locales/zh.ts
frontend/src/types/index.ts
frontend/src/views/admin/UsageView.vue
frontend/src/views/user/UsageView.vue
```

## Local Only

```text
backend/docs/superpowers/plans/2026-04-03-sub2api-openai-ungrouped-effective-platform.md
backend/docs/superpowers/plans/2026-04-03-sub2api-target-group-routing.md
backend/docs/superpowers/plans/2026-04-04-local-workbench-directory-reorg.md
backend/docs/superpowers/plans/2026-04-04-openai-hit-group-and-fast-billing.md
backend/docs/superpowers/plans/2026-04-04-opencode-fast-variant-rename.md
backend/docs/superpowers/plans/2026-04-04-opencode-fast-variant.md
backend/docs/superpowers/plans/2026-04-04-opencode-openai-metadata-mirror.md
backend/docs/superpowers/plans/2026-04-04-sub2api-openai-error-semantics-and-instructions.md
backend/docs/superpowers/plans/2026-04-04-sub2api-openai-routing-observability.md
backend/docs/superpowers/plans/2026-04-04-sub2api-opencode-custom-provider.md
backend/docs/superpowers/specs/2026-04-03-sub2api-openai-ungrouped-effective-platform-design.md
backend/docs/superpowers/specs/2026-04-03-sub2api-target-group-routing-design.md
backend/docs/superpowers/specs/2026-04-04-local-workbench-directory-reorg-design.md
backend/docs/superpowers/specs/2026-04-04-openai-hit-group-and-fast-billing-design.md
backend/docs/superpowers/specs/2026-04-04-opencode-fast-variant-design.md
backend/docs/superpowers/specs/2026-04-04-opencode-fast-variant-rename-design.md
backend/docs/superpowers/specs/2026-04-04-opencode-openai-metadata-mirror-design.md
backend/docs/superpowers/specs/2026-04-04-sub2api-openai-error-semantics-and-instructions-design.md
backend/docs/superpowers/specs/2026-04-04-sub2api-openai-routing-observability-design.md
backend/docs/superpowers/specs/2026-04-04-sub2api-opencode-custom-provider-design.md
backend/internal/handler/admin/admin_helpers_test.go
backend/internal/handler/admin/ops_dashboard_handler.go
backend/internal/handler/admin/ops_handler.go
backend/internal/handler/admin/setting_handler.go
backend/internal/handler/api_key_handler.go
backend/internal/handler/dto/account_runtime_state_test.go
backend/internal/handler/dto/mappers_usage_test.go
backend/internal/handler/dto/settings.go
backend/internal/handler/gateway_handler_models_test.go
backend/internal/handler/openai_gateway_handler_test.go
backend/internal/handler/ops_error_logger.go
backend/internal/handler/ops_error_logger_test.go
backend/internal/pkg/ctxkey/ctxkey.go
backend/internal/repository/ops_repo.go
backend/internal/repository/ops_repo_openai_routing_stats.go
backend/internal/repository/ops_repo_openai_routing_stats_test.go
backend/internal/repository/ops_repo_request_details.go
backend/internal/repository/ops_repo_request_details_test.go
backend/internal/server/api_contract_test.go
backend/internal/server/middleware/middleware.go
backend/internal/server/middleware/misc_coverage_test.go
backend/internal/server/routes/gateway.go
backend/internal/server/routes/gateway_test.go
backend/internal/server/routes/user.go
backend/internal/service/account.go
backend/internal/service/account_target_group_test.go
backend/internal/service/domain_constants.go
backend/internal/service/error_passthrough_runtime_test.go
backend/internal/service/gateway_anthropic_apikey_passthrough_test.go
backend/internal/service/openai_account_scheduler.go
backend/internal/service/openai_account_scheduler_test.go
backend/internal/service/openai_codex_transform.go
backend/internal/service/openai_codex_transform_test.go
backend/internal/service/openai_gateway_service_codex_cli_only_test.go
backend/internal/service/openai_gateway_service_test.go
backend/internal/service/openai_routing_observability.go
backend/internal/service/openai_routing_observability_test.go
backend/internal/service/openai_tool_continuation.go
backend/internal/service/openai_tool_continuation_test.go
backend/internal/service/openai_ws_account_sticky_test.go
backend/internal/service/opencode_openai_metadata.go
backend/internal/service/opencode_openai_metadata_test.go
backend/internal/service/ops_account_availability.go
backend/internal/service/ops_account_availability_test.go
backend/internal/service/ops_openai_routing_stats.go
backend/internal/service/ops_openai_routing_stats_test.go
backend/internal/service/ops_port.go
backend/internal/service/ops_repo_mock_test.go
backend/internal/service/ops_request_details.go
backend/internal/service/ops_request_details_test.go
backend/internal/service/ops_service_batch_test.go
backend/internal/service/setting_service.go
backend/internal/service/setting_service_update_test.go
backend/internal/service/settings_view.go
backend/migrations/082_add_openai_routing_observability.sql
backend/migrations/083_add_openai_billing_breakdown.sql
backend/migrations/084_recalculate_subscription_usage_from_actual_cost.sql
frontend/src/api/admin/ops.ts
frontend/src/api/admin/settings.ts
frontend/src/api/keys.ts
frontend/src/components/account/EditAccountModal.vue
frontend/src/components/account/__tests__/AccountStatusIndicator.spec.ts
frontend/src/components/account/__tests__/AccountUsageCell.spec.ts
frontend/src/components/keys/UseKeyModal.vue
frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
frontend/src/views/admin/SettingsView.vue
frontend/src/views/admin/ops/OpsDashboard.vue
frontend/src/views/admin/ops/components/OpsOpenAIRoutingCard.vue
frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue
frontend/src/views/admin/ops/components/__tests__/OpsOpenAIRoutingCard.spec.ts
frontend/src/views/admin/settings_ungrouped_openai_pool.spec.ts
frontend/src/views/admin/usage_routing_observability.spec.ts
frontend/src/views/user/__tests__/UsageView.spec.ts
```

## Remote Only

```text
backend/cmd/server/VERSION
backend/internal/handler/admin/channel_handler.go
backend/internal/handler/admin/channel_handler_test.go
backend/internal/handler/admin/dashboard_handler.go
backend/internal/handler/gateway_handler_chat_completions.go
backend/internal/handler/gateway_handler_responses.go
backend/internal/handler/gateway_handler_warmup_intercept_unit_test.go
backend/internal/handler/gemini_v1beta_handler.go
backend/internal/handler/handler.go
backend/internal/handler/sora_client_handler_test.go
backend/internal/handler/sora_gateway_handler.go
backend/internal/handler/sora_gateway_handler_test.go
backend/internal/handler/wire.go
backend/internal/pkg/antigravity/claude_types.go
backend/internal/pkg/antigravity/gemini_types.go
backend/internal/pkg/antigravity/response_transformer.go
backend/internal/pkg/antigravity/stream_transformer.go
backend/internal/repository/channel_repo.go
backend/internal/repository/channel_repo_pricing.go
backend/internal/repository/channel_repo_test.go
backend/internal/repository/wire.go
backend/internal/service/billing_service.go
backend/internal/service/channel.go
backend/internal/service/channel_service.go
backend/internal/service/channel_service_test.go
backend/internal/service/channel_test.go
backend/internal/service/gateway_channel_restriction_fallback_test.go
backend/internal/service/gateway_channel_restriction_test.go
backend/internal/service/gateway_hotpath_optimization_test.go
backend/internal/service/gateway_multiplatform_test.go
backend/internal/service/gateway_record_usage_test.go
backend/internal/service/gemini_messages_compat_service.go
backend/internal/service/model_pricing_resolver.go
backend/internal/service/model_pricing_resolver_test.go
backend/internal/service/openai_channel_restriction_test.go
backend/internal/service/openai_compat_prompt_cache_key.go
backend/internal/service/openai_gateway_chat_completions.go
backend/internal/service/openai_gateway_messages.go
backend/internal/service/ops_retry.go
backend/internal/service/pricing_service.go
backend/internal/service/testhelpers_test.go
backend/migrations/081_create_channels.sql
backend/migrations/082_refactor_channel_pricing.sql
backend/migrations/083_channel_model_mapping.sql
backend/migrations/084_channel_billing_model_source.sql
backend/migrations/085_channel_restrict_and_per_request_price.sql
backend/migrations/086_channel_platform_pricing.sql
backend/migrations/087_usage_log_billing_mode.sql
backend/migrations/088_channel_billing_model_source_channel_mapped.sql
backend/migrations/089_usage_log_image_output_tokens.sql
frontend/src/api/admin/channels.ts
frontend/src/api/admin/dashboard.ts
frontend/src/api/admin/index.ts
frontend/src/api/admin/usage.ts
frontend/src/components/admin/channel/IntervalRow.vue
frontend/src/components/admin/channel/ModelTagInput.vue
frontend/src/components/admin/channel/PricingEntryCard.vue
frontend/src/components/admin/channel/types.ts
frontend/src/components/charts/EndpointDistributionChart.vue
frontend/src/components/charts/GroupDistributionChart.vue
frontend/src/components/charts/ModelDistributionChart.vue
frontend/src/components/layout/AppSidebar.vue
frontend/src/router/index.ts
frontend/src/utils/formatters.ts
frontend/src/views/admin/ChannelsView.vue
```

## Local Baseline To Preserve

These local-only areas are current product/design baselines and should not be overwritten by upstream follow-up work:

- OpenAI active/exhausted routing and `-Sys` behavior
- Current usage/billing semantics:
  - priority-account multiplier
  - priority service-tier pricing source
  - user-visible billing-factor breakdown
- `sub2api-openai` independent OpenCode provider semantics and recommendation pipeline
- Existing local docs/specs/plans under `backend/docs/superpowers/**`

## Batch A — Low Coupling

These upstream additions are structurally independent enough that they can usually be absorbed mechanically after basic compile/test checks.

- `backend/cmd/server/VERSION`
  - Recommendation: mechanical absorb
- Channel domain core:
  - `backend/internal/repository/channel_repo.go`
  - `backend/internal/repository/channel_repo_pricing.go`
  - `backend/internal/repository/channel_repo_test.go`
  - `backend/internal/service/channel.go`
  - `backend/internal/service/channel_service.go`
  - `backend/internal/service/channel_service_test.go`
  - `backend/internal/service/channel_test.go`
  - Recommendation: mechanical absorb as a self-contained capability block
- Channel-adjacent independent tests/helpers:
  - `backend/internal/service/gateway_channel_restriction_fallback_test.go`
  - `backend/internal/service/gateway_channel_restriction_test.go`
  - `backend/internal/service/gateway_hotpath_optimization_test.go`
  - `backend/internal/service/gateway_multiplatform_test.go`
  - `backend/internal/service/openai_channel_restriction_test.go`
  - `backend/internal/service/testhelpers_test.go`
  - Recommendation: absorb together with channel block or defer as a test-only follow-up
- Platform-specific independent handlers/services:
  - `backend/internal/handler/gemini_v1beta_handler.go`
  - `backend/internal/handler/sora_client_handler_test.go`
  - `backend/internal/handler/sora_gateway_handler.go`
  - `backend/internal/handler/sora_gateway_handler_test.go`
  - `backend/internal/pkg/antigravity/claude_types.go`
  - `backend/internal/pkg/antigravity/gemini_types.go`
  - `backend/internal/pkg/antigravity/response_transformer.go`
  - `backend/internal/pkg/antigravity/stream_transformer.go`
  - `backend/internal/service/gemini_messages_compat_service.go`
  - `backend/internal/service/ops_retry.go`
  - Recommendation: **reclassified after compile check**. These files look independent, but in current local tree they already depend on newer gateway/service signatures. Defer them to Batch B/C instead of mechanical absorb.
- Independent channel migrations:
  - `backend/migrations/081_create_channels.sql`
  - `backend/migrations/082_refactor_channel_pricing.sql`
  - `backend/migrations/083_channel_model_mapping.sql`
  - Recommendation: absorb as a grouped migration batch
- Independent admin/channel UI and APIs:
  - `frontend/src/api/admin/channels.ts`
  - `frontend/src/components/admin/channel/IntervalRow.vue`
  - `frontend/src/components/admin/channel/ModelTagInput.vue`
  - `frontend/src/components/admin/channel/PricingEntryCard.vue`
  - `frontend/src/components/admin/channel/types.ts`
  - `frontend/src/views/admin/ChannelsView.vue`
  - Recommendation: mechanical absorb together with backend channel block
- Independent visualization/support UI:
  - `frontend/src/components/charts/EndpointDistributionChart.vue`
  - `frontend/src/components/charts/GroupDistributionChart.vue`
  - `frontend/src/components/charts/ModelDistributionChart.vue`
  - `frontend/src/utils/formatters.ts`
  - Recommendation: absorb if they do not touch local billing-display semantics

## Batch B — Medium Coupling

These changes mainly touch routing, wiring, DTO/API glue, or navigation layers. They can be absorbed, but not blindly.

- Backend handler/router wiring:
  - `backend/internal/handler/admin/channel_handler.go`
  - `backend/internal/handler/admin/channel_handler_test.go`
  - `backend/internal/handler/admin/dashboard_handler.go`
  - `backend/internal/handler/gateway_handler_chat_completions.go`
  - `backend/internal/handler/gateway_handler_responses.go`
  - `backend/internal/handler/gateway_handler_warmup_intercept_unit_test.go`
  - `backend/internal/handler/handler.go`
  - `backend/internal/handler/wire.go`
  - `backend/internal/repository/wire.go`
  - `backend/internal/server/routes/admin.go` (overlap)
  - `backend/internal/service/wire.go` (overlap)
  - `backend/cmd/server/wire_gen.go` (overlap)
  - Recommendation: manual absorb with route-order and constructor-signature checks
- Wiring support dependency discovered during implementation:
  - `backend/internal/service/model_pricing_resolver.go`
  - `backend/internal/service/model_pricing_resolver_test.go`
  - Recommendation: absorb only as a compile-enabling dependency for Batch B wiring; do not treat it as billing-semantics adoption.
- API/index/navigation glue:
  - `frontend/src/api/admin/dashboard.ts`
  - `frontend/src/api/admin/index.ts`
  - `frontend/src/components/layout/AppSidebar.vue`
  - `frontend/src/router/index.ts`
  - Recommendation: absorb after checking current local menu/order/product semantics
- Usage/admin glue with moderate coupling:
  - `backend/internal/handler/admin/usage_handler.go` (overlap)
  - `backend/internal/pkg/usagestats/usage_log_types.go` (overlap)
  - `frontend/src/components/admin/usage/UsageFilters.vue` (overlap)
  - `frontend/src/components/admin/usage/UsageTable.vue` (overlap)
  - `frontend/src/views/admin/UsageView.vue` (overlap)
  - Recommendation: do not mechanically copy; carry only non-conflicting glue after Batch C hotspot review

## Batch C — High-Risk Hotspots

These files sit directly in the same semantic band as the local routing/billing/opencode work. They require explicit per-file conflict analysis before any absorption.

- Billing/pricing service core:
  - `backend/internal/service/billing_service.go` (remote only)
  - `backend/internal/service/pricing_service.go` (remote only)
  - `backend/internal/service/model_pricing_resolver.go` (remote only)
  - `backend/internal/service/model_pricing_resolver_test.go` (remote only)
  - `backend/internal/service/gateway_service.go` (overlap)
  - `backend/internal/service/gateway_record_usage_test.go` (remote only)
  - `backend/internal/service/openai_gateway_record_usage_test.go` (overlap)
  - Recommendation: manual transplant only, preserve local billing semantics first
- OpenAI transport and request semantics:
  - `backend/internal/service/openai_gateway_service.go` (overlap)
  - `backend/internal/service/openai_model_mapping.go` (overlap)
  - `backend/internal/service/openai_model_mapping_test.go` (overlap)
  - `backend/internal/service/openai_ws_forwarder.go` (overlap)
  - `backend/internal/service/openai_ws_protocol_forward_test.go` (overlap)
  - `backend/internal/service/openai_compat_prompt_cache_key.go` (remote only)
  - `backend/internal/handler/gateway_handler.go` (overlap)
  - `backend/internal/handler/openai_gateway_handler.go` (overlap)
  - `backend/internal/handler/openai_chat_completions.go` (overlap)
  - Recommendation: manual transplant only, preserve local `-Sys` and active/exhausted semantics
- Usage persistence and schema band:
  - `backend/ent/schema/usage_log.go` (overlap)
  - `backend/ent/migrate/schema.go` (overlap)
  - `backend/ent/mutation.go` (overlap)
  - `backend/ent/runtime/runtime.go` (overlap)
  - `backend/ent/usagelog.go` (overlap)
  - `backend/ent/usagelog/usagelog.go` (overlap)
  - `backend/ent/usagelog/where.go` (overlap)
  - `backend/ent/usagelog_create.go` (overlap)
  - `backend/ent/usagelog_update.go` (overlap)
  - `backend/internal/repository/usage_log_repo.go` (overlap)
  - `backend/internal/repository/usage_log_repo_request_type_test.go` (overlap)
  - `backend/internal/service/usage_log.go` (overlap)
  - `backend/internal/service/usage_log_helpers.go` (overlap)
  - `backend/migrations/084_channel_billing_model_source.sql` (remote only)
  - `backend/migrations/085_channel_restrict_and_per_request_price.sql` (remote only)
  - `backend/migrations/086_channel_platform_pricing.sql` (remote only)
  - `backend/migrations/087_usage_log_billing_mode.sql` (remote only)
  - `backend/migrations/088_channel_billing_model_source_channel_mapped.sql` (remote only)
  - `backend/migrations/089_usage_log_image_output_tokens.sql` (remote only)
  - Recommendation: manual schema/SQL reconciliation only; do not absorb mechanically
- DTO and user/admin display semantics:
  - `backend/internal/handler/dto/types.go` (overlap)
  - `backend/internal/handler/dto/mappers.go` (overlap)
  - `frontend/src/types/index.ts` (overlap)
  - `frontend/src/i18n/locales/en.ts` (overlap)
  - `frontend/src/i18n/locales/zh.ts` (overlap)
  - `frontend/src/views/user/UsageView.vue` (overlap)
  - Recommendation: manual transplant only, preserve current local price-factor and explanation UX

### backend/internal/service/billing_service.go
- Local semantics to preserve:
  - `priority` 只代表单价来源，不再叠加额外 Fast 2x
  - 用户侧“标准计费 / 实际计费 / 单价来源”语义已经稳定
- Upstream additions:
  - 引入 channel 定价解析、按次/图片/渠道映射计费来源、统一 billing model 解析
- Conflict points:
  - 上游 `channel_mapped` / `BillingModel` 逻辑可能覆盖本地 `priority_account_multiplier + pricing_source` 解释体系
  - 上游图片/按次计费与本地 usage breakdown 字段都落在同一 cost 语义层
- Recommendation:
  - manual transplant only
  - 先抽出上游 `channel` 定价决策点，再映射进本地 `pricing_source` 体系

### backend/internal/service/gateway_service.go
- Local semantics to preserve:
  - 订阅用量按 `actual_cost` 累计
  - `priority_account_multiplier` 写库固化
  - 错误语义与 `instructions` 兼容修复已落地
- Upstream additions:
  - 统一计费入口、channel 限制前移、credits 预检查、prompt/cache 相关重构
- Conflict points:
  - RecordUsage / postUsageBilling / gateway 计费入口与本地 recent billing fixes 完全处于同一条执行链
  - 任何机械吸收都可能重新引入旧的订阅累计或错误包装问题
- Recommendation:
  - split before transplant
  - 先把上游 channel/credits 决策分离成独立 patch，再逐段合入

### backend/internal/service/openai_gateway_service.go
- Local semantics to preserve:
  - `active/exhausted` 目标组语义
  - `-Sys` continuation / role-based user item 识别
  - upstream structured error envelope 和 instructions 提升逻辑
- Upstream additions:
  - channel restriction、channel-mapped billing source、codex model mapping 调整、缓存和命名优化
- Conflict points:
  - 与本地 `-Sys` / passthrough / error semantics / billing source 多点重叠
  - 上游 channel 限制逻辑可能改变账号筛选与计费决策顺序
- Recommendation:
  - manual transplant only
  - 先按“选择账号前 / 选择账号后 / 写 usage 前”三个阶段拆开比对

### backend/internal/repository/usage_log_repo.go
- Local semantics to preserve:
  - usage log 已扩展 routing observability、billing breakdown、effective unit prices、pricing source
  - best-effort 和主插入路径都已补齐新列
- Upstream additions:
  - usage billing mode、image output tokens、channel billing source 相关持久化
- Conflict points:
  - 列顺序、扫描字段、insert arg 索引、migration 序列全部高风险
  - 很容易出现“代码编译通过但线上列值全是 NULL”这种隐性错位
- Recommendation:
  - manual schema/repo reconciliation only
  - 不做文本级 merge，按列清单重建一次最终 usage schema/insert/scan 设计

### backend/internal/handler/dto/types.go
- Local semantics to preserve:
  - 用户侧 usage DTO 已包含价格因子、实际单价、pricing source
  - 管理侧 routing/billing 字段已扩展
- Upstream additions:
  - usage billing mode、channel 相关 DTO 字段
- Conflict points:
  - 同一 DTO 上既有本地 billing factor，又有上游 billing mode/channel source，命名和展示责任容易混淆
- Recommendation:
  - manual transplant
  - 先列出“用户可见 vs 管理可见”字段边界后再吸收上游字段

### backend/internal/handler/dto/mappers.go
- Local semantics to preserve:
  - `usageLogFromServiceUser/Admin` 当前映射的是本地最新 billing 和 routing 语义
- Upstream additions:
  - usage billing mode / channel-related response mapping
- Conflict points:
  - mapper 很容易在无冲突文本下把字段口径默默切回上游旧语义
- Recommendation:
  - manual transplant
  - 逐个字段对照 service.UsageLog -> DTO 的来源与目标含义

### backend/ent/schema/usage_log.go
- Local semantics to preserve:
  - 已新增 routing observability 和 billing breakdown 相关字段
- Upstream additions:
  - usage billing mode、image output token 相关字段
- Conflict points:
  - 与本地 082/083/084 migration 直接重叠
  - ent regenerate 一旦走错顺序，会污染大量生成文件
- Recommendation:
  - defer direct merge
  - 先做 unified usage_log target schema，再统一生成 ent 和 migration 编号策略

### frontend/src/views/admin/UsageView.vue
- Local semantics to preserve:
  - admin usage 已显示 routing fields、模型链路、价格来源和新字段
- Upstream additions:
  - 三层模型映射显示、billing mode 展示、channel 相关 usage 列
- Conflict points:
  - UI 列、导出、过滤、tooltip 全部重叠
  - 上游新的 model mapping 展示很可能和本地 routing/billing 说明抢位置
- Recommendation:
  - manual transplant
  - 先确定“admin usage 页面最终信息架构”后再逐块搬上游 UI

### frontend/src/views/user/UsageView.vue
- Local semantics to preserve:
  - 用户已能看到标准费用、实际费用、所有倍率因子、实际单价、pricing source、`-Sys` 提示
- Upstream additions:
  - billing mode 相关展示、图片 token breakdown、共享 formatter
- Conflict points:
  - 同一 tooltip 和 summary card 上会同时竞争展示位
  - 容易把本地“价格原因解释”回退成更抽象的 billing mode 展示
- Recommendation:
  - manual transplant only
  - 只挑对当前用户解释有增益的上游片段，不整体替换页面

### frontend/src/types/index.ts
- Local semantics to preserve:
  - 用户/管理 usage 类型已经带上 routing + billing breakdown 字段
- Upstream additions:
  - channel、usage billing mode、dashboard 相关类型
- Conflict points:
  - 类型层最容易把“同名不同义”的字段并到一起
- Recommendation:
  - light manual absorb
  - 先定义保留字段，再补新增字段，避免覆盖本地口径

## Recommended Next Order

1. Batch A mechanical absorption shortlist
2. Batch B wiring absorption shortlist
3. Batch C per-file transplant plan

Current recommendation: do not merge origin/main directly; follow the inventory batch order above.

## Actual absorption result so far

### Batch A absorbed in current worktree

- `backend/cmd/server/VERSION`
- `backend/internal/repository/channel_repo.go`
- `backend/internal/repository/channel_repo_pricing.go`
- `backend/internal/repository/channel_repo_test.go`
- `backend/internal/service/channel.go`
- `backend/internal/service/channel_service.go`
- `backend/internal/service/channel_service_test.go`
- `backend/internal/service/channel_test.go`
- `backend/internal/service/gateway_channel_restriction_fallback_test.go`
- `backend/internal/service/gateway_channel_restriction_test.go`
- `backend/internal/service/openai_channel_restriction_test.go`
- `backend/internal/service/testhelpers_test.go`
- `backend/internal/pkg/antigravity/claude_types.go`
- `backend/internal/pkg/antigravity/gemini_types.go`
- `backend/internal/pkg/antigravity/response_transformer.go`
- `backend/internal/pkg/antigravity/stream_transformer.go`
- `backend/migrations/081_create_channels.sql`
- `backend/migrations/082_refactor_channel_pricing.sql`
- `backend/migrations/083_channel_model_mapping.sql`
- `frontend/src/api/admin/channels.ts`
- `frontend/src/api/admin/index.ts` (support glue absorbed early to keep ChannelsView compiling)
- `frontend/src/components/admin/channel/IntervalRow.vue`
- `frontend/src/components/admin/channel/ModelTagInput.vue`
- `frontend/src/components/admin/channel/PricingEntryCard.vue`
- `frontend/src/components/admin/channel/types.ts`
- `frontend/src/components/charts/EndpointDistributionChart.vue`
- `frontend/src/components/charts/GroupDistributionChart.vue`
- `frontend/src/components/charts/ModelDistributionChart.vue`
- `frontend/src/utils/formatters.ts`
- `frontend/src/views/admin/ChannelsView.vue`

### Batch A candidates deferred after hidden-coupling check

- `backend/internal/handler/gemini_v1beta_handler.go`
- `backend/internal/handler/sora_client_handler_test.go`
- `backend/internal/handler/sora_gateway_handler.go`
- `backend/internal/handler/sora_gateway_handler_test.go`
- `backend/internal/service/gemini_messages_compat_service.go`
- `backend/internal/service/ops_retry.go`
- `backend/internal/service/gateway_hotpath_optimization_test.go`
- `backend/internal/service/gateway_multiplatform_test.go`

Reason: they already assume newer local gateway/service signatures and therefore are not truly low-coupling in this branch state.

### Batch B absorbed in current worktree

- `backend/internal/handler/admin/channel_handler.go`
- `backend/internal/handler/admin/channel_handler_test.go`
- `backend/internal/handler/handler.go`
- `backend/internal/handler/wire.go`
- `backend/internal/repository/wire.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/service/wire.go`
- `backend/cmd/server/wire_gen.go`
- `backend/internal/service/model_pricing_resolver.go` (support dependency)
- `backend/internal/service/model_pricing_resolver_test.go` (support dependency)
- `frontend/src/api/admin/dashboard.ts`
- `frontend/src/components/layout/AppSidebar.vue`
- `frontend/src/router/index.ts`

### Batch B candidates still deferred

- `backend/internal/handler/admin/dashboard_handler.go`
- `backend/internal/handler/gateway_handler_chat_completions.go`
- `backend/internal/handler/gateway_handler_responses.go`
- `backend/internal/handler/gateway_handler_warmup_intercept_unit_test.go`

Reason: they still assume newer local `GatewayService` / `UserBreakdownDimension` interfaces and should be reconsidered in a later pass.

### Batch C absorbed so far

Stage 1 completed: `usage_log` target structure is now aligned to hold both local billing-breakdown fields and upstream channel/image-output fields.

Absorbed files in this stage:

- `backend/internal/service/usage_log.go`
- `backend/ent/schema/usage_log.go`
- `backend/internal/repository/usage_log_repo.go`
- `backend/internal/repository/usage_log_repo_request_type_test.go`
- `backend/migrations/090_add_usage_log_channel_and_image_output.sql`

Resulting schema/repository support now includes:

- local billing-breakdown fields preserved:
  - `priority_account_multiplier`
  - `effective_multiplier`
  - `effective_input_unit_price`
  - `effective_output_unit_price`
  - `effective_cache_read_unit_price`
  - `pricing_source`
- upstream channel/image-output fields added:
  - `channel_id`
  - `model_mapping_chain`
  - `billing_tier`
  - `billing_mode`
  - `image_output_tokens`
  - `image_output_cost`

Stage 2 completed: billing/pricing execution chain now persists channel/image-output concepts without overriding local pricing semantics.

Absorbed files in this stage:

- `backend/internal/service/usage_log_helpers.go`
- `backend/internal/service/billing_service.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/openai_gateway_record_usage_test.go`
- `backend/internal/service/gateway_record_usage_test.go`

Stage 3 completed: DTO and admin/user Usage display layers are now aligned enough to expose the new fields.

Absorbed files in this stage:

- `backend/internal/handler/dto/mappers.go`
- `backend/internal/handler/dto/mappers_usage_test.go`
- `frontend/src/types/index.ts`
- `frontend/src/views/user/UsageView.vue`
- `frontend/src/views/user/__tests__/UsageView.spec.ts`
- `frontend/src/components/admin/usage/UsageTable.vue`
- `frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
- `frontend/src/views/admin/UsageView.vue`
- `frontend/src/i18n/locales/en.ts`
- `frontend/src/i18n/locales/zh.ts`

Local verification after Stage 3 is green:

- `go test ./internal/service -count=1`
- `go test ./internal/repository -count=1`
- `go test ./internal/handler/dto -count=1`
- `pnpm test:run "src/views/user/__tests__/UsageView.spec.ts" "src/components/admin/usage/__tests__/UsageTable.spec.ts"`
- `pnpm build`
- `git diff --check`

### Batch C still pending

- Recheck whether `billing_tier` needs first-class persistence semantics or can remain reserved for later channel pricing source work.
- Admin/user stats and subscription-facing endpoints still need one final end-to-end rerun after Stage 3 to confirm all new fields surface consistently beyond the local regression suites.
- The inventory/report should be updated again after final verification to distinguish “structure absorbed” from “runtime verified on VPS”.
