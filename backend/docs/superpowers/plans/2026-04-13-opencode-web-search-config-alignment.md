# OpenCode 推荐配置 `fast` 派生与 `builtin_tools` 对齐 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让本地 `sub2api-openai` 推荐配置跟上 upstream 当前 `experimental.modes -> *-fast` 模型派生语义，并在所有最终推荐模型上统一输出本地 `builtin_tools: { web_search: true }` 默认参数。

**Architecture:** 后端 metadata mirror 负责把 upstream `models.dev` 中的基础模型和 `experimental.modes` 派生模型扁平化成前端可消费的数据契约；前端推荐配置生成层负责在此基础上继续派生本地 `-Sys` 模型，并统一挂载 `builtin_tools`。不把 `web_search` 伪装成 upstream 原生 `tools` 配置字段，继续保留 `sub2api-openai` 与本地 `key = 最终请求 id` 语义。

**Tech Stack:** Go, Vue 3, TypeScript, Vitest

---

### Task 1: 扩展 backend metadata mirror，物化 upstream `experimental.modes`

**Files:**
- Modify: `backend/internal/service/opencode_openai_metadata.go`
- Test: `backend/internal/service/opencode_openai_metadata_test.go`
- Test: `backend/internal/server/api_contract_test.go`

- [ ] **Step 1: 写红灯测试，锁定 `experimental.modes.fast` 物化行为**

在 `opencode_openai_metadata_test.go` 增加一个合成 payload 用例，输入形如：

```go
payload := map[string]any{
  "openai": map[string]any{
    "models": map[string]any{
      "gpt-5.4": map[string]any{
        "id": "gpt-5.4",
        "name": "GPT-5.4",
        "reasoning": true,
        "attachment": true,
        "tool_call": true,
        "structured_output": true,
        "temperature": false,
        "modalities": map[string]any{
          "input":  []any{"text", "image", "pdf"},
          "output": []any{"text"},
        },
        "cost": map[string]any{"input": 2.5, "output": 15.0, "cache_read": 0.25},
        "limit": map[string]any{"context": 1050000, "input": 922000, "output": 128000},
        "experimental": map[string]any{
          "modes": map[string]any{
            "fast": map[string]any{
              "cost": map[string]any{"input": 5.0, "output": 30.0, "cache_read": 0.5},
              "provider": map[string]any{
                "body": map[string]any{"service_tier": "priority"},
                "headers": map[string]any{"x-test-header": "fast-mode"},
              },
            },
          },
        },
      },
    },
  },
}
```

断言：
- 结果里存在 `gpt-5.4`
- 结果里存在 `gpt-5.4-fast`
- `gpt-5.4-fast` 的 `Name == "GPT-5.4 Fast"`
- `gpt-5.4-fast` 的 `Options["serviceTier"] == "priority"`
- `gpt-5.4-fast` 的 `Headers["x-test-header"] == "fast-mode"`
- backend 过滤层仍保留，并明确锁住：
  - `gpt-5.4-fast` 在基础模型允许时会保留
  - `gpt-4o-fast` 这类不在本地允许集合中的派生模型仍会被过滤

- [ ] **Step 2: 运行测试确认当前失败**

Run: `go test ./internal/service -run "OpenCodeOpenAI.*Fast|ExtractOpenCodeOpenAIModels" -count=1`

Expected: 失败，原因是当前还不会物化 `gpt-5.4-fast`，也没有 `Options/Headers` 字段。

- [ ] **Step 3: 扩展后端数据结构与物化逻辑**

在 `opencode_openai_metadata.go`：

1. 扩展 `OpenCodeOpenAIModel`：

