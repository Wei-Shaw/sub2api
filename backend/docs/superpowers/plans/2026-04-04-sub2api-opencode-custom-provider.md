# sub2api OpenCode 独立 Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 OpenCode 配置一个不占用官方 `openai` provider 的 `sub2api-openai` 独立 provider，并同步把 `sub2api` 前端导出的 OpenCode 示例改成同一结构且补齐 `-Sys` 模型。

**Architecture:** 本次实现分成两条线并保持同一模型源：一条直接修改用户本机 `~/.config/opencode/opencode.jsonc`，新增 `sub2api-openai` provider；另一条修改 `sub2api` 前端 `UseKeyModal` 的 OpenCode 示例生成逻辑，让它输出相同 provider id 和相同的模型/`-Sys` 别名清单。模型定义应集中到一处，通过基础模型列表派生 `-Sys` 变体，避免本机配置和前端示例再次漂移。

**Tech Stack:** JSONC 配置、Vue 3、Vitest、TypeScript、pnpm、OpenCode 配置 schema。

---

## 文件结构

- `C:\Users\34404\.config\opencode\opencode.jsonc`
  用户级 OpenCode 配置文件；新增 `provider.sub2api-openai`。
- `frontend/src/components/keys/UseKeyModal.vue`
  当前 OpenCode 示例来源；需要改成输出独立 provider，并把 OpenAI/OpenAI Sys 模型清单集中到一处。
- `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  锁定 OpenCode 示例中 provider id 与 `-Sys` 模型输出。
- `frontend/src/i18n/locales/zh.ts`
  如 hint/title 需要补充“自定义 provider”说明，在这里更新中文文案。
- `frontend/src/i18n/locales/en.ts`
  与中文文案同步更新英文版本。

## 实施顺序

先用最小 TDD 改 `sub2api` 的 OpenCode 示例生成逻辑，锁定独立 provider 和 `-Sys` 输出；然后再落地用户本机 `opencode.jsonc`，最后统一做前端构建和本机配置读取验证。这样即使本机配置验证失败，也不会把仓库里的前端示例改坏。

### Task 1: 锁定 `sub2api` OpenCode 示例必须输出独立 provider 和 `-Sys` 模型

**Files:**
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- Modify: `frontend/src/components/keys/UseKeyModal.vue`

- [ ] **Step 1: 先写失败测试，锁定 provider id 与 `-Sys` 模型**

在 `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` 里新增一个测试，不要复用现有断言名字：

```ts
it('renders sub2api-openai provider config with Sys models in OpenCode example', async () => {
  const wrapper = mount(UseKeyModal, {
    props: {
      show: true,
      apiKey: 'sk-test',
      baseUrl: 'https://example.com/v1',
      platform: 'openai'
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: { template: '<span />' }
      }
    }
  })

  const opencodeTab = wrapper.findAll('button').find((button) =>
    button.text().includes('keys.useKeyModal.cliTabs.opencode')
  )

  expect(opencodeTab).toBeDefined()
  await opencodeTab!.trigger('click')
  await nextTick()

  const codeBlock = wrapper.find('pre code')
  expect(codeBlock.exists()).toBe(true)
  expect(codeBlock.text()).toContain('"sub2api-openai"')
  expect(codeBlock.text()).toContain('"baseURL": "https://example.com/v1"')
  expect(codeBlock.text()).toContain('"gpt-5.4-Sys"')
  expect(codeBlock.text()).toContain('"name": "GPT-5.4 (Sys)"')
  expect(codeBlock.text()).not.toContain('"provider": {\n    "openai"')
})
```

- [ ] **Step 2: 运行测试，确认当前行为还是占用 `openai` 且没有 `-Sys`**

Run: `pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: FAIL，至少会在以下一项失败：
- 示例里 provider 还是 `openai`
- 不包含 `gpt-5.4-Sys`
- 不包含 `GPT-5.4 (Sys)`

- [ ] **Step 3: 在 `UseKeyModal.vue` 中把 OpenAI 的 OpenCode 模型配置收敛到单一来源**

在 `frontend/src/components/keys/UseKeyModal.vue` 中新增一个局部 helper，不要把 `-Sys` 模型分散手写到多个地方：

```ts
function withSysVariants(
  models: Record<string, { name: string; limit: { context: number; output: number }; options?: Record<string, unknown>; variants?: Record<string, unknown> }>
) {
  const expanded: Record<string, { name: string; limit: { context: number; output: number }; options?: Record<string, unknown>; variants?: Record<string, unknown> }> = {}

  for (const [id, config] of Object.entries(models)) {
    expanded[id] = config
    expanded[`${id}-Sys`] = {
      ...config,
      name: `${config.name} (Sys)`
    }
  }

  return expanded
}
```

然后把 OpenAI OpenCode 模型常量改成类似：

