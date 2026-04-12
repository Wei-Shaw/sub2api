# Token Display Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一用户端与管理端的 token 展示口径，让“总输入（含缓存）/净输入/总 token”在表格、tooltip、summary 和导出中保持一致。

**Architecture:** 保持后端 DTO、SQL 和计费逻辑不变，只在前端展示层本地计算新的展示值：`displayInputTokens = input_tokens + cache_read_tokens + cache_creation_tokens`，`netInputTokens = input_tokens`，`displayTotalTokens = displayInputTokens + output_tokens`。图片请求和 cache TTL 细分明细保持现有分支不变，只收口 token 请求的展示语义。

**Tech Stack:** Vue 3, TypeScript, pnpm, existing admin/user usage views and tooltips

---

### Task 1: 抽出统一的 token 展示辅助函数

**Files:**
- Create: `frontend/src/utils/usageTokens.ts`
- Test: `frontend/src/utils/__tests__/usageTokens.spec.ts`

- [ ] **Step 1: 写失败测试，锁住统一口径**

```ts
import { describe, expect, it } from 'vitest'
import { buildUsageTokenDisplay } from '../usageTokens'

describe('buildUsageTokenDisplay', () => {
  it('computes display input and total tokens including cache tokens', () => {
    const result = buildUsageTokenDisplay({
      input_tokens: 100,
      output_tokens: 20,
      cache_read_tokens: 30,
      cache_creation_tokens: 10,
    })

    expect(result.netInputTokens).toBe(100)
    expect(result.displayInputTokens).toBe(140)
    expect(result.displayTotalTokens).toBe(160)
  })

  it('keeps zero-safe behaviour', () => {
    const result = buildUsageTokenDisplay({
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_creation_tokens: 0,
    })

    expect(result.netInputTokens).toBe(0)
    expect(result.displayInputTokens).toBe(0)
    expect(result.displayTotalTokens).toBe(0)
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run frontend/src/utils/__tests__/usageTokens.spec.ts`

Expected: FAIL，提示 `buildUsageTokenDisplay` 未定义。

- [ ] **Step 3: 写最小实现**

```ts
export interface UsageTokenLike {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
}

export function buildUsageTokenDisplay(row: UsageTokenLike) {
  const netInputTokens = row.input_tokens || 0
  const cacheReadTokens = row.cache_read_tokens || 0
  const cacheCreationTokens = row.cache_creation_tokens || 0
  const outputTokens = row.output_tokens || 0
  const displayInputTokens = netInputTokens + cacheReadTokens + cacheCreationTokens
  const displayTotalTokens = displayInputTokens + outputTokens

  return {
    netInputTokens,
    displayInputTokens,
    displayTotalTokens,
  }
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run frontend/src/utils/__tests__/usageTokens.spec.ts`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/utils/usageTokens.ts frontend/src/utils/__tests__/usageTokens.spec.ts
git commit -m "refactor(usage): 抽出统一 token 展示计算"
```

### Task 2: 收口管理端表格与 tooltip

**Files:**
- Modify: `frontend/src/components/admin/usage/UsageTable.vue`
- Test: `frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`

- [ ] **Step 1: 写失败测试，锁定管理端 token 展示口径**

先扩展当前 `DataTableStub`，让它真正渲染 `cell-tokens` slot；否则现有测试骨架看不到 token 主格子。

在 `UsageTable.spec.ts` 增加一条用例，构造：

```ts
{
  input_tokens: 100,
  output_tokens: 20,
  cache_read_tokens: 30,
  cache_creation_tokens: 10,
}
```

断言：

- 主格子显示 `140` 作为输入主值
- 次级文案显示 `净输入` 和 `100`
- tooltip 的 `总 Tokens` 显示 `160`

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`

Expected: FAIL，当前实现仍把 `100` 当输入主值。

- [ ] **Step 3: 改 `UsageTable.vue`**

实现要点：

```ts
import { buildUsageTokenDisplay } from '@/utils/usageTokens'
```

在 token 主格子里：

- 输入主值改成 `buildUsageTokenDisplay(row).displayInputTokens`
- 在其下方增加次级小字：`净输入：{{ buildUsageTokenDisplay(row).netInputTokens }}`
- cache read / cache creation 第二行保留不变

在 token tooltip 里：

- 输入项标题改为“总输入 Tokens”
- 新增“净输入 Tokens”一行
- `总 Tokens` 使用 `displayTotalTokens`

