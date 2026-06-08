# Upstream sync v0.1.115 → v0.1.121 — A 类 33 commit 移植对照表

> 目标分支：`feat/plugin-system-fixes--upstream-sync-115-121`（基于
> `feat/plugin-system-fixes`，主仓库已切换到该分支）。本文档只给出**判定**，
> 实际 cherry-pick / 移植代码不在本任务范围。
>
> 判定方法：
> 1. `git show <hash> --stat` 看 host 改动文件
> 2. 按 host → plugin 文件映射规则定位 plugin 对应文件
> 3. `diff` / `Read` plugin 当前内容 vs host @ v0.1.121
> 4. 判定为 ✅（已包含语义） / ❌（未包含） / ⚠️（部分包含或细节差异）
>
> **最重要的背景**：plugin 端**有意**简化掉了若干特性
> （admin FormDialog 不带 advanced/template/key-picker；
> available channels handler 不接 LiteLLM 全局回退；
> scheduler 用 SDK JobTrigger interval-tick，而非 host 的 event-driven）。
> 这些刻意取舍在下方表里记为 ⚠️/❌（按"语义是否一致"判定，不按"代码是否完全一致"），
> 移植时需要逐条与负责人确认是否补齐。

---

## A1 组：Available Channels（共 14 commit，原任务列了 11 + 3 个补充全部归档于此）

> 注：原任务文中 A1 写"11 个"但实际列了 14 个 commit hash（`6cd7c6054`、
> `4a3652ec0`、`375aefa20`、`88decb6e0`、`365ef1fdf`、`748a84d87`），这里把
> 全部归到 A1。

### ✅ 已包含（语义无需补移植）

| HASH | subject | 判定依据 |
|------|---------|----------|
| `654cfb648` | feat(channels): add Available Channels aggregate view | `plugins/channel-management/service/channel_available.go` 完整实现 ListAvailable / VisibleGroupIDs；`handler/available_channel_handler.go` 实现 List + 三层过滤；`service/channel.go` + `internal/domain/channel_view.go` 实现 SupportedModels = mapping ∪ pricing |
| `800802b8a` | feat(channels): explode by platform + apply platform theme | handler 已按 platform 切片成 `userChannelPlatformSection`；`SupportedModelChip.vue` + `AvailableChannelsTable.vue` 已带 platform 主题色 |
| `3cdd5754d` | feat(channels): aggregate by channel + rowspan table | `AvailableChannelsTable.vue` 用 tbody + rowspan 渲染；handler payload 是 channel-aggregate 形态 |
| `ff4ef1b57` | feat(channels): themed model popover + group-badge w/ rate, sub & exclusivity | `SupportedModelChip.vue` 带 hover 弹层 + 平台主题；`GroupBadge.vue` 渲染倍率/订阅/独占；plugin handler/service 透传 `RateMultiplier` / `SubscriptionType` / `IsExclusive` |
| `9dae6c7ae` | feat(sidebar+groups): available-channels above channel-status; show rate for subscription groups | plugin `GroupBadge.vue` 已有订阅组倍率渲染逻辑；sidebar 部分由 plugin manifest 声明，本身就排在 channel-status 前 |
| `25a503550` | fix(available-channels): description as own column, fixed table layout | `AvailableChannelsTable.vue` 已经把 description 渲染为独立列（`columns.description`） |
| `59290e39f` | chore(channels): drop admin-side available channels view | plugin 的 `frontend/src/views/admin/` 下只存 `ChannelMonitorView.vue`，admin available channels view 本来就没有 |
| `6cd7c6054` | fix(channels): supported models = mapping ∪ pricing | `domain.ChannelView.SupportedModels` 已实现 mapping ∪ pricing；**LiteLLM 全局回退刻意未携带**（见下方 ⚠️） |
| `4a3652ec0` | refactor(channels): normalize at cache fill, eliminate frontend as-cast | plugin handler 类型已强类型化，前端组件 import 自 plugin 本地 DTO，无 `as` 强转；`normalizeBillingModelSource` 在 service 层完成 |
| `375ef1fdf` | refactor: centralize BillingModelSource normalization + exhaustive enum maps | plugin `service/channel.go` 暴露 `BillingModelSourceChannelMapped` 等常量；`channel_available.go` 在写出口 normalize；前端 `SupportedModelChip.vue` 用穷举 enum |
| `88decb6e0` | refactor(channels): tighten types & error paths per second review | plugin 已采用强类型 DTO，handler 错误统一走 `response.ErrorFrom`，`SupportedModelChip` 类型收敛到 plugin 本地 |
| `365ef1fdf` | refactor(channels): consolidate pricing index, tighten DTOs | `channel_available.go` 已合并 pricing index；DTO 已紧凑；`PricingRow.vue` 在 plugin 本地 |