```go
type OpenCodeOpenAIModel struct {
    ID               string                        `json:"id"`
    Name             string                        `json:"name"`
    Family           string                        `json:"family,omitempty"`
    Attachment       bool                          `json:"attachment"`
    Reasoning        bool                          `json:"reasoning"`
    ToolCall         bool                          `json:"tool_call"`
    StructuredOutput bool                          `json:"structured_output"`
    Temperature      bool                          `json:"temperature"`
    Knowledge        string                        `json:"knowledge,omitempty"`
    Interleaved      any                           `json:"interleaved,omitempty"`
    Modalities       OpenCodeOpenAIModelModalities `json:"modalities,omitempty"`
    Cost             OpenCodeOpenAIModelCost       `json:"cost,omitempty"`
    Limit            OpenCodeOpenAIModelLimit      `json:"limit,omitempty"`
    ReleaseDate      string                        `json:"release_date,omitempty"`
    Options          map[string]any                `json:"options,omitempty"`
    Headers          map[string]string             `json:"headers,omitempty"`
}
```

2. 在 `extractOpenCodeOpenAIModels(...)` 中，不只读取基础模型，还要读取 `experimental.modes`，物化成额外条目。

3. 加一个小 helper，把 mode `provider.body` 的 snake_case key 转为 camelCase：

```go
func toCamelCaseProviderBody(input map[string]any) map[string]any
```

4. 生成 fast 模型时：
- key: `gpt-5.4-fast`
- `ID`: 继续保持本地最终请求 id 语义，设置成 `gpt-5.4-fast`
- `Name`: `GPT-5.4 Fast`
- 继承基础模型可复用字段
- 覆盖 `Cost`
- 写入 `Options`
- 写入 `Headers`

5. 继续保留 `filterOpenCodeOpenAIModelsForCodexOAuth()`，但要让 `*-fast` 模型在其基础模型允许时也能通过筛选。

6. 在 backend mirror 的 fast 物化逻辑旁补上 upstream 来源注释，位置必须落在 `backend/internal/service/opencode_openai_metadata.go`，而不是只写在前端：
- upstream repo
- 文件：`packages/opencode/src/provider/provider.ts`
- 函数：`fromModelsDevProvider()`
- commit：`7a6ce05`
- permalink / 行号范围
- 一句本地改写说明：backend 只负责 mirror/扁平化，`-Sys` 和 `builtin_tools` 是后续本地扩展层处理

- [ ] **Step 4: 重跑 backend 定向测试**

Run: `go test ./internal/service -run "OpenCodeOpenAI.*Fast|ExtractOpenCodeOpenAIModels" -count=1`

Expected: PASS

- [ ] **Step 5: 增加 backend HTTP 合约测试，锁住响应体不泄漏 raw 字段**

在 `backend/internal/server/api_contract_test.go` 增加一条最小合约测试，直接请求：

```text
GET /api/v1/keys/opencode/openai-models
```

断言至少包括：
- 返回体中存在 `gpt-5.4-fast`
- `gpt-5.4-fast.options.serviceTier == "priority"`
- `gpt-5.4-fast.headers["x-test-header"]`（或合成 fixture 中的测试 header）存在
- 返回体中不存在 raw `experimental`
- 返回体中不存在 raw mode `provider.body` / `provider.headers`

Run: `go test ./internal/server -run "OpenCodeOpenAIModels|api_contract" -count=1`

Expected: PASS

- [ ] **Step 6: 提交 Task 1**

```bash
git add backend/internal/service/opencode_openai_metadata.go backend/internal/service/opencode_openai_metadata_test.go backend/internal/server/api_contract_test.go
git commit -m "feat(opencode): 物化 fast 派生模型镜像"
```

### Task 2: 更新前端 API 契约，承载 `options` / `headers`

**Files:**
- Modify: `frontend/src/api/keys.ts`

- [ ] **Step 1: 先写类型层红灯（typecheck）**

目标：让当前代码在开始使用 `model.options` / `model.headers` 时先报类型缺失，而不是靠 `any` 混过去。

准备修改点：

