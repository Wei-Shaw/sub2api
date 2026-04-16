# OpenCode 推荐配置 `fast` 对齐与 `metadata.builtin_tools` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `sub2api-openai` 推荐配置跟上 upstream `experimental.modes -> *-fast + model.options/headers` 的生成语义，同时把默认 `web_search` 统一迁移到 `options.metadata.builtin_tools`，并让网关同时兼容顶层/metadata 两层 carrier。

**Architecture:** backend mirror 层只负责物化 upstream fast 模型和 `options/headers` 中间契约；前端最终推荐配置层负责后置派生 `-Sys` 并为 `*-fast` 覆写本地兼容 `id`；gateway 层独立扩展为双读 `body.builtin_tools` 与 `body.metadata.builtin_tools`，消费后双剥离，再把 phase-1 `web_search` 转成真实 upstream `tools`。执行顺序必须先让 gateway 能读新 carrier，再切前端输出，否则中间提交会把默认 `web_search` 弄失效。

**Tech Stack:** Go, Gin, Vue 3, TypeScript, Vitest

---

### Task 1: 扩展 backend mirror，物化 `experimental.modes` 并锁住 backend contract

**Files:**
- Modify: `backend/internal/service/opencode_openai_metadata.go`
- Test: `backend/internal/service/opencode_openai_metadata_test.go`
- Test: `backend/internal/server/api_contract_test.go`

- [ ] **Step 1: 写红灯测试，锁 `gpt-5.4-fast` mirror 语义**

在 `backend/internal/service/opencode_openai_metadata_test.go` 增加一个合成 payload，用例必须至少构造：

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
				"modalities": map[string]any{"input": []any{"text", "image", "pdf"}, "output": []any{"text"}},
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
			"gpt-4o": map[string]any{
				"id": "gpt-4o",
				"name": "GPT-4o",
				"experimental": map[string]any{
					"modes": map[string]any{
						"fast": map[string]any{
							"provider": map[string]any{"body": map[string]any{"service_tier": "priority"}},
						},
					},
				},
			},
		},
	},
}
```

断言：
- `extractOpenCodeOpenAIModels(payload)` 结果存在 `gpt-5.4-fast`
- `gpt-5.4-fast.ID == "gpt-5.4-fast"`（mirror 层不能提前覆写成 `gpt-5.4`）
- `gpt-5.4-fast.Name == "GPT-5.4 Fast"`
- `gpt-5.4-fast.Options["serviceTier"] == "priority"`
- `gpt-5.4-fast.Headers["x-test-header"] == "fast-mode"`
- backend 结果里**不得**出现 `gpt-5.4-fast-Sys`
- `gpt-4o-fast` 仍被过滤掉

- [ ] **Step 2: 跑红灯确认当前失败**

Run:
`go test ./internal/service -run "OpenCodeOpenAI.*Fast|ExtractOpenCodeOpenAIModels" -count=1`

Expected: FAIL，当前 backend 还不会物化 `gpt-5.4-fast`，也不会稳定返回 `Options/Headers`。

- [ ] **Step 3: 实现 backend mirror 物化逻辑**

在 `backend/internal/service/opencode_openai_metadata.go`：

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

2. 新增 helper：

```go
func toCamelCaseProviderBody(input map[string]any) map[string]any
func cloneAnyMap(input map[string]any) map[string]any
func cloneStringMap(input map[string]string) map[string]string
```

3. 在 `extractOpenCodeOpenAIModels(...)` 中：
- 保留基础模型提取
- 读取 `experimental.modes`
- 物化独立 fast key，例如 `gpt-5.4-fast`
- 继承基础模型公共字段
- 覆盖 mode cost
- 写入 `Options` / `Headers`
- **不要**在这里做 `-Sys` 派生
- **不要**在这里做 fast-id 覆写

4. 继续保留 `filterOpenCodeOpenAIModelsForCodexOAuth()`，但允许 `*-fast` 在其基础模型允许时通过。

5. 在 fast 物化逻辑旁补来源注释，必须写在 backend mirror 文件里，包含：
- repo: `anomalyco/opencode`
- file: `packages/opencode/src/provider/provider.ts`
- function: `fromModelsDevProvider()`
- commit: `7a6ce05`
- permalink + 行号范围
- 本地改写说明：backend 只负责 mirror/扁平化，`-Sys`、fast-id 覆写、`provider["sub2api-openai"]`、`metadata.builtin_tools` 都是后续本地扩展层处理

- [ ] **Step 4: 重跑 backend service 测试**

Run:
`go test ./internal/service -run "OpenCodeOpenAI.*Fast|ExtractOpenCodeOpenAIModels" -count=1`

Expected: PASS

- [ ] **Step 5: 增加 backend HTTP contract 测试**

在 `backend/internal/server/api_contract_test.go` 增加：

```text
GET /api/v1/keys/opencode/openai-models
```

断言：
- `models["gpt-5.4-fast"]` 存在
- `models["gpt-5.4-fast"].id == "gpt-5.4-fast"`
- `models["gpt-5.4-fast"].options.serviceTier == "priority"`
- `models["gpt-5.4-fast"].headers` 存在
- backend 响应中不存在 raw `experimental`
- backend 响应中不存在 raw mode `provider.body` / `provider.headers`
- backend 响应中不存在 `gpt-5.4-fast-Sys`

Run:
`go test ./internal/server -run "OpenCodeOpenAIModels|api_contract" -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/opencode_openai_metadata.go backend/internal/service/opencode_openai_metadata_test.go backend/internal/server/api_contract_test.go
git commit -m "feat(opencode): 物化 fast 派生模型镜像"
```

### Task 2: 确认并收紧前端 API 契约

**Files:**
- Modify: `frontend/src/api/keys.ts`

- [ ] **Step 1: 确认/补齐 `options` / `headers` 类型**

确保 `frontend/src/api/keys.ts` 中：

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
  cost?: { input?: number; output?: number; cache_read?: number; cache_write?: number }
  limit?: { context?: number; input?: number; output?: number }
  release_date?: string
  options?: Record<string, unknown>
  headers?: Record<string, string>
}
```