### ⚠️ 部分包含 / 需细查

| HASH | subject | 现状描述 |
|------|---------|----------|
| `6cd7c6054` | supported models … with global LiteLLM fallback | mapping ∪ pricing **已实现**，但 plugin 头注释明确写：`We deliberately drop the LiteLLM global-pricing fallback`。原因是 SDK 还没暴露全局价格表（V6 任务）。如果 v0.1.121 行为要完全对齐，需要等 SDK 增加 `HostServiceProxyCapability`，或暂时不接。**移植时不要硬塞 LiteLLM**。 |

### ❌ 需移植

| HASH | subject | 涉及 host 文件 → plugin 文件 | 难度 |
|------|---------|------------------------------|------|
| `9ba42aa55` | feat(channels): gate available channels behind feature switch (backend) | `backend/internal/handler/available_channel_handler.go` 加 `featureEnabled()`，依赖 `setting_service.GetAvailableChannelsRuntime` + `domain_constants` 新键 → plugin `handler/available_channel_handler.go` 头注释明确写"intentionally does not enforce a feature-flag in this Step"；plugin 已有 `pluginsdk.SettingsClient` 基础设施（monitor 已用），可仿照 `monitor/service/runtime.go` 写一个 `availablechannels/runtime.go` + 在 handler 里加 `featureEnabled` 守卫 + 对应 `settings/settings_schema.json` 字段 + `monitor/handler/user_handler.go` 模式 503/200 返回 | low |

### 🟦 与 channel-management 无关（A1 中误入清单）

| HASH | subject | 原因 |
|------|---------|------|
| `748a84d87` | sync: bring over remaining release/custom-0.1.115 changes | 这一笔是把所有"非 PR" 的 release-only 改动一次性带回（payment、settings、HomeView、AppHeader、partner logo 等 76 个文件），仅 `frontend/src/components/channels/SupportedModelChip.vue` 和 admin SettingsView 等少量带 channel 字眼。**channel 相关部分已经被前面 A1 commits 覆盖**，剩下的属于 host-only 的 fork 定制（CLAUDE.md / VERSION / WechatServiceButton 等），**不需要进入 plugin**。 |

---

## A2 组：Channel Monitor（19 个）

### ✅ 已包含