```ts
export interface OpenCodeOpenAIModel {
  id: string
  name: string
  family?: string
  attachment: boolean
  reasoning: boolean
  tool_call: boolean
  structured_output: boolean
  temperature: boolean
  knowledge?: string
  interleaved?: unknown
  modalities?: { input?: string[]; output?: string[] }
  cost?: {
    input?: number
    output?: number
    cache_read?: number
    cache_write?: number
  }
  limit?: { context?: number; input?: number; output?: number }
  release_date?: string
  options?: Record<string, unknown>
  headers?: Record<string, string>
}
```

- [ ] **Step 2: 更新类型并跑 typecheck**

Run: `pnpm typecheck`

Expected: PASS（或只暴露后续 Task 3 真实使用点）

- [ ] **Step 3: 提交 Task 2**

```bash
git add frontend/src/api/keys.ts
git commit -m "feat(opencode): 扩展推荐配置模型契约字段"
```

### Task 3: 重写 `sub2api-openai` 推荐配置生成，替换旧 fast variants

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 写红灯测试，锁定新结构**

在 `UseKeyModal.spec.ts` 当前 `renders sub2api-openai provider config with Sys models in OpenCode example` 基础上改成新的断言。

先把 mock API 返回改成 backend 新契约形态：
- mock 中直接提供 `gpt-5.4-fast`
- `gpt-5.4-fast.options.serviceTier = 'priority'`
- `gpt-5.4-fast.headers['x-test-header'] = 'fast-mode'`
- 同时故意在 mock 中混入 raw `experimental` 和 raw mode `provider.body/provider.headers` 字段，要求最终导出 JSON 中这些字段都被清洗掉
- 不再依赖前端自己从 `gpt-5.4` 推导 fast

必须存在：
- `provider['sub2api-openai'].models['gpt-5.4']`
- `provider['sub2api-openai'].models['gpt-5.4-fast']`
- `provider['sub2api-openai'].models['gpt-5.4-Sys']`
- `provider['sub2api-openai'].models['gpt-5.4-fast-Sys']`

必须满足：
- `gpt-5.4-fast.options.serviceTier === 'priority'`
- `gpt-5.4-fast-Sys.options.serviceTier === 'priority'`
- `gpt-5.4-fast.options.builtin_tools` 深等于 `{ web_search: true }`
- `gpt-5.4-fast-Sys.options.builtin_tools` 深等于 `{ web_search: true }`
- `gpt-5.4.options.builtin_tools` 深等于 `{ web_search: true }`
- `gpt-5.4-Sys.options.builtin_tools` 深等于 `{ web_search: true }`
- 至少再断言一个非 fast 模型（例如 `gpt-5.4-mini`）也输出 `builtin_tools: { web_search: true }`
- `gpt-5.4.id` / `gpt-5.4-fast.id` / `gpt-5.4-Sys.id` / `gpt-5.4-fast-Sys.id` 都保持 `undefined`
- `gpt-5.4-fast.headers['x-test-header'] === 'fast-mode'`
- `gpt-5.4-fast-Sys.headers['x-test-header'] === 'fast-mode'`

必须不存在：
- `gpt-5.4.variants.low-fast`
- `gpt-5.4.variants.medium-fast`
- `gpt-5.4.variants.high-fast`
- `gpt-5.4.variants.xhigh-fast`
- `gpt-5.4-Sys.variants.*-fast`
- 其它非 fast 模型也不能继续残留旧的 `*-fast` variant

还要断言：
- 最终 JSON 中不泄漏 raw `experimental`
- 最终 JSON 中不泄漏 raw mode `provider.body` / `provider.headers`
- `provider.openai` 仍不存在
- 最终 JSON 中不存在 `model.tools`

- [ ] **Step 2: 运行测试确认当前失败**