图片请求分支和 cache TTL 细分明细保持现状不变。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/admin/usage/UsageTable.vue frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts
git commit -m "fix(admin-usage): 统一 token 主值与 tooltip 口径"
```

### Task 3: 收口用户端表格与 tooltip

**Files:**
- Modify: `frontend/src/views/user/UsageView.vue`
- Test: `frontend/src/views/user/__tests__/UsageView.spec.ts`

- [ ] **Step 1: 写失败测试**

先扩展 `UsageView.spec.ts` 里的 `TablePageLayoutStub`，让它真正渲染 `#table` slot；否则现有测试骨架看不到用户端表格和 token tooltip。

新增一条用户端 token 展示用例，断言：

- 表格输入主值显示总输入
- 次级位置显示净输入
- tooltip 显示总输入 / 净输入 / 输出 / cache read / cache creation / 总 token 的完整拆分

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run frontend/src/views/user/__tests__/UsageView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 改 `UsageView.vue`**

与管理端同口径：

- 主格子输入主值使用 `displayInputTokens`
- 附近次级显示 `净输入`
- tooltip 中总输入/净输入/总 token 全部改成新公式

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run frontend/src/views/user/__tests__/UsageView.spec.ts`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/views/user/UsageView.vue frontend/src/views/user/__tests__/UsageView.spec.ts
git commit -m "fix(user-usage): 统一 token 展示口径"
```

### Task 4: 收口 summary 与导出

**Files:**
- Modify: `frontend/src/components/admin/usage/UsageStatsCards.vue`
- Modify: `frontend/src/views/admin/UsageView.vue`
- Modify: `frontend/src/views/user/UsageView.vue`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Test: `frontend/src/views/admin/__tests__/UsageView.spec.ts`
- Test: `frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts`
- Test: `frontend/src/views/user/__tests__/UsageView.spec.ts`

- [ ] **Step 1: 写失败测试，锁住 summary / 导出口径**

在 `frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts` 增加断言：

- summary 中“输入”展示使用 `total_input_tokens + total_cache_tokens`
- summary 中“总 token”继续直接使用现有 `total_tokens`
- 次级文案包含“净输入”

在 `frontend/src/views/admin/__tests__/UsageView.spec.ts` 增加断言：

- 导出行包含：总输入、净输入、输出、cache read、cache creation、总 token

在 `frontend/src/views/user/__tests__/UsageView.spec.ts` 增加断言：

- 用户端 summary 的输入主值使用 `total_input_tokens + total_cache_tokens`
- 用户端导出（当前页面已有）也同步输出：总输入、净输入、输出、cache read、cache creation、总 token

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run frontend/src/views/admin/__tests__/UsageView.spec.ts`

Expected: FAIL。

- [ ] **Step 3: 修改 summary / 导出 / 文案**

实现要点：

- `UsageStatsCards.vue`
  - summary “输入” 改成 `stats.total_input_tokens + stats.total_cache_tokens`
  - summary “总 token”继续直接使用现有 `stats.total_tokens`
  - 次级文案补 `净输入`
- `admin/UsageView.vue`
  - 导出列补齐：总输入、净输入、总 token
  - 生成导出行时使用与表格一致的前端本地计算
- `user/UsageView.vue`
  - 顶部 summary 的输入/总 token 口径同步调整
  - 用户侧导出也同步改成：总输入、净输入、输出、cache read、cache creation、总 token
- `en.ts` / `zh.ts`
  - 新增或调整：
    - `totalInputTokens`
    - `netInputTokens`
    - `grossInputTokens`
  - 不改已有 cache 相关标签含义

- [ ] **Step 4: 运行完整前端验证**

Run:

```bash
pnpm vitest run frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts frontend/src/views/user/__tests__/UsageView.spec.ts frontend/src/views/admin/__tests__/UsageView.spec.ts
pnpm typecheck
```

Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/admin/usage/UsageStatsCards.vue frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts frontend/src/views/admin/UsageView.vue frontend/src/views/user/UsageView.vue frontend/src/views/user/__tests__/UsageView.spec.ts frontend/src/i18n/locales/en.ts frontend/src/i18n/locales/zh.ts frontend/src/views/admin/__tests__/UsageView.spec.ts
git commit -m "fix(usage): 统一 token summary 与导出口径"
```

### Task 5: 最终收尾验证

- [ ] **Step 1: 跑最终验证**

Run:

```bash
pnpm vitest run frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts frontend/src/components/admin/usage/__tests__/UsageStatsCards.spec.ts frontend/src/views/user/__tests__/UsageView.spec.ts frontend/src/views/admin/__tests__/UsageView.spec.ts
pnpm typecheck
git diff --check
```

Expected: 全部通过。

- [ ] **Step 2: 仅在需要时补一条简短记录**

本次不新增任何文档变更。

- [ ] **Step 3: 确认结束**

向用户汇报：

- 管理端表格/tooltip、用户端表格/tooltip、summary、导出已统一口径
- 后端未改动
- 前端验证已通过