| HASH | subject | 判定依据 |
|------|---------|----------|
| `20a4e4187` | feat(monitor): admin channel monitor MVP w/ SSRF + batch aggregation | plugin `monitor/service/{checker,checker_http,validate}.go` 含 SSRF（私网/loopback 拦截）；`monitor/repository/channel_monitor_repo.go` 含 batch aggregation；`monitor/handler/admin_handler.go` 286 行覆盖 CRUD+run；migrations `001_add_channel_monitors.sql` 已 ported |
| `a1425b457` | feat(channel-monitor): redesign user dashboard as card grid | plugin 完整带 `MonitorCardGrid.vue` / `MonitorCard.vue` / `MonitorHero.vue` / `MonitorAvailabilityRow.vue` / `MonitorMetricPair.vue` / `MonitorTimeline.vue` / `ProviderIcon.vue`；user_handler 也按 card 形态返 |
| `8cf83c984` | feat(channel-monitor): aggregate history to daily rollups | plugin `monitor/repository/monitor_rollup_repo.go` + migration `002_add_channel_monitor_aggregation.sql`；service 层 aggregator + jobs.go 跑 daily rollup（**软删除部分被 ef6ec8a15 移除**） |
| `ef6ec8a15` | fix(channel-monitor): drop soft delete, refactor feature flag to declarative form | migration `003_drop_channel_monitor_deleted_at.sql`；repo 无 `deleted_at IS NULL` 过滤；feature flag 走 `monitor/settings/settings_schema.json` 声明式 + `LoadMonitorRuntime` |
| `a29642599` | feat(channel-monitor): request templates with snapshot apply + headers/body override | plugin `monitor/service/template_service.go` + `monitor/repository/template_repo.go` + migration `004_add_channel_monitor_request_templates.sql`；admin handler 暴露 template CRUD |
| `b363bff1d` | feat(channel-monitor): preserve upstream error body | plugin `monitor/service/checker_http.go::truncateForErrorBody` + 限长常量 `monitorErrorBodySnippetMaxBytes` |
| `0d01bd908` | refactor: remove INTELLIGENCE MONITOR hero title | plugin `MonitorHero.vue` 没有 hero 标题块（只有窗口 tabs + status chip + refresh 按钮） |
| `0c48f08f5` | refactor(channel-status): drop breadcrumb + subtitle from MonitorHero | plugin `MonitorHero.vue` 无 breadcrumb / subtitle |
| `09fd83ab9` | fix(monitor): clean up unused updatedAt/updatedLabel | plugin `MonitorHero.vue` / `ChannelStatusView.vue` 无 updatedAt/updatedLabel 引用 |
| `6699d3376` | fix(monitor): remove redundant "updated at" label from MonitorHero | 同上，plugin Hero 无 "updated at" |
| `f7c8377ab` | fix(monitor): remove UNAVAILABLE status, keep only OPERATIONAL/DEGRADED | plugin Hero 的 `OverallStatus` 只声明 `'operational' \| 'degraded'` |
| `0dcc0e050` | feat(monitor): proportion-based overall status + reusable auto-refresh | plugin `frontend/src/composables/useAutoRefresh.ts` + `components/common/AutoRefreshButton.vue`；`MonitorHero.vue` 接 `autoRefresh` prop；`ChannelStatusView.vue` 用 `useAutoRefresh()` |

### ⚠️ 部分包含 / 需细查

| HASH | subject | 现状描述 |
|------|---------|----------|
| `a7415d4d2` | feat(monitor): 30-day raw retention + timeline 4-tier style + CC template seed + JSON format button | **30 天 retention**：`monitor/service/const.go` 有 `monitorHistoryRetentionDays = 30` ✅。**Timeline 4-tier 样式**：`MonitorTimeline.vue` 在 plugin（127 行）需要再 diff 确认 4-tier 颜色阶梯是否一致；可用 `diff <(git show v0.1.121:frontend/src/components/user/monitor/MonitorTimeline.vue) plugins/.../MonitorTimeline.vue` 复核。**CC 模板 seed**：plugin migrations 没有等价的 `129_seed_claude_code_template.sql`（也没有任何 CC seed 数据），❌ 需补 seed migration。**JSON format button**：plugin admin `MonitorFormDialog.vue` 有意简化（删 advanced UI），❌ 没有 JSON format button。 |
| `6925ac25c` | feat(channel-monitor): apply template via subset picker | **后端 ✅**：`template_service.ApplyToMonitors` + `template_repo.ApplyToMonitors` + admin_handler 路由都在。**前端 ❌**：plugin 没有 `MonitorTemplateApplyPickerDialog.vue` / `MonitorTemplateManagerDialog.vue`，admin Form 也刻意不挂 template 字段。需补 UI 层。 |
| `7da512406` | feat(channel-monitor): add feature switch settings + fix extra_models save | **Feature switch**：plugin `monitor/settings/settings_schema.json` + `LoadMonitorRuntime` + `user_handler.featureEnabled` 已实现 ✅。**extra_models save**：plugin `MonitorFormDialog.vue` 已写 `extra_models` 提交逻辑（line 188、233、266、284），可能已包含；需 diff host 当时的 fix 验证是否一致 ⚠️。 |

### ❌ 需移植