Run: `pnpm vitest run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: FAIL，当前仍是旧的手工 `low-fast` / `high-fast` 结构。

- [ ] **Step 3: 重写生成逻辑**

在 `UseKeyModal.vue`：

1. 删除旧的 `buildReasoningVariants(..., id === 'gpt-5.4')` fast 生成策略。

2. 保留普通 reasoning variants 生成，但把 fast 从“variant”提升为独立模型条目；实现时必须消费 backend 已物化出的 `gpt-5.4-fast`，而不是继续在前端本地生成它。

3. 新增 helper，基于后端 mirror 数据构造基础模型集合：

```ts
function buildOpenCodeOpenAIBaseModels(openaiSource: Record<string, OpenCodeOpenAIModel>) {
  return Object.fromEntries(
    Object.entries(openaiSource).map(([id, model]) => {
      const normalized = normalizeOpenCodeModelConfig(model)
      return [
        id,
        {
          ...normalized,
          options: {
            ...(normalized.options ?? {}),
            builtin_tools: { web_search: true },
            store: false,
          },
          headers: normalized.headers,
          variants: buildReasoningVariants(reasoningLevels(id, model), false),
        },
      ]
    }),
  )
}
```

4. 让 `withSysVariants(...)` 继续作用在“基础模型 + fast 模型”的最终集合上。

5. 继续保留：
- `sub2api-openai` provider id
- `npm: '@ai-sdk/openai'`
- 现有 `agent.build.options.store=false`
- 现有 `agent.plan.options.store=false`

6. 在这段镜像/派生逻辑上方加入来源注释，必须包含：
- upstream repo
- `packages/opencode/src/provider/provider.ts`
- `fromModelsDevProvider()`
- commit `7a6ce05`
- permalink / 行号说明

并在 `builtin_tools` 默认注入处注明：
- 这是本地扩展
- upstream runtime 不消费 model-level `tools`
- 我们通过本地 `builtin_tools` 私有参数把 `web_search` 默认开启语义透传给 API 转发服务

- [ ] **Step 4: 重跑前端测试和 typecheck**

Run:
- `pnpm vitest run "src/components/keys/__tests__/UseKeyModal.spec.ts"`
- `pnpm typecheck`

Expected: PASS

- [ ] **Step 5: 提交 Task 3**

```bash
git add frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "feat(opencode): 对齐 fast 派生并默认开启 web_search"
```

### Task 4: 全量验证与边界确认

**Files:**
- Verify current changes only
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 跑 backend 验证**

Run:
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go test ./internal/service -run "OpenCodeOpenAI.*Fast|ExtractOpenCodeOpenAIModels|BuiltinTools|ForwardAsChatCompletions|AugmentsBuiltinTools" -count=1`
- `go build ./cmd/server`

Expected: PASS

- [ ] **Step 2: 跑前端验证**

Run:
- `pnpm typecheck`
- `pnpm vitest run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: PASS

- [ ] **Step 3: 边界人工确认**

确认以下几项：
- `provider['sub2api-openai']` 仍存在，`provider.openai` 未被误改
- `gpt-5.4-fast` / `gpt-5.4-fast-Sys` 存在
- 旧 `*-fast` variants 已移除
- `builtin_tools` 统一以 `{ web_search: true }` 输出
- raw `experimental` / raw mode provider 结构未泄漏到最终 JSON

- [ ] **Step 4: 补一条 backend 消费链确认**

目标：确认前端统一输出的对象形态 `builtin_tools: { web_search: true }`，能通过现有 backend 消费链真正落成 upstream `tools:[{"type":"web_search"}]`，并剥离私有字段。

最小要求：
- 分别补两条 service 级测试：
  1. `/v1/responses` 路径输入对象形态 `builtin_tools: { web_search: true }`
  2. `/v1/chat/completions` 路径输入对象形态 `builtin_tools: { web_search: true }`
- 两条测试都必须断言最终 upstream request body：
  - 存在 `tools:[{"type":"web_search"}]`
  - 不再存在 `builtin_tools`
  - `tool_choice` 保持原样

- [ ] **Step 5: 提交 Task 4 / 收尾提交**

```bash
git add backend/internal/service/opencode_openai_metadata.go backend/internal/service/opencode_openai_metadata_test.go frontend/src/api/keys.ts frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "feat(opencode): 对齐 fast 派生并默认注入 builtin_tools"
```
