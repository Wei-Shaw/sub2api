# ADR: Upstream sync v0.1.115 → v0.1.121 移植决策

> 适用分支：`feat/plugin-system-fixes--upstream-sync-115-121`
> 关联对照表：[UPSTREAM-SYNC-115-121-MAPPING.md](./UPSTREAM-SYNC-115-121-MAPPING.md)
> 最后更新：2026-05-02

本 ADR 固化本次 upstream sync 过程中**取舍**决策（做什么、不做什么、为什么），避免下次再争论同一问题。

---

## 1. `c2f9ad7a2` event-driven channel monitor scheduler — 不移植

**上游改动**：host `channel_monitor_runner.go` 从周期 tick 改为 event-driven goroutine + `select` on channel，CRUD 写入时立即触发调度，不等下一个 tick。

**Plugin 现状**：plugin 走 SDK 官方机制 `JobTrigger.Interval(60s) + Cron`。jobs.go 顶部注释明确："CRUD writes do not need to notify the runner: the 60-second tick picks up"。

**决策**：**永久不移植**。

**理由**：
1. 移植等于绕过 SDK trigger 形态，往 plugin 内自己做 goroutine/chan 管理，违反"用 SDK 抽象层"的约束。
2. channel monitor 是健康检查，非关键路径。60s 延迟可接受；"CRUD 后立即触发"不是 SLO 要求。
3. SDK 设计时已决定 trigger 统一走 host 调度，plugin 不持有 scheduler goroutine——破这个约束会让 plugin lifecycle 更难管。

**如果将来真要立即触发**：扩 SDK 加 `JobTrigger.Immediate()` / `JobTrigger.OnEvent(topic)`，由 host 统一调度，不在 plugin 内写 goroutine。

---

## 2. `c46744f36` runner lifecycle 单测 — 不移植

**上游改动**：277 行针对 host event-driven runner 的单测。

**决策**：不移植。

**理由**：依赖 §1 的 event-driven 架构。plugin 不存在对应 runner，host 测试用例对 plugin 无意义。

**后续**：如需覆盖 plugin 的 `jobs.go` / tick 路径，另写一份针对 SDK `JobTrigger` 的轻量测试，不走 host 形态。这是新工作、不是"移植"。

---

## 3. `748a84d87` host release-only sync — 不进 plugin

**上游改动**：76 文件的 release/custom-0.1.115 回归同步（含 payment、settings、HomeView、AppHeader、partner logo、CLAUDE.md、VERSION、WechatServiceButton 等）。

**决策**：不在 plugin 分支处理。

**理由**：
1. 其中 channel 相关文件（如 `SupportedModelChip.vue`）已经被 A1 组前序 commits 覆盖。
2. 剩下的是 host-only 的 fork 定制——与插件无关，应由 host 主线（`release/custom-*` 分支）吸纳，不是 plugin 的职责。
3. 本次 upstream sync 的范围定义是"把影响渠道管理语义的改动同步到 plugin"，host 定制不在此列。

---

## 4. `6cd7c6054` 的 LiteLLM 全局回退 — 采用窄版 HostService 方案

**上游改动**：`channel_available.go` 的 `ListAvailable` 在 enrich 时调 `PricingService.GetModelPricing`（内部走 LiteLLM 全局价格表）给未配置模型兜底价格，让"Available Channels"页面不会出现"未配置模型"空白。

**Plugin 现状**：`channel_available.go` 头注释明确写"deliberately drop the LiteLLM global-pricing fallback"，把问题推给"V6 once HostServiceProxyCapability is in place"。

**决策**：**不再暂缓**。在 plugin SDK 引入**窄版** `HostService` gRPC，只加一个 RPC `ResolveModelPricing(model) → pricing`，plugin 通过它反向调用 host 的 `PricingService.GetModelPricing`。

**不采用的备选**：
- ❌ **永久暂缓**：等价于"永远不做"。UX 缺陷会永久化（用户看到渠道但看不到价格）。
- ❌ **通用 HostServiceProxyCapability**（完整 Layer 1）：正经基础设施但超出本次 sync 范围；本 commit 只需要一个方法，不需要预先建通用框架。
- ❌ **Plugin 内嵌静态 LiteLLM 价格 JSON**：违反"复用"原则（host 更新价格 plugin 不知道），数据源分裂。

**窄版 `HostService` 设计**：

| 项 | 决定 |
|----|------|
| gRPC service 名 | `HostService`（非 `HostPricingService`）——为未来扩展留空间（如 `ResolveGroup`），但**只加当前需要的 RPC**，不提前建通用 proxy |
| 位置 | `plugin-sdk/proto/pluginsdk/sdk.proto` 追加 |
| Server 实现 | `backend/internal/plugin/` 内注入 host `*PricingService`，收到 RPC 调 `GetModelPricing(model)` 转发 |
| Client stub | Plugin 通过 `ctx.Host().ResolvePricing(ctx, model)` 调用 |
| 反向连接复用 | 复用 `manager_pricing.go` 已建立的 plugin ↔ host gRPC 连接，不建第二条 |
| Host 断链时 plugin 行为 | 降级为 nil pricing（当前行为），不报错、不阻塞页面 |
| Capability 声明 | plugin manifest 声明 `needs: [host.pricing]`，host 启动时注入对应 client；未声明的 plugin 调 `ctx.Host()` 返回 nil |