| HASH | subject | 涉及 host 文件 → plugin 文件 | 难度 |
|------|---------|------------------------------|------|
| `e1193212b` | feat(monitor): switch headers input to key-value rows | `frontend/src/components/admin/monitor/MonitorAdvancedRequestConfig.vue` 137 行重写为 KV 行 → plugin **没有 MonitorAdvancedRequestConfig.vue 文件**，整个 advanced 区块都被 V5 W7 砍掉。需要补 advanced UI（与 a7415d4d2、ba98243cc 一同打包） | high |
| `c2f9ad7a2` | refactor(channel-monitor): event-driven scheduler + sidebar cleanup | host `channel_monitor_runner.go` 改为事件驱动（goroutine + select on chan）→ plugin scheduler 是 SDK `JobTrigger.Interval(60s)` + `Cron`（`monitor/service/jobs.go`），**架构不同**。plugin 端 jobs.go 第 30 行注释明确："CRUD writes do not need to notify the runner: the 60-second tick picks up"。如果坚持移植，要重做 plugin SDK 的 trigger 形态（高度侵入），不推荐 | high（**可能不要做**） |
| `c46744f36` | refactor(channel-monitor): tighten runner lifecycle + add unit tests | 277 行新增 runner 单测 → plugin `monitor/service/` 下**没有任何 _test.go 文件**。如果要补，需先确定 plugin 的 runner 形态（c2f9ad7a2 决定），再写对应测试 | med（**依赖 c2f9ad7a2 的取舍**） |
| `ba98243cc` | feat(channel-monitor): gate UI by feature switch + polish form UX | host 改了 `MonitorFormDialog.vue` / `MonitorKeyPickerDialog.vue` / sidebar / `useChannelMonitorFormat.ts` / `KeysView.vue` / `ChannelStatusView.vue` → plugin 没有 `MonitorKeyPickerDialog.vue`（V5 W7 已删 "use my key" picker），FormDialog 也简化过；user 侧 ChannelStatusView 的 feature gate 部分 ⚠️ 需要确认 plugin 是否在 user_handler 503 时就能让前端隐藏入口，还是需要 client 侧也读 SettingsClient | med |

---

## A3 组：杂项（4 个 — 已与 A2 整合）

> 原任务列了 0dcc0e050 / 09fd83ab9 / 6699d3376 / f7c8377ab，本质都属于 A2
> Hero/View 的清理 commit。已在 A2 ✅ 中分别记录。**A3 全部 ✅**。

---

## 总结

| 状态 | 数量 | 说明 |
|------|------|------|
| ✅ 已包含 | **23** | 主功能（available channels 聚合视图 / monitor MVP / templates / rollup / preserve error / hero 清理 / auto-refresh）已经在 plugin 中实现 |
| ⚠️ 部分包含 | **4** | `6cd7c6054`(LiteLLM 故意不带) / `a7415d4d2`(retention✅，CC seed❌，JSON btn❌) / `6925ac25c`(后端✅前端❌) / `7da512406`(switch✅，extra_models 需复核) |
| ❌ 需移植 | **5** | `9ba42aa55`(available-channels feature switch) / `e1193212b`(headers KV 输入) / `c2f9ad7a2`(event-driven scheduler，**可能放弃**) / `c46744f36`(runner 单测，依赖前者) / `ba98243cc`(UI feature gate + form UX) |
| 🟦 不进 plugin | **1** | `748a84d87` 是 host 端 release-only 同步，channel 部分已被前序 commits 覆盖 |

合计 33 个 commit（A1 14 + A2 19 + A3 0 = 33，A3 已并入 A2）。

### 推荐执行顺序

按依赖关系：

1. **先把 ⚠️ 收齐**
   - `7da512406` 的 extra_models save 部分先 diff 确认；如已一致跳过，否则一行小修
   - `a7415d4d2` 的 CC 模板 seed migration → 新增 `plugins/channel-management/migrations/015_seed_claude_code_template.sql`
   - `a7415d4d2` 的 Timeline 4-tier 颜色 → 一次 diff + 小补丁
   - `6cd7c6054` 的 LiteLLM 全局回退 → **暂缓**（等 SDK V6 暴露 host 价格表）