```ts
const openCodeOpenAIBaseModels = {
  'gpt-5-codex': {
    name: 'GPT-5 Codex',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {} }
  },
  'gpt-5.1-codex': {
    name: 'GPT-5.1 Codex',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {} }
  },
  'gpt-5.1-codex-max': {
    name: 'GPT-5.1 Codex Max',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {} }
  },
  'gpt-5.1-codex-mini': {
    name: 'GPT-5.1 Codex Mini',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {} }
  },
  'gpt-5.2': {
    name: 'GPT-5.2',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'gpt-5.4': {
    name: 'GPT-5.4',
    limit: { context: 1050000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'gpt-5.4-mini': {
    name: 'GPT-5.4 Mini',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'gpt-5.4-nano': {
    name: 'GPT-5.4 Nano',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'gpt-5.3-codex-spark': {
    name: 'GPT-5.3 Codex Spark',
    limit: { context: 128000, output: 32000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'gpt-5.3-codex': {
    name: 'GPT-5.3 Codex',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'gpt-5.2-codex': {
    name: 'GPT-5.2 Codex',
    limit: { context: 400000, output: 128000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {}, xhigh: {} }
  },
  'codex-mini-latest': {
    name: 'Codex Mini',
    limit: { context: 200000, output: 100000 },
    options: { store: false },
    variants: { low: {}, medium: {}, high: {} }
  }
} as const

const openaiModels = withSysVariants(openCodeOpenAIBaseModels)
```

- [ ] **Step 4: 把 OpenAI OpenCode 示例的 provider key 从 `openai` 改成 `sub2api-openai`**

把 `generateOpenCodeConfig('openai', apiBase, apiKey)` 改成：

```ts
generateOpenCodeConfig('sub2api-openai', apiBase, apiKey)
```

并确保 `generateOpenCodeConfig()` 在 `platform === 'sub2api-openai'` 时：

```ts
provider[platform] = {
  npm: '@ai-sdk/openai',
  name: 'sub2api OpenAI',
  options: {
    baseURL: baseUrl,
    apiKey
  },
  models: openaiModels
}
```

不要继续复用 `provider.openai` 这个 key。

- [ ] **Step 5: 重新运行测试，确认示例切到独立 provider 且带 `-Sys`**

Run: `pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: PASS，断言里能看到：
- `sub2api-openai`
- `gpt-5.4-Sys`
- `GPT-5.4 (Sys)`

- [ ] **Step 6: 提交这一层改动**

```bash
git add frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "feat(opencode): 独立输出 sub2api OpenAI provider 示例"
```

### Task 2: 同步更新 OpenCode 示例文案，明确这是自定义 provider

**Files:**
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 先补一条失败断言，锁定 hint 不再误导成“默认 openai provider”**

在已有测试文件里追加一个测试：

```ts
it('describes OpenCode config as custom provider based', async () => {
  const wrapper = mount(UseKeyModal, {
    props: {
      show: true,
      apiKey: 'sk-test',
      baseUrl: 'https://example.com/v1',
      platform: 'openai'
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: { template: '<span />' }
      }
    }
  })

  const opencodeTab = wrapper.findAll('button').find((button) =>
    button.text().includes('keys.useKeyModal.cliTabs.opencode')
  )

  await opencodeTab!.trigger('click')
  await nextTick()

  expect(wrapper.text()).toContain('provider_id')
})
```

- [ ] **Step 2: 运行测试，确认当前文案还没有体现“自定义 provider”**

Run: `pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: FAIL，提示文案里还没有 `provider_id` 或自定义 provider 说明。

- [ ] **Step 3: 在中英文文案里改成独立 provider 语义**

把文案调整为明确强调：

中文示例：

```ts
hint: '配置文件路径：~/.config/opencode/opencode.json（或 opencode.jsonc），不存在需手动创建。当前示例会新增一个自定义 provider_id（例如 sub2api-openai），不会占用 OpenCode 官方 openai provider。API Key 可直接写入配置，后续也可自行改成 /connect 或环境变量。'
```

英文示例：

```ts
hint: 'Config path: ~/.config/opencode/opencode.json (or opencode.jsonc). Create it manually if missing. This example adds a custom provider_id (for example sub2api-openai) and does not replace OpenCode\'s built-in openai provider. The API key is written directly for convenience and can later be moved to /connect or env vars.'
```

- [ ] **Step 4: 重新运行测试，确认提示语义与输出结构一致**

