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

## Post-v0.1.108 Delta

After the original `f585a15e` baseline, upstream moved again to `339d906e` (tag line `v0.1.108`).

### Already absorbed from this newer delta

- `backend/cmd/server/VERSION`
- `README.md`
- `README_CN.md`
- `README_JA.md`
- `assets/partners/logos/ctok.png`
- `assets/partners/logos/poixe.png`

Reason: these are low-risk version/docs/assets updates and do not interfere with local routing, billing, or OpenCode semantics.

### New delta currently deferred

The remaining upstream changes since `f585a15e` are not low-risk mechanical absorbs. They are concentrated in three areas:

1. **Sora removal / cleanup line**
   - `backend/internal/handler/sora_client_handler.go`
   - `backend/internal/handler/sora_client_handler_test.go`
   - `backend/internal/handler/sora_gateway_handler.go`
   - `backend/internal/handler/sora_gateway_handler_test.go`
   - `backend/internal/server/routes/sora_client.go`
   - `backend/internal/service/sora_*`
   - `backend/migrations/090_drop_sora.sql`
   - `frontend/src/api/sora.ts`
   - `frontend/src/components/sora/*`
   - `frontend/src/views/user/SoraView.vue`

2. **Admin/settings/group/account restructuring**
   - `frontend/src/api/admin/settings.ts`
   - `frontend/src/views/admin/SettingsView.vue`
   - `frontend/src/components/layout/AppSidebar.vue`
   - `frontend/src/components/common/GroupBadge.vue`
   - `frontend/src/components/common/GroupOptionItem.vue`
   - `frontend/src/components/common/PlatformIcon.vue`
   - `frontend/src/components/common/PlatformTypeBadge.vue`
   - `backend/internal/handler/admin/account_data.go`
   - `backend/internal/handler/admin/account_handler.go`
   - `backend/internal/handler/admin/group_handler.go`
   - `backend/internal/handler/admin/openai_oauth_handler.go`
   - `backend/internal/handler/admin/setting_handler.go`
   - `backend/internal/handler/admin/user_handler.go`
   - `backend/internal/service/account_service.go`
   - `backend/internal/service/admin_service.go`
   - `backend/internal/service/api_key_auth_cache.go`
   - `backend/internal/service/api_key_auth_cache_impl.go`
   - `backend/internal/service/openai_oauth_service.go`
   - `backend/internal/service/token_*`

3. **Further overlap in current hotspots**
   - `backend/internal/service/billing_service.go`
   - `backend/internal/service/gateway_service.go`
   - `backend/internal/repository/usage_log_repo.go`
   - `backend/internal/handler/dto/{types.go,mappers.go}`
   - `backend/ent/schema/{group.go,usage_log.go,user.go}` plus generated ent files

### Recommendation for the post-v0.1.108 delta