2. **补 ❌ available-channels feature switch**（独立、易做）
   - `9ba42aa55` → 在 `plugins/channel-management/` 加 `runtime.go`（仿 monitor），handler 加 `featureEnabled`，schema 加字段

3. **补 ❌ admin advanced UI 链**（一并做）
   - `e1193212b` headers KV 行
   - `ba98243cc` UI feature gate + form UX
   - `6925ac25c` template apply picker（前端 UI）
   - `a7415d4d2` JSON format button
   - 三者都涉及 admin `MonitorFormDialog.vue`，**强烈建议合并到一个移植 PR** 一次性补齐 advanced 区块（重新引入 `MonitorAdvancedRequestConfig.vue` 等组件）

4. **scheduler/test 决策**（最后处理 / 也可放弃）
   - `c2f9ad7a2` event-driven scheduler：与 plugin SDK JobTrigger 架构冲突；**建议保留 plugin 当前的 60s tick + cron**，把这条标记为 "不移植"，并在文档里写明取舍
   - `c46744f36` runner 单测：依赖上一条决策。如果 plugin scheduler 不改，新写一份针对 jobs.go / 60s tick 的轻量单测即可（也可标 "不移植"）

### Host → plugin 文件映射速查（用于后续移植）

| host 路径 | plugin 路径 |
|-----------|-------------|
| `backend/internal/service/channel.go` / `channel_available.go` | `plugins/channel-management/service/channel.go` / `channel_available.go` |
| `backend/internal/handler/available_channel_handler.go` | `plugins/channel-management/handler/available_channel_handler.go` |
| `backend/internal/service/channel_monitor_*.go` | `plugins/channel-management/monitor/service/{aggregator,checker,checker_http,jobs,runtime,service,template_service,types,validate}.go` |
| `backend/internal/repository/channel_monitor_repo.go` | `plugins/channel-management/monitor/repository/channel_monitor_repo.go`（+ aggregation/history/rollup/template 拆分文件） |
| `backend/internal/handler/admin/channel_monitor_*.go` | `plugins/channel-management/monitor/handler/admin_handler.go`（合并） |
| `backend/internal/handler/channel_monitor_user_handler.go` | `plugins/channel-management/monitor/handler/user_handler.go` |
| `backend/migrations/12X_*.sql` | `plugins/channel-management/migrations/00X_*.sql`（编号重新排，参见现有 001~014） |
| `frontend/src/views/user/AvailableChannelsView.vue` | `plugins/channel-management/frontend/src/views/user/AvailableChannelsView.vue` |
| `frontend/src/views/user/ChannelStatusView.vue` | `plugins/channel-management/frontend/src/views/user/ChannelStatusView.vue` |
| `frontend/src/components/channels/*` | `plugins/channel-management/frontend/src/components/channels/*` |
| `frontend/src/components/user/monitor/*` | `plugins/channel-management/frontend/src/components/user/monitor/*` |
| `frontend/src/components/admin/monitor/*` | `plugins/channel-management/frontend/src/components/admin/monitor/*` |
| `frontend/src/api/channels.ts` | `plugins/channel-management/frontend/src/api/user/availableChannels.ts` |
| `frontend/src/api/channelMonitor.ts` | `plugins/channel-management/frontend/src/api/user/channelMonitor.ts` + `api/admin/channelMonitor.ts` |
| `frontend/src/composables/useAutoRefresh.ts` / `useChannelMonitorFormat.ts` | `plugins/channel-management/frontend/src/composables/useAutoRefresh.ts`（plugin 已 port） |
| `frontend/src/components/common/AutoRefreshButton.vue` | `plugins/channel-management/frontend/src/components/common/AutoRefreshButton.vue` |
| `frontend/src/components/common/GroupBadge.vue` | `plugins/channel-management/frontend/src/components/channels/GroupBadge.vue` |

### 重要 plugin 设计取舍（移植时必读）

