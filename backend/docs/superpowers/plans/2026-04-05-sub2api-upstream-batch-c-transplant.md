# sub2api Upstream Batch C Transplant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不破坏本地主线 routing / billing / opencode 语义的前提下，为上游 Batch C 热点建立可执行的分阶段移植顺序，并只在每一阶段验证通过后推进下一阶段。

**Architecture:** Batch C 不做大 merge，而是按“数据结构先统一、执行链后迁入、展示层最后对齐”的顺序推进。先统一 usage schema 与 repository，再对 billing/gateway/openai execution chain 做概念级移植，最后再吸收 DTO 与 admin/user usage 展示层增量。

**Tech Stack:** git, Go, Ent schema generation, SQL migrations, Vue/TypeScript UI, markdown hotspot design docs.

---

## 文件边界

- Read: `backend/docs/superpowers/specs/2026-04-05-sub2api-batch-c-hotspots-design.md`
- Read: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`
- Create: `backend/docs/superpowers/plans/2026-04-05-sub2api-upstream-batch-c-transplant.md`
- Batch C hotspot files:
  - `backend/internal/service/billing_service.go`
  - `backend/internal/service/pricing_service.go`
  - `backend/internal/service/model_pricing_resolver.go`
  - `backend/internal/service/gateway_service.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/repository/usage_log_repo.go`
  - `backend/internal/service/usage_log.go`
  - `backend/internal/handler/dto/types.go`
  - `backend/internal/handler/dto/mappers.go`
  - `backend/ent/schema/usage_log.go`
  - `frontend/src/views/admin/UsageView.vue`
  - `frontend/src/views/user/UsageView.vue`
  - `frontend/src/types/index.ts`
  - `frontend/src/i18n/locales/en.ts`
  - `frontend/src/i18n/locales/zh.ts`

## Task 1: 统一目标 `usage_log` 结构，再谈逻辑迁移

**Files:**
- Modify: `backend/ent/schema/usage_log.go`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/service/usage_log.go`
- Modify: `backend/docs/superpowers/specs/2026-04-05-sub2api-batch-c-hotspots-design.md`

- [ ] **Step 1: 先写一份 `usage_log` 目标字段矩阵**

在热点设计文档里新增一个小节，按三列整理：

```md
| Field | Keep local | Adopt upstream | Unified rule |
```

至少覆盖：

```text
routing_* fields
priority_account_multiplier
effective_* unit price fields
pricing_source
billing_mode / request_type / stream / image_output_tokens / channel billing source fields
```

Expected: 有一张能指导 schema 合并的字段矩阵，而不是边改边想。

- [ ] **Step 2: 基于字段矩阵，先写失败测试锁定最终 scan/insert 列顺序**

Target tests to add/update:

```text
backend/internal/repository/usage_log_repo_request_type_test.go
```

必须覆盖：
- 本地 billing breakdown 字段仍在
- 上游 usage billing mode / image output token 字段也能进入最终列顺序
- 没有因为 merge 造成列位错乱或 NULL 落库

- [ ] **Step 3: 跑 RED，确认 schema/repo 还没满足统一结构**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/repository -run 'UsageLog' -count=1
```

Expected: FAIL，且失败点直接指向列顺序/字段缺失/scan 不一致，而不是别的无关问题。

- [ ] **Step 4: 再做最小实现，让 repository / service / ent schema 三处对齐**

要求：
- 不在这个任务里碰 `billing_service.go` / `gateway_service.go` 逻辑
- 只让 `usage_log` 结构本身先变成可承载两边字段的统一形态

- [ ] **Step 5: 跑 GREEN，确认 `usage_log` 结构层稳定**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/repository -count=1
```

Expected: PASS

## Task 2: 把 billing / pricing 执行链按“本地口径优先”移植上游增量

**Files:**
- Modify: `backend/internal/service/billing_service.go`
- Modify: `backend/internal/service/pricing_service.go`
- Modify: `backend/internal/service/model_pricing_resolver.go`
- Modify: `backend/internal/service/gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service.go`

- [ ] **Step 1: 写失败测试锁定本地 billing 基线不能被回退**

Tests must protect:

```text
priority is pricing-source only, not extra Fast multiplier
subscription accumulates actual_cost
priority_account_multiplier stays intact
OpenAI active/exhausted and -Sys semantics stay intact while billing data is recorded
```

- [ ] **Step 2: 跑 RED，确认吸收上游前这些关键语义会被破坏**

Run focused suites such as:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run 'Billing|RecordUsage|Sys|TargetGroup' -count=1
```

Expected: FAIL after introducing the needed upstream pieces, proving tests are meaningful.

- [ ] **Step 3: 逐概念吸收，而不是整文件替换**

吸收顺序必须是：

1. channel pricing decision helpers
2. billing model source tagging
3. image output token accounting
4. cache/prompt optimizations

禁止直接把 upstream `billing_service.go` / `gateway_service.go` 整文件 checkout 覆盖本地版本。

- [ ] **Step 4: 跑 GREEN，确认执行链仍然保持本地语义**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -count=1
```

Expected: PASS

## Task 3: 让 DTO / Admin Usage / User Usage 吸收上游增量但不覆盖本地解释层

**Files:**
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/admin/UsageView.vue`
- Modify: `frontend/src/views/user/UsageView.vue`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`

- [ ] **Step 1: 先定义页面信息架构优先级**

写在任务注释里并严格遵守：

```text
用户页优先展示价格原因与倍率解释
管理页优先展示路由/模型映射/审计明细
```

- [ ] **Step 2: 写失败测试锁定现有用户解释层不被上游 UI 回退**

至少覆盖：
- `frontend/src/views/user/__tests__/UsageView.spec.ts`
- 如果需要，新增 admin usage 对应测试文件

Expected: RED when upstream-style UI pieces overwrite current local explanation layout.

- [ ] **Step 3: 有选择地移植上游 UI 增量**

只吸收对当前页面有净增益的部分，例如：
- 三层模型映射显示（admin）
- image token breakdown（如果与当前本地页面不冲突）
- billing mode 辅助标签（仅当不冲掉本地价格解释）

禁止整体替换 `UsageView.vue`。

- [ ] **Step 4: 跑前端 GREEN 验证**

Run:

```powershell
pnpm test:run "src/views/user/__tests__/UsageView.spec.ts"
pnpm build
```

Expected: PASS

## Task 4: 做 Batch C 汇总验证并准备下一轮提交

**Files:**
- Modify: `backend/docs/superpowers/reports/2026-04-05-sub2api-upstream-absorption-inventory.md`

- [ ] **Step 1: 在吸收清单报告里补一节 `Batch C actual changes`**

必须列出：
- 哪些热点已吸收
- 哪些热点仍暂缓
- 哪些上游功能被刻意放弃以保留本地语义

- [ ] **Step 2: 跑整体验证**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -count=1
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/repository -count=1
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/handler/dto -count=1
pnpm build
git diff --check
```

Expected: all pass.

- [ ] **Step 3: 交付时明确指出这是一轮“批量移植后的最终语义”还是“仍保留部分 defer”**

输出必须明确说明：
- Batch C 是否全部完成
- 还剩哪些上游能力没有纳入

## 自查清单

- Spec coverage:
  - schema 先统一：Task 1
  - execution chain 再迁：Task 2
  - DTO/UI 最后对齐：Task 3
  - 最终汇总验证：Task 4
- Placeholder scan: 无 `TODO/TBD/后续再看`
- Type consistency:
  - 始终以本地 frozen semantics 为优先
  - Batch C 的 4 个任务顺序不可打乱