- Keep the docs/version/assets changes already absorbed.
- Product decision is now explicit: local main should also remove Sora semantics.
- Current Sora-removal wave status:
  - core runtime Sora files are already removed from the worktree (`backend/internal/handler/sora_*`, `backend/internal/service/sora_*`, `backend/internal/server/routes/sora_client.go`, `frontend/src/api/sora.ts`, `frontend/src/components/sora/*`, `frontend/src/views/user/SoraView.vue`, `backend/internal/repository/sora_account_repo.go`, `backend/internal/repository/sora_generation_repo.go`)
  - compile chain is green after second-ring cleanup:
    - `go test ./internal/service -count=1`
    - `go test ./internal/handler -count=1`
    - `go test ./cmd/server -run TestNonExistent -count=1`
    - `pnpm build`
  - second-ring route/settings cleanup now also absorbed:
    - `backend/internal/server/routes/gateway.go` no longer registers `/sora/v1` or `/sora/media*`
    - `backend/internal/server/routes/gateway_test.go` no longer depends on `SoraGatewayHandler`
    - `backend/internal/server/router.go` no longer calls `RegisterSoraClientRoutes(...)`
    - `backend/internal/server/routes/admin.go` now matches upstream removal of Sora OAuth and Sora S3 admin settings routes
    - `backend/cmd/server/wire.go`, `backend/cmd/server/wire_gen.go`, and `backend/cmd/server/wire_gen_test.go` no longer build or clean up Sora-specific dependencies
    - `frontend/src/views/admin/SettingsView.vue` no longer exposes the Sora client toggle or the Sora-only data-management tab
    - `frontend/src/api/admin/settings.ts` no longer exposes Sora S3 management API methods/types
    - `frontend/src/views/admin/DataManagementView.vue` has been removed because it was entirely Sora S3 profile management UI with no remaining route or caller
  - product-surface cleanup is also now absorbed:
    - `backend/internal/service/setting_service.go` / `backend/internal/handler/admin/setting_handler.go` Sora S3 profile-management dead code removed
    - `backend/internal/handler/dto/{types.go,mappers.go}` and `frontend/src/types/index.ts` cleaned of visible Sora pricing/storage/client-toggle fields
    - `frontend/src/views/admin/GroupsView.vue` aligned to upstream no-Sora pricing UI
    - `frontend/src/views/admin/SettingsView.vue` / `frontend/src/api/admin/settings.ts` removed Sora client toggle and Sora S3 API surface
    - `frontend/src/views/admin/ChannelsView.vue`、`frontend/src/views/admin/SubscriptionsView.vue`、`frontend/src/views/admin/ProxiesView.vue`、`frontend/src/components/admin/channel/types.ts` aligned to upstream no-Sora branches
    - account-facing helpers aligned to no-Sora flow:
      - `frontend/src/api/admin/accounts.ts`
      - `frontend/src/composables/useModelWhitelist.ts`
      - `frontend/src/composables/useOpenAIOAuth.ts`
      - `frontend/src/composables/__tests__/useOpenAIOAuth.spec.ts`
      - `frontend/src/components/admin/account/AccountTableFilters.vue`
  - backend internal OAuth/endpoint cleanup is also fully aligned now:
    - `backend/internal/handler/admin/openai_oauth_handler.go`
    - `backend/internal/handler/endpoint.go`
    - `backend/internal/handler/endpoint_test.go`
    - `backend/internal/service/openai_token_provider.go`
    - `backend/internal/service/token_cache_invalidator.go`
    - `backend/internal/service/openai_oauth_service.go`
    - `backend/internal/service/openai_oauth_service_auth_url_test.go`
    - `backend/internal/pkg/openai/oauth.go`
    - `backend/internal/pkg/openai/oauth_test.go`
    - `backend/internal/domain/constants.go`
    - `backend/internal/service/domain_constants.go`
    - plus deletion of `backend/internal/service/openai_oauth_service_sora_session_test.go` and `backend/internal/service/account_test_service_sora_test.go`
  - deeper data-model cleanup has also moved forward:
    - `backend/ent/schema/group.go` and `backend/ent/schema/user.go` are aligned to upstream no-Sora definitions
    - `backend/internal/service/group.go`
    - `backend/internal/service/user.go`
    - `backend/internal/repository/group_repo.go`
    - `backend/internal/repository/api_key_repo.go`
    - `backend/internal/service/api_key_auth_cache.go`
    - `backend/internal/service/api_key_auth_cache_impl.go`
    - ent generated files were regenerated after schema cleanup and compile green is preserved
- Remaining Sora cleanup still pending in this delta:
  - runtime/frontend source tree is now effectively free of active `Sora/sora_` references; latest cleanup also removed lingering locale text, API contract fixture fields, and README-facing Sora product说明
  - remaining cleanup is mostly historical residue in docs, migrations, comments, and any optional deeper semantic/media terminology cleanup
  - optional deeper semantic cleanup of media fields in `gateway_service.go` / pricing paths if we want to remove every historical Sora-oriented concept rather than just unreachable behavior
  - legacy migrations/docs/tests mentioning removed Sora concepts