1. **不引入 LiteLLM 全局价格回退**：plugin SDK 还没有 `HostServiceProxyCapability`。`channel_available.go` 头注释已写明，移植 `6cd7c6054` 时不要复刻 `synthesizePricingFromLiteLLM`。
2. **scheduler 用 SDK JobTrigger（60s interval + cron），不要改成 event-driven**：`c2f9ad7a2` 与 plugin 架构冲突，建议放弃移植。
3. **admin form 已被 V5 W7 简化**：补 advanced UI 时需"重新引入"组件（KV headers / body override / template apply picker / key picker），而非"在现有结构上加字段"。
4. **plugin handler 走 SDK metadata，不走 host JWT 中间件**：移植涉及鉴权的代码时统一用 `pluginsdk.RequestMetadata(c.Request)` 取 `UserID`，不要 `middleware.GetAuthSubjectFromContext(c)`。
5. **plugin 前端 import 路径**：`@sub2api/plugin-sdk`（`Icon`/`PlatformIcon`/`BaseDialog`/`Select`），其余从 plugin 本地 `../../api/...` `../../utils/...` 引；不要 `@/...`。

---

## Post-merge TODO（7 批 merge 全部完成后处理）

以下 plugin 文件在逐版本 host merge 期间采用了 `git checkout --ours` 策略，
上游改动投射到 plugin 路径上被跳过，需要在所有 merge 完成后统一检查是否要二次 port。

### v0.1.115
- `plugins/channel-management/frontend/src/api/channels.ts`
- `plugins/channel-management/frontend/src/components/types.ts`
- `plugins/channel-management/repository/channel_repo.go`
  - v0.1.115 引入了 `features_config` 列（新列），plugin HEAD 可能没有对应处理
  - 检查 migration 是否涉及该列（host 加列 DDL 是否应并进 plugins/channel-management/migrations/）
  - 检查 channel service/handler/DTO 是否需要透传该字段

### v0.1.115 host 文件 channel-coupled 残留（merge 时手动处理）

#### 1. `backend/internal/service/openai_gateway_service.go::RecordUsage`
- **现象**：v0.1.115 引入 `applyAccountStatsCost(ctx, usageLog, s.channelService, ...)`
  调用，调用了 plugin 已删除的 `channelService` + 已删除的 `account_stats_pricing.go`
  helper（channel-management 在 commit `120d521e1` 中已 port 该功能）
- **临时处理**：在 merge 中删除该调用块（用注释说明 plugin 已接管），保留 fork
  通过 `accountStatsResolver` hook 走 `PricingExtension.ResolveAccountStatsCost` RPC 的方式
- **后续验证**：plugin 的 `ResolveAccountStatsCost` RPC 是否覆盖了 v0.1.115 的所有
  upstream model 匹配场景（custom rules、wildcard、规则优先级）；是否需要补 port

#### 2. `backend/internal/service/gateway_websearch_emulation.go::shouldEmulateWebSearch`【**已决策 - 方案 A**】
- **现象**：v0.1.115 新增的 websearch 模拟功能（host 文件，非 plugins/）的 `default`
  模式分支原本调用 `s.channelService.GetChannelForGroup(...)` 和
  `ch.IsWebSearchEmulationEnabled(...)`
- **冲突原因**：plugin 已删除 host 的 `channelService` 字段和 `Channel` struct
- **决策（v0.1.115 merge 时采用）**：**方案 A** —— 删 channel fallback；
  websearch emulation 改为依赖账号 `extra.web_search_emulation` 显式 `enable` 才启用
  - 落地位置：`backend/internal/service/gateway_websearch_emulation.go`
    `shouldEmulateWebSearch` 的 `default` 分支直接 `return false`（保留 `groupID`
    形参以维持调用签名稳定，标 `_ = groupID` 抑制 unused 警告）
  - 文件头部追加 NOTE 说明 v0.1.115 上游语义、fork 退化原因和 V5-CURATE 后续路径
- **损失**：失去 "channel 级一键开关 web search emulation" 产品能力
- **后续（V5 W7 / V5-CURATE §X）**：如有需求扩 plugin SDK `ChannelExtension`
  加 `IsWebSearchEmulationEnabled(groupID, platform)` RPC，host 反向查 plugin