Run: `pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: PASS，文案不再暗示占用默认 `openai` provider。

- [ ] **Step 5: 提交文案修正**

```bash
git add frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "docs(opencode): 更新自定义 provider 示例说明"
```

### Task 3: 把独立 provider 配置落到本机 OpenCode，并补齐同一套 `-Sys` 模型

**Files:**
- Modify: `C:\Users\34404\.config\opencode\opencode.jsonc`

- [ ] **Step 1: 先备份当前本机配置**

Run: `Copy-Item C:\Users\34404\.config\opencode\opencode.jsonc C:\Users\34404\.config\opencode\opencode.jsonc.bak-2026-04-04`

Expected: 生成同目录备份文件，便于回滚。

- [ ] **Step 2: 在本机配置中新增 `sub2api-openai` provider，不改现有 provider**

把以下 provider 片段插入 `opencode.jsonc` 的顶层 `provider` 节点（若节点不存在则新建，若已存在则合并）：

```jsonc
"provider": {
  "sub2api-openai": {
    "npm": "@ai-sdk/openai",
    "name": "sub2api OpenAI",
    "options": {
      "baseURL": "https://oai.jwyihao.top/v1",
      "apiKey": "sk-71c02f94211ea843311ab07e497ea141b987e44a042837ef5ced5c629d29e7c9"
    },
    "models": {
      "gpt-5-codex": { "name": "GPT-5 Codex", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5-codex-Sys": { "name": "GPT-5 Codex (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.1-codex": { "name": "GPT-5.1 Codex", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.1-codex-Sys": { "name": "GPT-5.1 Codex (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.1-codex-max": { "name": "GPT-5.1 Codex Max", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.1-codex-max-Sys": { "name": "GPT-5.1 Codex Max (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.1-codex-mini": { "name": "GPT-5.1 Codex Mini", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.1-codex-mini-Sys": { "name": "GPT-5.1 Codex Mini (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "gpt-5.2": { "name": "GPT-5.2", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.2-Sys": { "name": "GPT-5.2 (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.4": { "name": "GPT-5.4", "limit": { "context": 1050000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.4-Sys": { "name": "GPT-5.4 (Sys)", "limit": { "context": 1050000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.4-mini": { "name": "GPT-5.4 Mini", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.4-mini-Sys": { "name": "GPT-5.4 Mini (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.4-nano": { "name": "GPT-5.4 Nano", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.4-nano-Sys": { "name": "GPT-5.4 Nano (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.3-codex-spark": { "name": "GPT-5.3 Codex Spark", "limit": { "context": 128000, "output": 32000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.3-codex-spark-Sys": { "name": "GPT-5.3 Codex Spark (Sys)", "limit": { "context": 128000, "output": 32000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.3-codex": { "name": "GPT-5.3 Codex", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.3-codex-Sys": { "name": "GPT-5.3 Codex (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.2-codex": { "name": "GPT-5.2 Codex", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "gpt-5.2-codex-Sys": { "name": "GPT-5.2 Codex (Sys)", "limit": { "context": 400000, "output": 128000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {}, "xhigh": {} } },
      "codex-mini-latest": { "name": "Codex Mini", "limit": { "context": 200000, "output": 100000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } },
      "codex-mini-latest-Sys": { "name": "Codex Mini (Sys)", "limit": { "context": 200000, "output": 100000 }, "options": { "store": false }, "variants": { "low": {}, "medium": {}, "high": {} } }
    }
  }
}
```

- [ ] **Step 3: 目视确认不影响现有 `agent.general/explore` 与插件配置**

检查以下内容仍然存在且未被覆盖：

- `agent.general.model`
- `agent.explore.model`
- `plugin` 数组
- 现有 `mcp` 配置

Expected: 这些既有节点保持原样，仅新增 `provider.sub2api-openai`。

- [ ] **Step 4: 如果本机有 provider / models 查看命令，就做一次实际读取验证**

优先尝试以下命令（能跑哪个用哪个）：

```bash
opencode models
opencode model list
opencode config get
```

Expected: 至少能确认配置文件被解析，或者在模型列表中能看到 `sub2api-openai/gpt-5.4` / `sub2api-openai/gpt-5.4-Sys`。

- [ ] **Step 5: 记录回滚方式，不提交本机配置到 git**

回滚命令：

```bash
Copy-Item C:\Users\34404\.config\opencode\opencode.jsonc.bak-2026-04-04 C:\Users\34404\.config\opencode\opencode.jsonc -Force
```

注意：这一步是用户本机环境修改，不应加入 `sub2api` 仓库提交。

### Task 4: 跑前端构建回归并整理交付

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: 跑定向前端测试，确认 OpenCode 示例输出稳定**

Run: `pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: PASS，覆盖：
- 独立 provider id
- `-Sys` 模型
- 文案提示

- [ ] **Step 2: 跑前端构建，确认 UI 与复制配置流程未被打坏**

Run: `pnpm build`

Expected: PASS；如有已有 chunk warning 可以接受，但不能出现编译错误。

- [ ] **Step 3: 汇总最终交付物并说明限制**

交付说明必须明确包含：

1. 本机 OpenCode 已新增 `sub2api-openai` provider
2. `sub2api` 前端 OpenCode 示例也改成输出 `sub2api-openai`
3. `-Sys` 模型已加入
4. 当前仍然是手工维护模型清单，未自动继承 models.dev

- [ ] **Step 4: 只提交仓库内改动，不提交本机 `opencode.jsonc`**

```bash
git add frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(opencode): 独立输出 sub2api OpenAI 配置示例"
```

## 自查清单

- Spec 覆盖检查：
  - 独立 provider：Task 1 + Task 3
  - `sub2api` 示例不再占用 `openai`：Task 1 + Task 2
  - `-Sys` 模型：Task 1 + Task 3
  - 本机配置落地：Task 3
- 占位符检查：本计划没有 `TODO/TBD/implement later` 等空泛步骤。
- 类型一致性检查：provider id 全程统一为 `sub2api-openai`，`-Sys` 通过同一模型扩展逻辑生成，不在不同任务里使用不同命名。