**实施归属**：Worker C 执行，4 个独立 commit（`9c2ea9adc` → `f5c17d010` → `ba6e700c5` → `8fcbe924a`）。

**实际文件范围**（完成后更新）：
- `plugin-sdk/proto/sdk.proto` — 追加 `HostService` + `ResolveModelPricingRequest/Response`
- `plugin-sdk/host.go`（新）— `HostClient` interface、`HostModelPricing`（`*decimal.Decimal` 字段）、`ErrHostPricingUnavailable` sentinel、2s RPC timeout、`nilHostClient` fallback
- `plugin-sdk/context.go` + `runner_init.go` + `runner_pluginctx.go` — `PluginContext.Host()` accessor + `wireHost` 无条件 wire
- `backend/internal/plugin/grpc_server_host.go`（新）— `HostServiceServer`，依赖窄口 `HostPricingResolver` 接口（`*service.BillingService` 实现）
- `backend/internal/plugin/manager.go` + `manager_setters.go` + `manager_sdk.go` — 新字段 + `SetHostPricingResolver` + 注册到 gRPC
- `backend/cmd/server/wire_gen.go` — `pluginManager.SetHostPricingResolver(billingService)`
- `plugins/channel-management/service/channel_available.go` — 移除"deliberately drop"段落，`ListAvailable` 调用 `fillGlobalPricingFallback`
- `plugins/channel-management/plugin.go` — wire `ctx.Host()` + `ctx.Logger()` 到 AvailableChannelsService

**字段裁剪决策**：`ResolveModelPricingResponse` 只带 plugin popover 消费的 5 个字段（input/output/cache_write/cache_read/image_output price per token），不打包 priority/long-context multiplier/cache breakdown 5m-1h split。增加字段是非破坏性 wire change，将来按需扩展。

**Decimal ↔ float 边界**：线上 decimal string 传输避免 IEEE-754 漂移；plugin 侧再降为 `*float64` 只用于展示，不走计费路径（计费始终在 host 侧保持完整精度）。

**单测**：本次未加，因 `plugins/channel-management/service` 原本就没有 `channel_available` 相关单测。建议后续独立 PR 补：plugin 侧 stub `HostClient` 验证三种路径（nil/populated/unavailable）；host 侧 stub `HostPricingResolver` 验证 unknown model、empty model、nil resolver、decimal 编码零值。

---

## 5. Worker B'（admin advanced UI）缩小范围

原 Worker B 合并 `e1193212b` + `ba98243cc` + `6925ac25c`(前端) + `a7415d4d2`(JSON btn) 全部一次重引入 V5 W7 砍掉的 advanced UI，但 V5 W7 简化是有意的。经评估拆分：

| Commit | 决策 | 理由 |
|--------|------|------|
| `6925ac25c` 前端 template picker | **做** | 后端 `template_service.ApplyToMonitors` 已在 plugin 里，没前端等于死代码——不能这样吊着 |
| `e1193212b` headers KV 行 | **做** | UX 改进（数组化 vs 字符串），跟 template picker 一起重引入 `MonitorAdvancedRequestConfig.vue` 容器 |
| `ba98243cc` form polish | **不做** | 纯 UX polish，价值低；等用户反馈再补 |
| `a7415d4d2` JSON format btn | **不做** | 小 polish，价值低；等用户反馈再补 |

**实施归属**：Worker B' 执行（前端，2 个 commit：`69f718167` + `8eb0ee150`）+ Worker D 补齐后端路由（1 个 commit：`760af0a0e`）。

**Worker B' 文件**：
- `plugins/channel-management/frontend/src/api/admin/channelMonitorTemplate.ts`（新）
- `plugins/channel-management/frontend/src/components/admin/monitor/MonitorAdvancedRequestConfig.vue`（新）
- `plugins/channel-management/frontend/src/components/admin/monitor/MonitorTemplateApplyPickerDialog.vue`（新）
- `plugins/channel-management/frontend/src/components/admin/monitor/MonitorTemplateManagerDialog.vue`（新）
- `plugins/channel-management/frontend/src/views/admin/ChannelMonitorView.vue`（挂载模板管理按钮 + dialog）
- `plugins/channel-management/frontend/src/components/admin/monitor/MonitorFormDialog.vue`（挂载 advanced config + 带 `extra_headers`/`body_override_mode`/`body_override`）
- i18n: `zh/channelMonitor.ts` + `en/channelMonitor.ts` + `zh/common.ts` + `en/common.ts`（各 42 条新文案）

