# sub2api Upstream Follow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 先产出一份以本地设计为基线的上游结构化吸收清单，把 `origin/main` 的增量拆成可机械吸收、需人工移植、应暂缓三类，为后续真正跟进上游改动建立可执行边界。

**Architecture:** 先固定 `merge-base`，分别抽取本地与上游自该点以来的文件级变更，再按 `Batch A / B / C` 分类。最终交付不是代码合并，而是一份带文件清单、冲突点、保留语义和建议动作的结构化报告，用它作为后续真正移植计划的输入。

**Tech Stack:** git, PowerShell, local repo analysis, markdown design/report writing.

---

## 文件边界

- Read: `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.git`
- Read: `backend/docs/superpowers/specs/2026-04-05-sub2api-upstream-follow-design.md`
- Create: `backend/docs/superpowers/plans/2026-04-05-sub2api-upstream-follow.md`
- Create: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

## Task 1: 固定对比基线并导出本地/上游差异清单

**Files:**
- Create: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

- [ ] **Step 1: 写一个最小验证命令，先锁定 merge-base 与当前分叉状态**

Run:

```powershell
git rev-parse HEAD
git rev-parse origin/main
git merge-base HEAD origin/main
git status -sb
```

Expected: 明确记录 `HEAD`、`origin/main`、`merge-base`，并确认当前分叉状态不是简单 ahead/behind。

- [ ] **Step 2: 导出自 merge-base 以来的本地文件集合与上游文件集合**

Run:

```powershell
$base = git merge-base HEAD origin/main
git diff --name-only "$base..HEAD"
git diff --name-only "$base..origin/main"
```

Expected: 拿到本地独有、上游独有、以及后续可算交集的文件级输入。

- [ ] **Step 3: 计算三类集合：Overlap / LocalOnly / RemoteOnly**

Run:

```powershell
$base = git merge-base HEAD origin/main
$local = git diff --name-only "$base..HEAD"
$remote = git diff --name-only "$base..origin/main"

$overlap = Compare-Object $local $remote -IncludeEqual -ExcludeDifferent |
  Where-Object { $_.SideIndicator -eq '==' } |
  Select-Object -ExpandProperty InputObject

$localOnly = Compare-Object $local $remote -PassThru |
  Where-Object { $_ -in $local -and $_ -notin $remote }

$remoteOnly = Compare-Object $local $remote -PassThru |
  Where-Object { $_ -in $remote -and $_ -notin $local }
```

Expected: 3 组可直接写入报告的文件集合。

- [ ] **Step 4: 把这三组结果写进吸收清单报告初稿**

报告必须至少包含以下标题：

```md
# sub2api Upstream Absorption Inventory

## Baseline
- HEAD: <sha>
- origin/main: <sha>
- merge-base: <sha>

## Overlap
<file list>

## Local Only
<file list>

## Remote Only
<file list>
```

Expected: 报告文件存在并含上述 4 节。

## Task 2: 按 Batch A / B / C 对上游变更做结构化分类

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

- [ ] **Step 1: 在报告里新增 Batch A / B / C 三节骨架**

写入以下标题：

```md
## Batch A — Low Coupling
## Batch B — Medium Coupling
## Batch C — High-Risk Hotspots
```

- [ ] **Step 2: 将 RemoteOnly 和 Overlap 中文件按批次归类**

归类规则：
- Batch A：独立 channel 文件、独立 migration、独立 admin API/前端组件
- Batch B：routes / DTO / handler 接线层
- Batch C：`billing_service.go`、`gateway_service.go`、`usage_log_repo.go`、`UsageView.vue`、`handler/dto`、`ent/schema/usage_log.go` 等热点

Expected: 每个文件都至少落到一批，或明确标注“暂缓/不纳入当前跟进”。

- [ ] **Step 3: 对 Batch A / B 中每个条目写一句建议动作**

每个条目至少标注其建议动作之一：
- `机械吸收`
- `轻度人工接线`
- `暂缓`

示例：

```md
- `backend/internal/service/channel_service.go` — Batch A — 机械吸收（独立能力）
- `backend/internal/server/routes/admin.go` — Batch B — 轻度人工接线（需要保留本地路由顺序）
```

## Task 3: 对 Batch C 热点逐文件写冲突说明

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

- [ ] **Step 1: 为每个 Batch C 热点新增固定模板说明**

每个热点文件必须写成下面格式：

```md
### <file path>
- Local semantics to preserve:
  - ...
- Upstream additions:
  - ...
- Conflict points:
  - ...
- Recommendation:
  - manual transplant / defer / split before transplant
```
```

- [ ] **Step 2: 至少覆盖这些热点文件**

必须逐项覆盖：

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

- [ ] **Step 3: 在每个热点里明确“本地必须保留”的语义**

至少要写出并反复沿用这三条本地优先语义：

```text
OpenAI active/exhausted 路由与 -Sys 语义
当前 billing/usage 的优先账号倍率 + priority 单价来源 + 用户可见价格因子口径
sub2api-openai / OpenCode 推荐配置独立 provider 语义
```

Expected: Batch C 不只是文件列表，而是有明确的冲突解释与建议动作。

## Task 4: 自查并形成下一阶段执行入口

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

- [ ] **Step 1: 回看 spec，确认报告覆盖了三类目标**

逐项对照 spec：
- 本地语义冻结是否明确
- Batch A/B/C 是否齐全
- 是否先交付结构化吸收清单，而不是直接合并

- [ ] **Step 2: 检查报告里有没有这些禁止项**

不得出现：
- `TODO`
- `TBD`
- `后续再看`
- `大概`
- `可能可以`

每一个批次判断都必须落到明确动作。

- [ ] **Step 3: 在报告最后补一个“下一阶段建议顺序”小节**

写出类似：

```md
## Recommended Next Order
1. Batch A mechanical absorption
2. Batch B wiring absorption
3. Batch C per-file transplant plan
```

- [ ] **Step 4: 用一句话总结当前结论**

报告最后必须明确写出：

```md
Current recommendation: do not merge origin/main directly; follow the inventory batch order above.
```

## 自查清单

- Spec coverage:
  - 不直接 merge：Task 1 + Task 4
  - 冻结本地设计语义：Task 3
  - Batch A/B/C：Task 2 + Task 3
  - 第一阶段交付结构化吸收清单：Task 1-4
- Placeholder scan: 无 `TODO/TBD/大概/后续再看`
- Type consistency:
  - 始终使用 `Batch A / Batch B / Batch C`
  - 始终以 `merge-base` 为同一比较基线
