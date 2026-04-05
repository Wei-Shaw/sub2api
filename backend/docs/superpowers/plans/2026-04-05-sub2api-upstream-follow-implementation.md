# sub2api Upstream Follow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在保留当前本地 routing / billing / opencode 设计语义的前提下，分批吸收 `origin/main` 的非冲突增量，并为高风险热点建立独立移植入口。

**Architecture:** 以 `b384570de3545f036f250e68e9ca31362142dadf` 为固定对比基线，先按结构化吸收清单推进：Batch A 机械吸收低耦合增量，Batch B 吸收接线层，Batch C 不直接改代码，只写逐文件移植设计。每一批完成后都重新跑对应测试，确保不破坏本地主线语义。

**Tech Stack:** git, PowerShell, Go test, pnpm build/test, markdown inventory/report maintenance.

---

## 文件边界

- Read: `backend/docs/superpowers/specs/2026-04-05-sub2api-upstream-follow-design.md`
- Read: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`
- Create: `backend/docs/superpowers/plans/2026-04-05-sub2api-upstream-follow-implementation.md`
- Modify during execution:
  - Batch A targets:
    - `backend/internal/repository/channel_repo.go`
    - `backend/internal/repository/channel_repo_pricing.go`
    - `backend/internal/repository/channel_repo_test.go`
    - `backend/internal/service/channel.go`
    - `backend/internal/service/channel_service.go`
    - `backend/internal/service/channel_service_test.go`
    - `backend/internal/service/channel_test.go`
    - `backend/internal/handler/admin/channel_handler.go`
    - `backend/internal/handler/admin/channel_handler_test.go`
    - `backend/internal/handler/admin/dashboard_handler.go`
    - `backend/internal/repository/wire.go`
    - `backend/internal/handler/wire.go`
    - `frontend/src/api/admin/channels.ts`
    - `frontend/src/components/admin/channel/IntervalRow.vue`
    - `frontend/src/components/admin/channel/ModelTagInput.vue`
    - `frontend/src/components/admin/channel/PricingEntryCard.vue`
    - `frontend/src/components/admin/channel/types.ts`
    - `frontend/src/views/admin/ChannelsView.vue`
    - `backend/migrations/081_create_channels.sql`
    - `backend/migrations/082_refactor_channel_pricing.sql`
    - `backend/migrations/083_channel_model_mapping.sql`
  - Batch B targets:
    - `backend/internal/server/routes/admin.go`
    - `backend/internal/service/wire.go`
    - `backend/cmd/server/wire_gen.go`
    - `frontend/src/api/admin/dashboard.ts`
    - `frontend/src/api/admin/index.ts`
    - `frontend/src/components/layout/AppSidebar.vue`
    - `frontend/src/router/index.ts`
  - Batch C outputs:
    - `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`
    - `backend/docs/superpowers/specs/2026-04-05-sub2api-batch-c-hotspots-design.md`

## Task 1: 锁定吸收清单并完成 Batch A 机械吸收

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`
- Modify/Create: Batch A file list above

- [ ] **Step 1: 复核清单里 Batch A 的文件集合没有混入 Batch B/C 热点**

Run:

```powershell
Get-Content "backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md"
```

Expected: Batch A 只包含 channel 独立能力块、独立 migration、独立 admin/channel UI/API，不包含 `billing_service.go`、`gateway_service.go`、`usage_log_repo.go`、`UsageView.vue` 等热点。

- [ ] **Step 2: 写一个保护性检查，确认本地关键语义文件在 Batch A 之外**

Run:

```powershell
$forbidden = @(
  'backend/internal/service/billing_service.go',
  'backend/internal/service/gateway_service.go',
  'backend/internal/service/openai_gateway_service.go',
  'backend/internal/repository/usage_log_repo.go',
  'frontend/src/views/user/UsageView.vue'
)

$report = Get-Content "backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md" -Raw
foreach ($f in $forbidden) {
  if ($report -match [regex]::Escape($f) -and $report -match 'Batch A') {
    throw "Forbidden hotspot leaked into Batch A: $f"
  }
}
```

Expected: no output / no exception.

- [ ] **Step 3: 按文件从 `origin/main` 吸收 Batch A**

执行方式：逐个文件从远端取内容，不做整批 merge。示例命令模式：

```powershell
git checkout origin/main -- backend/internal/repository/channel_repo.go
git checkout origin/main -- backend/internal/repository/channel_repo_pricing.go
git checkout origin/main -- backend/internal/repository/channel_repo_test.go
git checkout origin/main -- backend/internal/service/channel.go
git checkout origin/main -- backend/internal/service/channel_service.go
git checkout origin/main -- backend/internal/service/channel_service_test.go
git checkout origin/main -- backend/internal/service/channel_test.go
git checkout origin/main -- backend/internal/handler/admin/channel_handler.go
git checkout origin/main -- backend/internal/handler/admin/channel_handler_test.go
git checkout origin/main -- backend/internal/handler/admin/dashboard_handler.go
git checkout origin/main -- backend/internal/repository/wire.go
git checkout origin/main -- backend/internal/handler/wire.go
git checkout origin/main -- frontend/src/api/admin/channels.ts
git checkout origin/main -- frontend/src/components/admin/channel/IntervalRow.vue
git checkout origin/main -- frontend/src/components/admin/channel/ModelTagInput.vue
git checkout origin/main -- frontend/src/components/admin/channel/PricingEntryCard.vue
git checkout origin/main -- frontend/src/components/admin/channel/types.ts
git checkout origin/main -- frontend/src/views/admin/ChannelsView.vue
git checkout origin/main -- backend/migrations/081_create_channels.sql
git checkout origin/main -- backend/migrations/082_refactor_channel_pricing.sql
git checkout origin/main -- backend/migrations/083_channel_model_mapping.sql
```