- Additional non-Sora upstream-follow progress after the Sora wave:
  - admin usage outer-ring filtering now also absorbs `billing_mode` from upstream while preserving local routing filters:
    - `backend/internal/pkg/usagestats/usage_log_types.go`
    - `backend/internal/handler/admin/usage_handler.go`
    - `backend/internal/repository/usage_log_repo.go`
    - `backend/internal/repository/usage_log_repo_request_type_test.go`
    - `frontend/src/api/admin/usage.ts`
    - `frontend/src/components/admin/usage/UsageFilters.vue`
    - `frontend/src/views/admin/UsageView.vue`
    - `frontend/src/views/admin/usage_routing_observability.spec.ts`
    - `frontend/src/i18n/locales/en.ts`
    - `frontend/src/i18n/locales/zh.ts`
  - this keeps local `routing_target_group / routing_schedule_layer` filters intact instead of adopting upstream’s narrower replacement.
  - admin dashboard user-breakdown parsing is also now aligned with upstream’s extra filter dimensions while preserving the current dashboard semantics:
    - `backend/internal/pkg/usagestats/usage_log_types.go`
    - `backend/internal/handler/admin/dashboard_handler.go`
    - `backend/internal/handler/admin/dashboard_handler_user_breakdown_test.go`
    - `backend/internal/repository/usage_log_repo.go`
  - newly supported breakdown filters:
    - `user_id`
    - `api_key_id`
    - `account_id`
    - `request_type`
    - `stream`
    - `billing_type`
  - the deeper `channel restriction / pricing resolver` chain has now started to align as well:
    - `backend/internal/service/gateway_service.go`
    - `backend/internal/service/openai_gateway_service.go`
    - `backend/internal/service/gateway_channel_restriction_test.go`
    - `backend/internal/service/gateway_channel_restriction_fallback_test.go`
    - `backend/internal/service/openai_channel_restriction_test.go`
    - `backend/internal/service/gateway_record_usage_test.go`
    - `backend/internal/service/openai_gateway_record_usage_test.go`
    - `backend/internal/service/openai_ws_protocol_forward_test.go`
    - `backend/internal/handler/gateway_handler_warmup_intercept_unit_test.go`
    - `backend/cmd/server/wire_gen.go`
  - currently absorbed semantics in this deep chain:
    - `GatewayService` and `OpenAIGatewayService` both carry `channelService` / `resolver`
    - gateway and openai account selection now perform channel pricing pre-checks before selection
    - sticky session and best-account selection now skip accounts whose upstream-mapped model is restricted when `BillingModelSource=upstream`
    - existing unit tests for `billingModelForRestriction` / `resolveAccountUpstreamModel` / fallback-group channel restriction and OpenAI channel restriction are green again
  - remaining risk in this hotspot is no longer basic compile survival, but the broader concept-by-concept absorb of upstream pricing-resolver use in the billing path and any later handler/admin exposure that depends on it
  - first deep `channel restriction / pricing resolver` semantics are now reintroduced locally:
    - `GatewayService` regains `channelService` / `resolver` fields and constructor wiring
    - `OpenAIGatewayService` regains `channelService` / `resolver` fields and constructor wiring
    - `GatewayService` now includes:
      - `ResolveChannelMappingAndRestrict`
      - `checkChannelPricingRestriction`
      - `billingModelForRestriction`
      - `resolveAccountUpstreamModel`
      - `isUpstreamModelRestrictedByChannel`
      - `needsUpstreamChannelRestrictionCheck`
    - `OpenAIGatewayService` now includes the matching pre-check / upstream-check helpers for channel restriction
    - fallback-group and OpenAI channel-restriction unit tests are green again, and `cmd/server` wiring has been updated so these new constructor dependencies compile in the main binary
  - the next remaining gap in this deep hotspot is no longer selection gating, but the heavier upstream pricing-resolver use in the billing path itself (for example the `CalculateCostUnified` / channel billing-mode override line), which should be absorbed concept-by-concept rather than by wholesale checkout
  - first billing-path concept from that resolver chain is now also landing:
    - `backend/internal/service/billing_service.go` has a local `CostInput` / `CalculateCostUnified` entry compatible with the current branch’s billing model
    - `GatewayService.RecordUsage` and `OpenAIGatewayService.RecordUsage` now route channel-priced requests through the unified billing entry when `resolver` and `groupID` are present
    - unit coverage now locks that channel token pricing can override the image billing branch without regressing the existing `ImageOutputTokens / BillingMode` persistence path
  - the next deeper `per_request / image tier` layer is now also starting to align:
    - `TestGatewayServiceRecordUsage_ChannelImagePricingUsesRequestTier` is green and proves image-mode channel pricing can use tier labels like `1K/2K/4K` through `CalculateCostUnified`
    - `TestOpenAIGatewayServiceRecordUsage_ChannelPerRequestPricingUsesContextTier` is green and proves OpenAI-side per-request channel pricing can select request tiers by context when `ImageCount/ImageSize` are not part of the request path
    - this means the unified billing entry is no longer only covering token-mode override, but has started to absorb the upstream `per_request / image` branch semantics too
    - direct billing-service unit coverage now also confirms these two branches at the core helper层 are green:
      - `TestCalculateCostUnified_PerRequestUsesRequestTierAndRateMultiplier`
      - `TestCalculateCostUnified_ImageFallsBackToDefaultPerRequestPrice`
    - 这说明 `CostInput / CalculateCostUnified` 本身已经能稳定承接 request-tier 价格选择，而不只是依赖外围 `RecordUsage` 路径间接验证
  - this still does **not** mean the entire upstream billing resolver chain is fully absorbed yet; it only establishes the first safe bridge from selection-side channel semantics into the billing path while keeping the local billing model intact
  - resolver source is now also persisted into `usage_log.BillingTier` on both Gateway/OpenAI usage write paths, so the first layer of pricing-source writeback is already connected
  - resolver-side token override absorption has also advanced one more small step:
    - `ModelPricingResolver.applyTokenOverrides` now copies `ChannelModelPricing.ImageOutputPrice` into `BasePricing.ImageOutputPricePerToken`
    - `TestResolve_WithChannelOverride_TokenImageOutputPrice` is green and proves this upstream token-mode image-output pricing branch is no longer missing locally
  - long-context billing absorption also moved one step closer to upstream:
    - `BillingService.CalculateCostWithLongContext` now keeps `ImageOutputTokens` on the in-range split, so image-output cost is not dropped when long-context pricing is applied
    - `TestCalculateCostWithLongContext_PreservesImageOutputCost` is green and locks that path
  - from a future "mark upstream as merged" perspective, the remaining `billing_service.go` diff is now better understood as three buckets rather than one monolith:
    - already-equivalent local rewrites: `CostInput` / `CalculateCostUnified` / `computeTokenBreakdown` / `calculatePerRequestCost` are carrying upstream intent in a local shape
    - local-shape-preserving divergences that should likely stay: current `PricingService` exposes `OutputCostPerImage` (not `OutputCostPerImageToken`), and local long-context results intentionally retain `BillingMode`
    - legacy upstream paths that probably should **not** be reintroduced mechanically: `GetModelPricingWithChannel` / `calculateCostInternal`, because local channel billing now flows through `resolver + CalculateCostUnified`
  - focused billing-model-source tests are now green as well:
    - `TestOpenAIGatewayServiceRecordUsage_BillsMappedRequestsUsingRequestedModel`
    - `TestOpenAIGatewayServiceRecordUsage_ChannelMappedBillingFallsBackToResultBillingModelWhenNotMapped`
    - `TestOpenAIGatewayServiceRecordUsage_ChannelMappedBillingUsesMappedModelWhenMapped`
    - together with the existing generic gateway test `TestGatewayServiceRecordUsage_ChannelTokenPricingOverridesImageBilling`
  - OpenAI-side channel restriction semantics are now also aligned at the same first-layer depth:
    - `OpenAIGatewayService` now performs channel pricing pre-checks before selection
    - sticky-session reuse and best-account selection now skip accounts whose upstream-mapped model is restricted when `BillingModelSource=upstream`
    - upstream unit tests for `ChannelMapped` early rejection, `Upstream` per-account restriction, and sticky-session fallback are green again
  - phase-two gateway scheduling absorption has now started as well:
    - `GatewayService.selectAccountForModelWithPlatform` and `GatewayService.selectAccountWithMixedScheduling` now both reapply `needsUpstreamChannelRestrictionCheck + isUpstreamModelRestrictedByChannel` in their main candidate loops
    - this restores the upstream-model channel restriction filter at the exact place where the final account is chosen, instead of leaving the helper chain disconnected
    - direct unit coverage is now green for both paths:
      - `TestSelectAccountForModelWithPlatform_SkipsUpstreamRestrictedAccounts`
      - `TestSelectAccountWithMixedScheduling_SkipsUpstreamRestrictedAccounts`
  - `OpenAIGatewayService.RecordUsage` has also recovered the low-conflict upstream nil-safety on default rate multiplier lookup:
    - when `s.cfg == nil`, it now falls back to `1.0` instead of dereferencing a nil config pointer
    - `TestOpenAIGatewayServiceRecordUsage_NilConfigDefaultsRateMultiplierToOne` is green and locks that behavior
  - the smaller Gemini tail has also been pulled back into line:
    - `GeminiV1BetaModels` again threads `ChannelUsageFields` into `RecordUsageWithLongContext`, so the channel mapping / billing-source chain is no longer disconnected on the Gemini native path
    - `extractGeminiUsage` once again captures `IMAGE` modality token counts from `candidatesTokensDetails`
    - handler / repository / service test suites are all green after this tail cleanup
  - `GatewayHandler.ChatCompletions` has now also caught back up on the account-slot glue used by the mature Anthropic entry path:
    - when `selection.Acquired == false`, it now restores account wait counting, queue-full rejection, and sticky-session binding after successful slot acquisition
    - this closes one of the clearest remaining handler-layer upstream glue gaps without changing the local product semantics above it
  - `openai_chat_completions.go` has also reconnected one missing handler-layer channel-mapping chain:
    - it now resolves channel mapping at the entry point, rewrites the forwarded body when a mapping applies, and threads `ChannelUsageFields` back into `RecordUsage`
    - this restores the end-to-end `channel_id / model_mapping_chain / billing_model_source` writeback path for the OpenAI chat-completions compatibility entry
  - `OpenAIGatewayHandler.Responses` now mirrors that same local channel-mapping glue on the main `/v1/responses` path:
    - after `prepareResponsesRequestForScheduling`, it now resolves channel mapping, uses the mapped model/body for scheduling and forward, and carries `ChannelUsageFields` into `RecordUsage`
    - this brings the local routing snapshot / usage writeback chain closer to a single consistent entry-path model instead of leaving `responses` as a partially disconnected special case
  - `GatewayHandler.Messages` has now regained the same channel-mapping parity on the native Anthropic entry path:
    - it resolves channel mapping before selection, rewrites the parsed/body model when a mapping applies, and threads `ChannelUsageFields` into both usage-recording call sites
    - this removes another remaining handler-level gap where `/v1/messages` could previously miss mapped-model selection and lose `channel_id / model_mapping_chain / billing_model_source` writeback
  - `GatewayHandler.Responses` now matches that same Anthropic-side channel-mapping parity:
    - it resolves channel mapping before account selection, rewrites the request body when a mapping applies, and restores `ChannelUsageFields` on the usage-recording path
    - this closes the last obvious gap among the Anthropic compatibility entry handlers where mapped-model selection and channel writeback could drift from the rest of the merged billing chain
  - the local `OpenAIGlobalPoolForUngroupedKeys` divergence has now been removed instead of preserved:
    - backend settings constants / DTOs / handlers / `SettingService` parsing and defaults no longer expose `openai_global_pool_for_ungrouped_keys`
    - `OpenAIGatewayService` no longer widens ungrouped OpenAI scheduling to the full platform pool, and `ResolveEffectivePlatform` no longer injects OpenAI effective platform for ungrouped keys on that basis
    - frontend admin settings API / SettingsView / dedicated spec / locale strings for that toggle have all been removed together, so the product now aligns back to `allow_ungrouped_key_scheduling` as the only remaining ungrouped-key control
  - separate from that removal, the local `active / exhausted / any` target-group semantics remain an explicit high-risk merge guardrail:
    - 审查前提必须设成：上游默认规则**会挡掉**我们需求里的 exhausted-group 账号，而不是默认假设它们天然安全
    - 所以后续任何 upstream absorption 只要碰到 quota checks、target-group helpers、sticky hit、candidate filtering、selection boundaries，都要先按“可能把 exhausted 账号挡掉”的风险来审，除非有明确证据证明不会破坏 `TargetGroupExhausted` 语义
  - the next hot-path merged pass has also started from the lowest-risk OpenAI helper boundary rather than the largest scheduler function:
    - `OpenAIGatewayService.SelectAccountWithLoadAwareness` now centralizes its Layer 1/2/3 `fresh + recheck + upstream channel restriction` gate through one local `prepareSelectedAccount` path
    - this does not roll back the local `TargetGroupAny/Active/Exhausted` design; it just makes the retained OpenAI freshness/recheck semantics explicit before touching larger selection branches
  - that same retained gate is now also shared by `selectBestAccount`, so the non-load-batch OpenAI main selection path no longer carries a parallel copy of the `fresh + recheck + upstream restriction` chain
  - `tryStickySessionHit` has now been folded into that same gate as well, while keeping sticky-specific cleanup / TTL refresh semantics outside the helper
  - `selectAccountForModelWithExclusions` no longer emits a separate bare-string “no available OpenAI accounts” branch; its miss path now rejoins `ErrNoAvailableAccounts`, so the OpenAI ordinary-selection入口和 load-aware 退化路径的失败语义更一致
  - the remaining non-guardrail divergence in OpenAI load-aware selection has also narrowed: Layer 2/3 candidate handling now goes back to upstream-style `resolveFreshSchedulableOpenAIAccount(..., TargetGroupAny)` plus in-place upstream restriction checks, instead of forcing every candidate through the extra `prepareSelectedOpenAIAccount` recheck path
  - merged-prep also exposed and fixed a trailing infrastructure mismatch: `cmd/server/wire_gen.go` now matches the current `NewOpenAIGatewayService` signature again, so `go build ./cmd/server` is no longer blocked by the old `settingService` argument
  - after switching from "热点链收口" to the real `behind -> 0` phase, the first narrow upstream tail has also been absorbed: `oauth_refresh_api.go` now again includes the upstream local-mutex + `invalid_grant` race recovery path and its matching tests, instead of staying on the earlier simplified refresh path
  - the next remote tail has also been pulled back in: OpenAI-compatible forwarding no longer forces API Key accounts through Codex/GPT model normalization, and the related `normalizeOpenAIModelForUpstream` split plus tests are back in place for chat/messages/ws entry paths
  - admin `status=active` filtering now again excludes currently rate-limited accounts in `account_repo.ListWithFilters`, matching the upstream fix instead of treating all `StatusActive` rows as equally active in the admin list
  - OpenAI non-streaming compatibility now again covers the upstream empty-output edge case: buffered chat responses accept `response.done`, and OAuth SSE-to-JSON will reconstruct `response.output` from accumulated delta events when the terminal payload arrives with an empty `output` array
  - the next merged batch has started to pull the billing writeback chain back toward upstream structure as well:
    - `GatewayService.RecordUsage` and `RecordUsageWithLongContext` are again thin wrappers over a shared `recordUsageCore`
    - the restored core still keeps local writeback semantics instead of reverting them, including unified resolver billing, `BillingTier` / `BillingMode`, `ChannelUsageFields`, and the prompt / long-context handling branches
    - `OpenAIGatewayService.RecordUsage` has now been pulled to the same shape as well: the entry is a thin wrapper over an OpenAI-specific shared writeback core, while preserving local routing snapshot, multiplier, effective unit price, `PricingSource`, `OpenAIWSMode`, and other OpenAI-only writeback fields
    - Anthropic compatibility entries on the OpenAI side are also back on that writeback chain: `GatewayHandler.ChatCompletions` once again applies channel mapping to the forwarded body and carries `ChannelUsageFields` into `RecordUsage`
    - the OpenAI writeback core now also aligns its effective unit prices and `PricingSource` with resolver-backed token pricing when `ResolvedPricing` is present, instead of always backfilling from bare `GetModelPricing`
    - `BillingService` has also started converging under that same batch: `CalculateCostUnified` 的无 resolver 回退与 `CalculateCostWithServiceTier` 现在不再各自维护一份 token 计费逻辑，而是共享同一个 `calculateCostWithPricing -> computeTokenBreakdown` core
    - along the same line, `CalculateCost` / `CalculateCostWithServiceTier` now also fold through a shared `calculateCostInternal`, and cache-creation cost has been split out of `computeTokenBreakdown` into `computeCacheCreationCost`
    - the first deeper semantic mismatch in `BillingService` has also been corrected: token-mode `ImageOutputPricePerToken` no longer reuses LiteLLM's per-image `output_cost_per_image`, so `ImageOutputTokens` fall back to ordinary output-token pricing unless a true image-token unit price source exists
    - OpenAI compatibility entries are also moving back toward upstream on the handler side: `Messages` and `ResponsesWebSocket` once again apply channel mapping to the forwarded body and carry `ChannelUsageFields` into `RecordUsage`, instead of dropping those links on the non-standard entry paths
  - `GatewayService.SelectAccountWithLoadAwareness` has also started the same style of request-level gate unification:
    - routed sticky, normal sticky, and Layer 2 candidate filtering now share one local account-selection reject path covering platform/mixed scheduling, model support, model limit, quota, window cost, RPM, and upstream restriction
    - wait-plan / slot-acquire / sticky binding outer shells are still intentionally left in place, so this remains a low-risk convergence step rather than a full scheduler rewrite
  - `selectAccountWithMixedScheduling` now follows that same convergence pattern on the legacy mixed path, while still keeping group membership checks on the sticky-cache lookup shell instead of incorrectly pushing them into the generic reject helper
  - `selectAccountForModelWithPlatform` now mirrors the same convergence on the single-platform legacy path, so routed sticky, normal sticky, and main candidate scanning no longer each carry their own copy of the same request-level reject chain
  - after targeted review, the first wave of concrete regressions has also been fixed in-place:
    - routed candidate filtering in `GatewayService.SelectAccountWithLoadAwareness` now checks upstream restriction just like routed sticky, eliminating that branch mismatch
    - legacy mixed routed sticky again requires group membership, and `RequirePrivacySet` side effects are no longer evaluated before the basic `isAccountSchedulableForSelection` precheck in single-platform / mixed candidate loops
    - OpenAI sticky handling now only clears the binding for `recheck` / upstream-restricted hard invalidation, instead of deleting sticky on every ordinary gate miss
