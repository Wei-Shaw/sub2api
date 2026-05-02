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

**实施归属**：独立 commit 链，由 Worker C 执行。文件范围：
- `plugin-sdk/proto/pluginsdk/sdk.proto`（+service + 再生成 `.pb.go`）
- `backend/internal/plugin/host_service_server.go`（新）
- `plugin-sdk/sdk/host_client.go`（新，plugin 侧）
- `plugins/channel-management/service/channel_available.go`（接入 + 移除"deliberately drop"注释）
- `plugins/channel-management/main.go`（wire host client）
- 对应单测

---

## 5. Worker B'（admin advanced UI）缩小范围

原 Worker B 合并 `e1193212b` + `ba98243cc` + `6925ac25c`(前端) + `a7415d4d2`(JSON btn) 全部一次重引入 V5 W7 砍掉的 advanced UI，但 V5 W7 简化是有意的。经评估拆分：

| Commit | 决策 | 理由 |
|--------|------|------|
| `6925ac25c` 前端 template picker | **做** | 后端 `template_service.ApplyToMonitors` 已在 plugin 里，没前端等于死代码——不能这样吊着 |
| `e1193212b` headers KV 行 | **做** | UX 改进（数组化 vs 字符串），跟 template picker 一起重引入 `MonitorAdvancedRequestConfig.vue` 容器 |
| `ba98243cc` form polish | **不做** | 纯 UX polish，价值低；等用户反馈再补 |
| `a7415d4d2` JSON format btn | **不做** | 小 polish，价值低；等用户反馈再补 |

**实施归属**：Worker B' 执行，合并到一个 commit 链。

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

| commit / 项 | 决策 | 归属 |
|-------------|------|------|
| 23 个已包含 commits | skip | — |
| `a7415d4d2` seed + timeline + `7da512406` + `9ba42aa55` | done | Worker A |
| `6cd7c6054` LiteLLM fallback（窄版 HostService） | TODO | Worker C |
| `6925ac25c` 前端 + `e1193212b` headers KV | TODO | Worker B' |
| `c2f9ad7a2` event-driven scheduler | 永久不移植 | ADR §1 |
| `c46744f36` runner 单测 | 永久不移植 | ADR §2 |
| `ba98243cc` form polish | 暂不做 | ADR §5 |
| `a7415d4d2` JSON format btn | 暂不做 | ADR §5 |
| `748a84d87` host release sync | 不进 plugin | ADR §3 |

---

## 8. 未来看板

- **V6 任务**：如果还有其它反向调用需求（e.g. plugin 查询 group info），扩 `HostService` 加新 RPC，**不要**每个需求都新建 service。
- **SDK trigger 扩展**：若将来真需要"event-driven"触发，扩 `JobTrigger` 加 `OnEvent(topic)` 机制，由 host 统一调度。
