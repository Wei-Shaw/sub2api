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
