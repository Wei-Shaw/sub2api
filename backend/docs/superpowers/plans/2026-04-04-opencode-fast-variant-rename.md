# OpenCode Fast Variant Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把现有 OpenCode Fast 组合 variant 从 `fast-*` 重命名为 `*-fast`，同时保持 `serviceTier: "priority" + reasoningEffort` 的请求语义不变。

**Architecture:** 这是一轮纯命名重排，不新增模型、不改后端、不保留兼容双写。实现上只同步修改 `sub2api` 推荐配置生成器、对应前端测试，以及本机 `opencode.jsonc`，让 repo 内推荐配置和本机实际配置继续保持一致。

**Tech Stack:** Vue 3, Vitest, pnpm, local OpenCode JSONC config.

---

## 文件结构

- Modify: `frontend/src/components/keys/UseKeyModal.vue`
  推荐配置生成源，负责把 `fast-low` 等旧 key 改成 `low-fast` 等新 key。
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  锁定推荐配置输出契约，确保只生成新 key，且 payload 仍然是 `serviceTier: "priority"`。
- Modify: `C:\Users\34404\.config\opencode\opencode.jsonc`
  本机实际使用配置，同步进行同样的命名重排。
- Reference: `backend/docs/superpowers/specs/2026-04-04-opencode-fast-variant-rename-design.md`
  这轮命名重排的设计依据。

## 任务拆分

### Task 1: 锁定推荐配置输出契约

**Files:**
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 先把失败测试写到具体命名契约上**

在 `renders sub2api-openai provider config with Sys models in OpenCode example` 这一条测试里，把断言改成：

```ts
expect(gpt54Variants['low-fast'].serviceTier).toBe('priority')
expect(gpt54Variants['medium-fast'].serviceTier).toBe('priority')
expect(gpt54Variants['high-fast'].serviceTier).toBe('priority')
expect(gpt54Variants['xhigh-fast'].serviceTier).toBe('priority')
expect(gpt54Variants['xhigh-fast'].reasoningEffort).toBe('xhigh')
expect(gpt54SysVariants['xhigh-fast'].serviceTier).toBe('priority')
expect(gpt54SysVariants['xhigh-fast'].reasoningEffort).toBe('xhigh')
expect(gpt54Variants['fast-low']).toBeUndefined()
expect(gpt54SysVariants['fast-low']).toBeUndefined()
```

同时保留：

- `gpt-5.2` 下不应出现 Fast 组合项
- `sub2api-openai` provider 不应退回官方 `openai` provider

- [ ] **Step 2: 跑测试确认 RED**

Run:

```powershell
pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"
```

Workdir: `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend`

Expected: FAIL，且失败点明确是当前输出里仍然存在旧 key `fast-low`、不存在新 key `low-fast`。

### Task 2: 修改推荐配置生成器

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 在生成器里把命名方向从 `fast-*` 改成 `*-fast`**

把当前生成 Fast 组合项的逻辑从：

```ts
`fast-${level}`
```

改成：

```ts
`${level}-fast`
```

并确保：

```ts
{
  ...variant,
  serviceTier: 'priority'
}
```

这层 payload 完全不变，只改 key 名。

- [ ] **Step 2: 确认 Fast 组合项仍然只作用于 `gpt-5.4` 基础模型**

保留当前范围，不把 Fast 组合项扩散到：

- `gpt-5.2`
- `gpt-5.4-mini`
- `gpt-5.4-nano`

同时因为现有 `withSysVariants(...)` 会克隆 `gpt-5.4` 配置，`gpt-5.4-Sys` 会自动继承 `low-fast / medium-fast / high-fast / xhigh-fast`。

- [ ] **Step 3: 重新跑测试确认 GREEN**

Run:

```powershell
pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"
```

Expected: PASS。

### Task 3: 同步修改本机 OpenCode 配置

**Files:**
- Modify: `C:\Users\34404\.config\opencode\opencode.jsonc`

- [ ] **Step 1: 在本机配置里只改 `gpt-5.4 / gpt-5.4-Sys` 的 variant key**

把这两段下的：

```json
"fast-low"
"fast-medium"
"fast-high"
"fast-xhigh"
```

改成：

```json
"low-fast"
"medium-fast"
"high-fast"
"xhigh-fast"
```

保留原值不变：

- `reasoningEffort`
- `reasoningSummary: "auto"`
- `include: ["reasoning.encrypted_content"]`
- `serviceTier: "priority"`

- [ ] **Step 2: 确认 `gpt-5.2 / gpt-5.2-Sys` 的误挂载不会重新出现**

修改后应满足：

- `gpt-5.2` 下没有 `low-fast`
- `gpt-5.2-Sys` 下没有 `low-fast`
- `gpt-5.4` / `gpt-5.4-Sys` 下才有 `low-fast` 等组合项

- [ ] **Step 3: 用文本检查看本机配置是否符合预期**

Run:

```powershell
Get-Content "C:\Users\34404\.config\opencode\opencode.jsonc" | Select-String 'gpt-5.2|gpt-5.4|low-fast|medium-fast|high-fast|xhigh-fast|fast-low|serviceTier'
```

Expected:

- 能看到 `gpt-5.4` / `gpt-5.4-Sys` 下出现新 key
- 看不到 `fast-low`
- `gpt-5.2` 段里没有 Fast 组合项

### Task 4: 做完整验证并整理交付

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- Modify: `C:\Users\34404\.config\opencode\opencode.jsonc`

- [ ] **Step 1: 跑前端构建**

Run:

```powershell
pnpm build
```

Workdir: `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend`

Expected: PASS。

- [ ] **Step 2: 用新 variant 发一条真实请求**

Run:

```powershell
opencode run -m sub2api-openai/gpt-5.4 --variant xhigh-fast "Reply only with OK."
```

Workdir: `C:\Users\34404`

Expected:

- 返回 `OK`
- 不出现 `Unsupported service_tier: fast`

- [ ] **Step 3: 总结最终状态**

必须明确说明：

1. 新 variant key 是 `low-fast / medium-fast / high-fast / xhigh-fast`
2. 旧 key `fast-low ...` 已移除
3. 请求语义仍然是 `serviceTier: "priority"` + 对应 `reasoningEffort`
4. 本机 `opencode.jsonc` 已同步

## 自查清单

- Spec 覆盖：
  - 只改 key 名，不改 payload：Task 2 + Task 3
  - 本机配置与推荐配置同步：Task 2 + Task 3
  - 旧 key 不再出现：Task 1 + Task 3
  - 真请求验证：Task 4
- 占位词检查：没有 `TODO/TBD/implement later` 之类占位描述。
- 命名一致性：全文统一使用 `low-fast / medium-fast / high-fast / xhigh-fast`，不再混用 `fast-low` 作为目标状态。
