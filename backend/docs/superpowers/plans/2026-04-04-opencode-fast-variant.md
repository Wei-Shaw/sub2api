# OpenCode Fast Variant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为本机 `sub2api-openai` OpenCode 配置和 `sub2api` 前端导出的推荐配置补上 `gpt-5.4` / `gpt-5.4-Sys` 的 Fast 组合 variant，并确保这些 variant 在线上真正落成 `serviceTier: "priority"`。

**Architecture:** 延续当前已有的单一 OpenCode 模型清单思路，在 `gpt-5.4` / `gpt-5.4-Sys` 现有 reasoning variants 之上派生 `fast-low/fast-medium/fast-high/fast-xhigh`，而不是再引入新的模型 ID。配置层仍然使用 Fast 语义，但 wire-level payload 统一写入 `serviceTier: "priority"` 与对应 `reasoningEffort`，同时分别在本机 `opencode.jsonc` 和 `sub2api` 前端推荐配置里保持一致。

**Tech Stack:** OpenCode JSONC config, Vue 3 frontend config generator, Vitest, Bun/Node tooling, real `/v1/responses` verification through `sub2api`.

---

## 文件结构

- `C:\Users\34404\.config\opencode\opencode.jsonc`
  本机实际使用的 OpenCode 配置，需要补 `gpt-5.4` / `gpt-5.4-Sys` 的 `fast-*` 组合 variant。
- `C:\Users\34404\.config\opencode\opencode.jsonc.bak-2026-04-04`
  现有本机配置备份，作为本次本机配置修改的回滚点。
- `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend\src\components\keys\UseKeyModal.vue`
  `sub2api` 前端 OpenCode 推荐配置的生成源，需要派生 Fast variant 并输出 `serviceTier: "priority"`。
- `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend\src\components\keys\__tests__\UseKeyModal.spec.ts`
  锁定推荐配置文本输出的前端回归测试。
- `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\backend\docs\superpowers\specs\2026-04-04-opencode-fast-variant-design.md`
  这次 Fast variant 设计依据。

## 任务拆分

### Task 1: 先写失败测试锁定推荐配置里的 Fast variant 输出

**Files:**
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 在推荐配置测试里先加失败断言**

在 `UseKeyModal.spec.ts` 里补一条用例，至少断言生成出来的 OpenCode 示例文本里包含：

```text
"fast-low"
"fast-medium"
"fast-high"
"fast-xhigh"
"serviceTier": "priority"
"reasoningEffort": "xhigh"
```

并且只要求这组 Fast 组合出现在：

```text
gpt-5.4
gpt-5.4-Sys
```

- [ ] **Step 2: 运行测试确认它先失败**

Run:

```powershell
pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"
```

Workdir: `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend`

Expected: FAIL，原因是当前推荐配置里还没有 `fast-*` 组合 variant 或 `serviceTier: "priority"`。

### Task 2: 修改 `sub2api` 推荐配置生成源，派生 Fast 组合 variant

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 在现有 OpenCode OpenAI 模型清单生成逻辑里新增 Fast 组合派生函数**

要求：

1. 不新增新的模型 ID。
2. 只对 `gpt-5.4` 和 `gpt-5.4-Sys` 增加：
   - `fast-low`
   - `fast-medium`
   - `fast-high`
   - `fast-xhigh`
3. 这些组合 variant 必须写出：

```json
{
  "serviceTier": "priority",
  "reasoningEffort": "xhigh",
  "reasoningSummary": "auto",
  "include": ["reasoning.encrypted_content"]
}
```

其中 `reasoningEffort` 根据具体 variant 名变化。

- [ ] **Step 2: 保证现有普通 reasoning variants 不被破坏**

要求：

1. `low/medium/high/xhigh` 原有 variants 继续存在。
2. `fast-*` 是在它们基础上新增，而不是替换原 variants。
3. `-Sys` 模型要继承同样的 Fast 组合逻辑。

- [ ] **Step 3: 重新运行测试确认转绿**

Run:

```powershell
pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"
```

Expected: PASS。

### Task 3: 修改本机 `opencode.jsonc`，补齐实际可用的 Fast variant

**Files:**
- Modify: `C:\Users\34404\.config\opencode\opencode.jsonc`
- Reference: `C:\Users\34404\.config\opencode\opencode.jsonc.bak-2026-04-04`

- [ ] **Step 1: 先确认备份文件存在，可用于回滚**

Run:

```powershell
Test-Path 'C:\Users\34404\.config\opencode\opencode.jsonc.bak-2026-04-04'
```

Expected: `True`。

- [ ] **Step 2: 只在本机配置里补 `gpt-5.4` / `gpt-5.4-Sys` 的 Fast 组合 variant**

要求：

1. 不改已有 provider id：仍然是 `sub2api-openai`。
2. 不扩展到其他模型。
3. 为 `gpt-5.4` 和 `gpt-5.4-Sys` 增加：
   - `fast-low`
   - `fast-medium`
   - `fast-high`
   - `fast-xhigh`
4. 每个 Fast 组合 variant 都必须包含：

```json
"serviceTier": "priority"
```

以及对应的：

```json
"reasoningEffort": "low|medium|high|xhigh"
```

- [ ] **Step 3: 用 OpenCode CLI 直接验证本机配置结构可见**

Run:

```powershell
opencode models sub2api-openai --verbose
```

Expected: 输出中能看到 `gpt-5.4` / `gpt-5.4-Sys` 的 `fast-*` variants，并包含 `serviceTier: "priority"`。

### Task 4: 跑构建与真实请求验证，确认 `priority` 线语义正常

**Files:**
- Modify: none (verification only)
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 跑前端构建，确认推荐配置改动不破坏构建**

Run:

```powershell
pnpm build
```

Workdir: `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend`

Expected: PASS。

- [ ] **Step 2: 通过本机 OpenCode 新 Fast variant 发一条真实请求**

建议命令（按实际 OpenCode CLI 支持情况调整）：

```powershell
opencode run -m sub2api-openai/gpt-5.4 --variant fast-xhigh "Reply only with OK."
```

Expected:

1. 请求成功，不出现 `Unsupported service_tier: fast`。
2. 响应正常返回。

- [ ] **Step 3: 如有必要，再用线上 API / 记录核对 wire-level 行为**

至少确认：

1. 请求没有再因 `service_tier: fast` 被拒绝。
2. reasoning effort 仍是期望值（如 `xhigh`）。
3. 如果响应或记录里有 tier 相关字段，核对它是否与 `priority` 兼容。

- [ ] **Step 4: 输出本轮交付与限制说明**

总结必须明确说明：

1. 这轮只给 `gpt-5.4` / `gpt-5.4-Sys` 增了 Fast 组合 variant。
2. 用户入口仍然叫 Fast，但 wire-level 发的是 `priority`。
3. 这轮没有修改后端去兼容手工传入 `service_tier = fast` 的客户端。

## 自查清单

- Spec 覆盖：
  - `fast-*` 组合 variant：Task 1 + Task 2 + Task 3
  - 只覆盖 `gpt-5.4` / `gpt-5.4-Sys`：Task 1 + Task 3
  - wire-level `priority` 而不是 `fast`：Task 1 + Task 2 + Task 3 + Task 4
  - 本机配置与推荐配置保持一致：Task 2 + Task 3
  - 真实请求验证：Task 4
- 占位词检查：没有 `TODO/TBD/implement later` 等占位描述。
- 命名一致性：Fast 组合 variant 统一为 `fast-low/fast-medium/fast-high/fast-xhigh`，不混用 `priority-*` 或 `gpt-5.4-fast-*` 之类命名。