**Worker D 文件**（补齐后端路由）：
- `plugins/channel-management/monitor/handler/template_handler.go`（新，7 个 endpoint）
- `plugins/channel-management/plugin.go`（wire template service/handler + `registerRoutes` 新增 7 条路由）
- `plugins/channel-management/manifest_decls.go`（新增 4 条 `endpointDecls` 覆盖 7 个 method×path 组合）

**对照表误判修正**：对照表把 `6925ac25c` 的后端部分标记为"✅ 已包含"，但实际只 port 了 `template_service.go` + `template_repo.go`，HTTP handler 层没 port，前端调用会 404。Worker D 补齐了缺口。后续做类似对照时，"后端已包含"要拆分为 service / handler / 路由 / manifest 四层分别核实。

---

## 6. 已在 Worker A 完成

| Commit | 状态 | Commit hash |
|--------|------|-------------|
| `a7415d4d2` CC 模板 seed migration | done | `eef3784a6` |
| `a7415d4d2` Timeline 4-tier 颜色 | skipped（已一致） | — |
| `7da512406` extra_models save | skipped（plugin 用 textarea 规避） | — |
| `9ba42aa55` available-channels feature switch | done | `0f5ce743b` |

详见 Worker A commit 说明。

---

## 7. 决策汇总表

| commit / 项 | 决策 | 归属 | 新 commit hash |
|-------------|------|------|----------------|
| 23 个已包含 commits | skip | — | — |
| `a7415d4d2` CC 模板 seed migration | done | Worker A | `eef3784a6` |
| `a7415d4d2` Timeline 4-tier 颜色 | skip（已一致） | Worker A | — |
| `7da512406` extra_models save | skip（plugin 用 textarea 规避） | Worker A | — |
| `9ba42aa55` available-channels feature switch | done | Worker A | `0f5ce743b` |
| 对照表 + ADR | done | 本文档 | `256a868ce` |
| `6cd7c6054` LiteLLM fallback（proto）| done | Worker C | `9c2ea9adc` |
| `6cd7c6054` LiteLLM fallback（SDK client + `ctx.Host()`）| done | Worker C | `f5c17d010` |
| `6cd7c6054` LiteLLM fallback（host server）| done | Worker C | `ba6e700c5` |
| `6cd7c6054` LiteLLM fallback（plugin 接入）| done | Worker C | `8fcbe924a` |
| `6925ac25c` 前端 template apply picker | done | Worker B' | `69f718167` |
| `e1193212b` headers KV + advanced config 容器 | done | Worker B' | `8eb0ee150` |
| template admin handler + 路由 + manifest | done | Worker D | `760af0a0e` |
| `c2f9ad7a2` event-driven scheduler | 永久不移植 | ADR §1 | — |
| `c46744f36` runner 单测 | 永久不移植 | ADR §2 | — |
| `ba98243cc` form polish | 暂不做 | ADR §5 | — |
| `a7415d4d2` JSON format btn | 暂不做 | ADR §5 | — |
| `748a84d87` host release sync | 不进 plugin | ADR §3 | — |

**本次 upstream sync 共产出 10 个 commit**，位于 `feat/plugin-system-fixes--upstream-sync-115-121`。

---

## 8. 未来看板

- **V6 任务**：如果还有其它反向调用需求（e.g. plugin 查询 group info），扩 `HostService` 加新 RPC，**不要**每个需求都新建 service。
- **SDK trigger 扩展**：若将来真需要"event-driven"触发，扩 `JobTrigger` 加 `OnEvent(topic)` 机制，由 host 统一调度。
- **补单测**：`channel_available` fallback 路径（plugin 侧）+ `HostServiceServer` 编码路径（host 侧）原 plugin 都无单测覆盖，建议独立 PR 补，不拖本次 sync 节奏。
- **验证 azcc 场景**：上游 `6cd7c6054` commit message 描述的"只有 pricing 没有 mapping 的渠道，popover 显示'未配置模型'"bug，部署 test 环境后应人工验证已修复。

---

## 9. 执行总结

- **原范围**：33 A 类 commit
- **已包含**：23（skip）
- **Worker A 完成**：2 个 done（seed migration + feature switch）+ 2 个 skip（timeline 颜色 / extra_models，plugin 已规避）
- **Worker C 完成**：1 个 done（LiteLLM fallback，拆成 4 个 commit）
- **Worker B' + D 完成**：2 个 done（template picker + headers KV），含后端路由补缺
- **永久不移植**：2（`c2f9ad7a2` scheduler + `c46744f36` 单测）
- **暂不做**：2（`ba98243cc` polish + `a7415d4d2` JSON btn）
- **不进 plugin**：1（`748a84d87` host release sync）

合计产出 **10 个 commit**，分支 `feat/plugin-system-fixes--upstream-sync-115-121`，待 push + test 环境部署验证。