- Defer the admin/settings/group/account restructuring until a separate compatibility pass is planned.
- Treat the hotspot overlap as part of the next Batch C / Batch D style transplant work, not as mechanical absorb.
- channel_service 的 antigravity 跨 anthropic/gemini 联查语义不再当成本地保留差异处理，现已按 upstream 收回严格平台隔离：pricing / mapping / restriction lookup 重新只在自身平台内匹配。

## Merged 导向阶段划分

- 阶段一：billing / resolver merged-ready 收口
  - 目标：把 `billing_service.go`、`model_pricing_resolver.go`、`gateway/openai record usage` 这一条深计费链收敛到“已等价吸收 / 必须保留的本地差异 / 不应机械恢复的上游旧路径”三个桶里。
  - 当前状态：进行中且已完成第一轮实质收口；`model_pricing_resolver.go` 的可直接吸收差异已经收平，`billing_service.go` 的剩余大 diff 已经完成第一版 merged 视角分类。
- 阶段二：gateway / openai gateway service merged pass
  - 目标：对 `gateway_service.go`、`openai_gateway_service.go` 做同样的 merged 视角拆解，优先吸收还能直接落地的 upstream 语义，并明确哪些差异是本地调度/路由设计必须保留。
  - 进入条件：阶段一不再存在明显遗漏的上游计费语义，只剩结构性或本地设计型差异。
- 阶段三：usage/admin observability 与统计链 merged pass
  - 目标：梳理 `usage_log`、ops request details、admin/user usage 统计链，形成“哪些 upstream 语义已等价存在、哪些字段/视图是本地扩展、哪些地方还不能标记 merged”的最终清单。
  - 当前判断：`usage_log.go`、`usage_log_helpers.go`、`ops_request_details.go`、`ops_openai_routing_stats.go`、`openai_routing_observability.go` 这一层目前没有比已完成项更好的 upstream 直接吸收候选；更合理的 merged 结论是“本地主线已用更完整的 observability/统计链路超集覆盖”。
  - 这一层后来又确认出一个低冲突直接候选并已吸收：`usage_log_repo.go` 不再持久化 `media_type`，从 `usageLogSelectColumns`、insert arg/order、`prepareUsageLogInsert`、`scanUsageLog` 到对应 repository tests 已全部同步清理。
  - 完成标准：可以按阶段给出一份明确的 upstream merged 判定依据，而不是继续无边界地逐个小补丁推进。