Expected: 仅 Batch A 文件进入工作区改动，不带入热点文件。

- [ ] **Step 4: 跑最小验证，确认 Batch A 吸收后项目仍可编译/构建**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/handler -count=1
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run 'TestChannel' -count=1
pnpm build
```

Workdirs:
- backend tests: `backend`
- frontend build: `frontend`

Expected: all commands pass.

## Task 2: 吸收 Batch B 接线层改动，但不让它改写本地语义

**Files:**
- Modify/Create: Batch B file list above

- [ ] **Step 1: 先写失败/保护检查，锁住本地关键入口仍存在**

Run:

```powershell
git grep -n "sub2api-openai" -- frontend/src/components/keys/UseKeyModal.vue
git grep -n "priority_account_multiplier" -- frontend/src/views/user/UsageView.vue backend/internal/service/openai_gateway_service.go
git grep -n "TargetGroupExhausted\|TargetGroupActive" -- backend/internal/service backend/internal/handler
```

Expected: 三组 grep 都有结果，说明本地冻结语义的关键代码仍在。

- [ ] **Step 2: 吸收 Batch B 文件，但每次只带一组接线改动**

Run in sequence:

```powershell
git checkout origin/main -- backend/internal/server/routes/admin.go
git checkout origin/main -- backend/internal/service/wire.go
git checkout origin/main -- backend/cmd/server/wire_gen.go
git checkout origin/main -- frontend/src/api/admin/dashboard.ts
git checkout origin/main -- frontend/src/api/admin/index.ts
git checkout origin/main -- frontend/src/components/layout/AppSidebar.vue
git checkout origin/main -- frontend/src/router/index.ts
```

Expected: only Batch B files change.

- [ ] **Step 3: 手工回看接线改动，确认没有绕过本地冻结语义**

Required checks:
- `routes/admin.go` 没把上游 channel 页面路由插到会影响本地现有 admin 页面顺序的位置
- `wire.go` / `wire_gen.go` 没移除本地自定义 service/handler 依赖
- 前端侧不会把新的 channel 菜单入口覆盖掉本地已有页面导航逻辑

Expected: 所有发现的问题在这一批就修正，不留到后面。

- [ ] **Step 4: 再跑一次接线层验证**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/server -run TestNonExistent -count=1
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/handler -count=1
pnpm build
```

Expected: compile/build all pass.

## Task 3: 把 Batch C 从“热点名单”提升为逐文件移植设计

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`
- Create: `backend/docs/superpowers/specs/2026-04-05-sub2api-batch-c-hotspots-design.md`

- [ ] **Step 1: 先把 Batch C 热点逐个抽成独立对照条目**

必须覆盖：

```text
backend/internal/service/billing_service.go
backend/internal/service/gateway_service.go
backend/internal/service/openai_gateway_service.go
backend/internal/repository/usage_log_repo.go
backend/internal/handler/dto/types.go
backend/internal/handler/dto/mappers.go
backend/ent/schema/usage_log.go
frontend/src/views/admin/UsageView.vue
frontend/src/views/user/UsageView.vue
frontend/src/types/index.ts
```

- [ ] **Step 2: 为每个热点写 4 项固定字段**

在报告或新 spec 中用统一模板：

```md
### <file>
- Local semantics to preserve
- Upstream additions worth adopting
- Conflict points
- Recommended transplant strategy
```

Expected: 每个热点都不再只是“高风险”，而是有可执行的下一步动作。

- [ ] **Step 3: 把 Batch C 输出单独写成下一阶段设计文档**

Create:

```text
backend/docs/superpowers/specs/2026-04-05-sub2api-batch-c-hotspots-design.md
```

This spec must explain:
- 哪些热点建议拆分后吸收
- 哪些热点建议暂缓
- 哪些热点需要先统一 usage/billing schema 再动

## Task 4: 收尾并形成下一轮真实移植入口

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

- [ ] **Step 1: 回看当前计划是否真的只完成“吸收清单 + A/B 低中耦合吸收 + C 设计入口”**

确认没有越界去直接改 Batch C 热点主逻辑。

- [ ] **Step 2: 运行最终验证**

Run:

```powershell
git diff --check
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/handler -count=1
pnpm build
```

Expected: pass

- [ ] **Step 3: 在报告底部补一个“实际吸收结果”小节**

必须列出：
- Batch A 实际已吸收哪些文件
- Batch B 实际已吸收哪些文件
- Batch C 当前仍未动哪些文件

- [ ] **Step 4: 准备下一轮提问**

最终交付给用户时要明确问：
- 是现在就对 Batch A/B 已吸收内容提交
- 还是继续写 Batch C 的真正移植计划

## 自查清单

- Spec coverage:
  - 冻结本地语义基线：Task 2 + Task 3
  - Batch A/B/C 分层推进：Task 1 + Task 2 + Task 3
  - 第一轮先交付吸收清单再进真正移植：Task 3 + Task 4
- Placeholder scan: 无 `TODO/TBD/后续再看/大概`
- Type consistency:
  - 一律使用 `Batch A / Batch B / Batch C`
  - 一律使用 `merge-base = b384570de3545f036f250e68e9ca31362142dadf` 作为比较基线