- **关联文件变更**：
  - `backend/internal/service/gateway_websearch_emulation.go`：`shouldEmulateWebSearch`
    的 default 分支精简为 `return false`，加 NOTE 注释
  - `backend/internal/service/channel_websearch_test.go`：**整文件删除**（依赖已删除
    的 host `Channel` struct）
  - `backend/internal/service/gateway_websearch_emulation_test.go`：**整文件删除**
    （混合 channel + 非 channel 测试，依赖 `channelService` 字段；按"删整个文件"
    的稳妥策略处理，丢失上游新增的 `isOnlyWebSearchToolInBody` /
    `extractSearchQueryFromBody` / `buildSearchResultBlocks` / `buildTextSummary`
    等纯函数测试，可在后续单独补回不依赖 channel 的部分）
  - `backend/internal/pkg/websearch/*`、`backend/internal/service/websearch_config.go`：
    保留上游版本，本次 merge 全量采纳

#### 3. `backend/internal/service/antigravity_credits_overages.go`
- **现象**：merge 期间被 `--theirs` 覆盖，丢失 fork 自有的 `checkAccountCredits`
  / `retryCreditsOnDegraded` / `logCreditsResult` / `hasEnoughCredits` /
  `isAntigravityDegradedResponse` 5 个函数；同时 `account_usage_service.go`
  丢了 `GetAntigravityCredits` / `InvalidateAntigravityCreditsCache` 2 个方法
- **修复**：merge 期间手动追加回上述 fork 自有 method/helper（完全是 fork 内容，
  与上游无冲突）

#### 4. `backend/internal/service/openai_gateway_service.go` — codex rate-limit helpers
- **现象**：fork 自有 `codexRateLimitResetAtFromSnapshot` 等 4 个 helper 被 merge
  覆盖删掉，但 fork 的 `account_test_service.go` 还在调用
- **修复**：在 openai_gateway_service.go 重新追加 `codexUsagePercentExhausted` +
  `codexRateLimitResetAtFromSnapshot`（仅 build 需要这一个；其他 3 个调用方已不存在）

（后续批次如遇类似情况，在下方继续追加）

---

## v0.1.119 merge 批次（2026-05-02）

`release/custom-0.1.119` 一次合并了 v0.1.117 + v0.1.118 + v0.1.119 全部上游 commit
（fork 跳过了 117/118 release 分支）。共 259 个上游 commit，绝大多数 host 改动通过
auto-merge 吸纳；最终需要手工处理的冲突仅 16 个（1 DU + 15 UU），且无 plugins/ 命中。

### 冲突分类

| 类型 | 数量 | 处理 |
|------|------|------|
| DU | 1 | `backend/internal/handler/admin/channel_handler.go` — git rm（plugin 已 port）|
| fork-only / frontend i18n / router / sidebar | 5 | `--ours`：VERSION / en.ts / zh.ts / index.ts / AppSidebar.vue |
| 真·双改 | 10 | 手工 hunk merge |

### 真·双改文件裁决

1. **backend/internal/handler/handler.go**：保留 plugin 的 `Plugin` / `PluginSettings`
   字段；吸纳上游新增 `Affiliate`；继续不引入 `Channel*` 字段（已迁 plugin）
2. **backend/internal/handler/wire.go**：参数列表追加 `affiliateHandler` + 保留
   `pluginHandler` / `pluginSettingsHandler`；`ProvideAdminHandlers` 返回结构体
   同步增加 `Affiliate` 字段；ProviderSet 列表追加 `admin.NewAffiliateHandler`
3. **backend/cmd/server/wire_gen.go**：上游新增 `proxyRepository` /
   `rectifierSettingsCache` / `settingService.SetRectifierSettingsCache(...)`
   全部吸纳；删除上游分支 channel handler 列表，保留 plugin 的 `pluginManager`
   初始化大块，`ProvideAdminHandlers` 调用同步增加 `affiliateHandler`
4. **backend/internal/service/wire.go**：`ProvideSettingService` 签名追加
   `proxyRepo ProxyRepository` 参数 + 调 `svc.SetProxyRepository(proxyRepo)`
   （setting_service.go 已被 auto-merge 引入新方法和字段）
5. **backend/internal/service/account_test_service.go**：全部接受上游
   （`prompt`/`mode` 参数 + `normalizeAccountTestMode` + `reconcileOpenAI429State`）