- [ ] **Step 2: 跑 typecheck**

Run:
`pnpm typecheck`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/api/keys.ts
git commit -m "feat(opencode): 收紧推荐配置模型契约字段"
```

### Task 3: 先扩 gateway 双读/双剥离，再切前端输出到 metadata carrier

**Files:**
- Modify: `backend/internal/service/openai_builtin_tools.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `backend/internal/pkg/apicompat/types.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 写红灯测试，锁住双读与双剥离**

在 `backend/internal/service/openai_gateway_service_test.go` 新增 service 级回归，分别覆盖：

1. `/v1/responses`：
- 输入只有 `metadata.builtin_tools = { web_search: true }`
- 最终 upstream body 含 `tools:[{"type":"web_search"}]`
- upstream body 不含顶层 `builtin_tools`
- upstream body 的 `metadata` 中不再含 `builtin_tools`
- `metadata` 中其它兄弟键必须保留
- `tool_choice` 保持原样

2. `/v1/chat/completions` compat：
- `metadata.builtin_tools = { web_search: true }` 能触发 augmentation
- 最终 upstream body 含 `tools:[{"type":"web_search"}]`
- final upstream body 不含 `metadata.builtin_tools`
- `tool_choice` 保持原样

3. 旧顶层兼容：
- 顶层 `builtin_tools` 仍可用
- 如果顶层和 `metadata.builtin_tools` 同时存在，以顶层优先
- 但最终 upstream body 中，顶层 `builtin_tools` 与 `metadata.builtin_tools` 都必须被剥离干净

4. passthrough / compact：
- `metadata.builtin_tools` 也会被 strip
- 但不会 augmentation
- `metadata` 其它键仍保留

- [ ] **Step 2: 跑红灯确认当前失败**

Run:
`go test ./internal/service -run "BuiltinTools|MetadataBuiltinTools|AugmentsBuiltinTools|PassthroughStripsBuiltinToolsWithoutAugmenting|CompactPathDoesNotAugmentBuiltinTools|ToolChoice" -count=1`

Expected: FAIL，当前 gateway 仍只读顶层 `builtin_tools`。

- [ ] **Step 3: 实现 gateway 双读与双剥离**

1. `backend/internal/service/openai_builtin_tools.go`
- 保持 `normalizeOpenAIBuiltinTools(raw any)` 的 phase-1 `web_search only` 结果不变。

2. `backend/internal/pkg/apicompat/types.go`
- 让 chat compat 请求结构能稳定承接 metadata carrier（如果已有 `Metadata` 则复用；若没有则补最小字段）。

3. `backend/internal/service/openai_gateway_service.go`
- 新增 helper：

```go
func extractOpenAIBuiltinToolsCarrier(reqBody map[string]any) any {
	if reqBody == nil {
		return nil
	}
	if raw, ok := reqBody["builtin_tools"]; ok {
		return raw
	}
	metadata, _ := reqBody["metadata"].(map[string]any)
	if metadata == nil {
		return nil
	}
	return metadata["builtin_tools"]
}
```

- augmentation 顺序改成：
  1. 先读顶层 `builtin_tools`
  2. 顶层没有再读 `metadata.builtin_tools`
  3. normalize
  4. augment `tools`

- strip helper 必须两层清理：
  - 删除顶层 `builtin_tools`
  - 删除 `metadata.builtin_tools`
  - 不得整段删除 `metadata`

4. `backend/internal/service/openai_gateway_chat_completions.go`
- compat 路径在 augmentation 前必须能看到 metadata carrier
- compat 最终 upstream body 也必须完成 `metadata.builtin_tools` 剥离

- [ ] **Step 4: 重跑 backend service / full validation**

Run:
- `go test ./internal/service -run "BuiltinTools|MetadataBuiltinTools|AugmentsBuiltinTools|PassthroughStripsBuiltinToolsWithoutAugmenting|CompactPathDoesNotAugmentBuiltinTools|ToolChoice" -count=1`
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`
- `git diff --check`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openai_builtin_tools.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_chat_completions.go backend/internal/pkg/apicompat/types.go backend/internal/service/openai_gateway_service_test.go
git commit -m "feat(openai): 兼容 metadata builtin_tools"
```

### Task 4: 切前端推荐配置到 fast-id 覆写 + metadata carrier

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Test: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 写红灯测试，锁住最终推荐配置契约**

mock backend mirror 返回必须包含：
- `gpt-5.4-fast`
- `gpt-5.4-fast.options.serviceTier = 'priority'`
- `gpt-5.4-fast.headers['x-test-header'] = 'fast-mode'`
- 故意混入 raw `experimental` 与 raw mode `provider.body/provider.headers`
- 并且这些 raw 值必须与已物化 fast 条目**故意冲突**，例如：
  - backend mirror 的 `gpt-5.4-fast.options.serviceTier = 'priority'`
  - 但 base model 的 raw `experimental.modes.fast.provider.body.service_tier = 'wrong-from-raw'`
  - backend mirror 的 `gpt-5.4-fast.headers['x-test-header'] = 'fast-mode'`
  - 但 raw mode `provider.headers['x-test-header'] = 'wrong-raw-header'`
  这样测试才能真正证明前端是在消费 backend 已物化的 fast 结果，而不是继续偷偷从 raw `experimental/provider` 重建 fast

最终配置必须存在：
- `provider['sub2api-openai'].models['gpt-5.4']`
- `provider['sub2api-openai'].models['gpt-5.4-fast']`
- `provider['sub2api-openai'].models['gpt-5.4-Sys']`
- `provider['sub2api-openai'].models['gpt-5.4-fast-Sys']`

必须断言：
- `gpt-5.4.id === 'gpt-5.4'`
- `gpt-5.4-Sys.id === 'gpt-5.4-Sys'`
- `gpt-5.4-fast.id === 'gpt-5.4'`
- `gpt-5.4-fast-Sys.id === 'gpt-5.4-Sys'`
- `gpt-5.4-fast.options.serviceTier === 'priority'`
- `gpt-5.4-fast-Sys.options.serviceTier === 'priority'`
- `gpt-5.4-fast.headers['x-test-header'] === 'fast-mode'`
- `gpt-5.4-fast-Sys.headers['x-test-header'] === 'fast-mode'`
- 所有最终模型（基础/fast/Sys/fast-Sys 以及至少一个非 fast 模型）都存在：
  - `options.metadata.builtin_tools = { web_search: true }`
- 最终配置里：
  - 不存在 `options.builtin_tools`
  - 不存在 `model.tools`
  - 不存在 raw `experimental`
  - 不存在 raw mode `provider.body/provider.headers`
  - 不存在 `provider.openai`
  - `gpt-5.4` / `gpt-5.4-Sys` 下不再残留旧 `low-fast` / `medium-fast` / `high-fast` / `xhigh-fast` variants

- [ ] **Step 2: 跑红灯确认当前失败**

Run:
`pnpm vitest run "src/components/keys/__tests__/UseKeyModal.spec.ts"`

Expected: FAIL，当前仍是旧 fast 变体与顶层 `options.builtin_tools` 形态。

- [ ] **Step 3: 重写 `UseKeyModal` 生成逻辑**

在 `frontend/src/components/keys/UseKeyModal.vue`：

1. 保留 `normalizeOpenCodeModelConfig(...)` 的 raw 清洗职责，但仍剥离 raw `experimental`、raw `provider`、raw `tools`。

2. 构造基础模型集合时：
- 直接消费 backend 提供的 `gpt-5.4-fast`
- 不再手工从 `gpt-5.4` 生成 old `*-fast` 变体
- 对每个模型统一写：

```ts
const options = {
  ...(normalized.options ?? {}),
  metadata: {
    ...(((normalized.options as Record<string, any> | undefined)?.metadata as Record<string, any> | undefined) ?? {}),
    builtin_tools: { web_search: true },
  },
  store: false,
}
```

- 对 `*-fast` key 做本地 `id` 覆写：

```ts
const finalId = id.endsWith('-fast') ? id.replace(/-fast$/, '') : id
```

3. `withSysVariants(...)` 对最终模型集合统一后置派生，并处理 `id`：

```ts
expanded[`${id}-Sys`] = {
  ...config,
  id: `${config.id}-Sys`,
  name: `${config.name} (Sys)`
}
```

4. 保留：
- `provider['sub2api-openai']`
- `npm: '@ai-sdk/openai'`
- `agent.build.options.store = false`
- `agent.plan.options.store = false`

5. 在镜像/派生逻辑旁补来源注释：
- upstream repo
- `packages/opencode/src/provider/provider.ts`
- `fromModelsDevProvider()`
- commit `7a6ce05`
- permalink / 行号
- 说明：`-Sys`、fast-id 覆写、`metadata.builtin_tools` 属于本地扩展

- [ ] **Step 4: 重跑前端测试和 typecheck**

Run:
- `pnpm vitest run "src/components/keys/__tests__/UseKeyModal.spec.ts"`
- `pnpm typecheck`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "feat(opencode): 对齐 fast 派生并迁移 metadata builtin_tools"
```

### Task 5: 全量验证与边界确认

**Files:**
- No production files; verification only

- [ ] **Step 1: 运行最终验证**

Run:
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`
- `pnpm typecheck`
- `pnpm vitest run "src/components/keys/__tests__/UseKeyModal.spec.ts"`
- `git diff --check`

Expected: PASS

- [ ] **Step 2: 最终边界人工确认**

确认以下事实成立：
- backend mirror 只产出 `gpt-5.4-fast`，不产出 `-Sys`
- backend mirror 中 `gpt-5.4-fast.id` 仍是 `gpt-5.4-fast`
- 最终推荐配置中 `gpt-5.4-fast.id === 'gpt-5.4'`
- 最终推荐配置中 `gpt-5.4-fast-Sys.id === 'gpt-5.4-Sys'`
- 最终推荐配置中只有 `options.metadata.builtin_tools`，没有 `options.builtin_tools`
- gateway 对顶层 `builtin_tools` 和 `metadata.builtin_tools` 双读有效
- passthrough / compact 只 strip，不 augment
- `provider['sub2api-openai']` 仍保留，`provider.openai` 不被改写