6. **backend/internal/service/account_test_service_openai_test.go**：全部接受上游
   （新签名 + 新增 SSE EOF / 429 等测试用例）
7. **backend/internal/service/openai_gateway_service.go**：5 个 hunk 都取上游骨架
   并把 `channelService.X(ctx, groupID)` 改回 plugin 的 `channelCacheReader.X(ctx,
   groupID, PlatformOpenAI)`（保留 plugin 的 reader 字段 + 上游的 compact / restriction
   语义）
8. **backend/internal/service/payment_service.go**：struct 字段表保留 plugin 的
   `eventPublisher PluginEventPublisher`（Plugin Hook Phase B）+ 吸纳上游新增
   `affiliateService *AffiliateService`
9. **frontend/src/api/admin/index.ts**：删除 `channels*` 三个 export，吸纳
   `affiliates: affiliatesAPI` + `affiliatesAPI` 命名导出
10. **frontend/src/components/account/EditAccountModal.vue**：保留 plugin 的
    sdk-bundled UI 组件 import（`@sub2api/plugin-sdk` 出 `BaseDialog` / `ConfirmDialog`
    / `Select`），吸纳上游新增的 `OpenAICompactMode` 类型
11. **frontend/src/views/admin/SettingsView.vue**：保留 plugin 的 sdk-bundled UI
    （`Select` / `ConfirmDialog` / `Toggle` 来自 `@sub2api/plugin-sdk`），吸纳上游
    新增的 `affiliatesAPI` 等 affiliate 相关 import

### Build / vet 验证

- `cd backend && go build ./... && go vet ./...` — clean
- `cd plugin-sdk && go build ./...` — clean
- `cd plugins/channel-management && go build ./... && go vet ./...` — clean
- VERSION 保持 `0.1.114.4-test.3`（未递增）

### 二次修复

无：本批次冲突均为 host 端 hunk-level，未触发 channel-coupled blocker。
plugins/ 全程 0 个 UU；所有 channel monitor / channel handler 文件继续保持
"已删除 + 不重新引入"状态，符合 plugin port 边界。

### 待办

无 plugins/ TODO 新增。后续 v0.1.120 / v0.1.121 上游同步如再涉及 OpenAI gateway 的
channel 相关重构，注意继续把 `channelService` 改回 `channelCacheReader` 的命名差异。

---

## Batch 4: v0.1.120 (56 commits)

### 冲突分类

| 类型 | 数量 | 文件 |
|------|------|------|
| DU | 1 | `account_handler_mixed_channel_test.go` |
| fork-only UU | 1 | `VERSION` |
| i18n UU | 2 | `en.ts`, `zh.ts` |
| wire_gen UU (3-way) | 1 | `wire_gen.go` |
| **合计** | **5** | |

### 真·双改裁决

1. **backend/cmd/server/wire_gen.go**（2 处冲突）：
   - 冲突1：`antigravityGatewayService` 保留 fork 的 `accountUsageService` 参数；
     `accountTestService` 吸纳上游新增 `claudeTokenProvider` 参数
   - 冲突2：`openAIGatewayService` 保留 fork 的 `channelCacheReader`（替代上游
     `channelService`），吸纳上游新增 `settingService` 参数

### i18n 补齐

`--ours` 后手工从上游 diff 补入两组 key（en.ts + zh.ts 各一份）：
- **Vertex Service Account** 相关 20 个 key（`vertexLabel` .. `vertexSaJsonRequired`）
- **OpenAI Fast/Flex Policy** 相关 32 个 key（`openaiFastPolicy.title` .. `fallbackErrorMessagePlaceholder`）

### Build / vet 验证

- `cd backend && go build ./... && go vet ./...` — clean
- `cd plugin-sdk && go build ./...` — clean
- `cd plugins/channel-management && go build ./... && go vet ./...` — clean
- VERSION 保持 `0.1.114.4-test.3`（未递增）

### 二次修复

无。本批次冲突均为 wire 层参数变更 + i18n 新增 key，未触发 channel-coupled blocker。

### 待办

无 plugins/ TODO 新增。v0.1.121 合并时继续注意 `channelService` → `channelCacheReader`
命名差异。
