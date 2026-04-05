# OpenCode OpenAI Metadata Mirror Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `sub2api` 前端生成的 OpenCode 推荐配置改为通过后端拉取/缓存 `models.dev` 的 built-in `openai` 元数据生成，再统一叠加 `-Sys` 与 `*-fast` 变体。

**Architecture:** 在后端新增一条仅供推荐配置使用的 OpenAI 元数据链：后端从 `models.dev` 拉取 built-in `openai` provider，做与 OpenCode `fromModelsDevModel()` 同构的字段转换，并通过专用接口下发给前端。前端 `UseKeyModal.vue` 不再维护手写基础模型表，而是消费这条接口返回的基础模型，再统一追加 `-Sys` 与 Fast 组合变体，生成 `sub2api-openai` 推荐配置。

**Tech Stack:** Go backend service/handler/repository, Vue 3 frontend, TypeScript, Vitest, OpenCode provider metadata semantics.

---

## 文件边界

- Create: `backend/internal/service/opencode_openai_metadata.go`
  - 后端 models.dev 拉取、缓存、转换逻辑。
- Create: `backend/internal/service/opencode_openai_metadata_test.go`
  - 锁定 `models.dev openai -> 我们的推荐配置模型结构` 的关键映射。
- Modify: `backend/internal/handler/dto/types.go`
  - 如需要，为推荐配置接口增加前端消费 DTO。
- Modify: `backend/internal/handler/admin/...` or `backend/internal/handler/...`
  - 新增推荐配置专用元数据接口 handler（按现有路由组织选择位置）。
- Modify: `backend/internal/server/routes/...`
  - 注册新的推荐配置元数据接口。
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
  - 改为请求后端元数据接口，再叠加 `-Sys` / `*-fast` 变体。
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
  - 锁定 `attachment/image/pdf` 等能力字段和现有自定义变体。

## Task 1: 先写失败测试锁定后端元数据转换契约

**Files:**
- Create: `backend/internal/service/opencode_openai_metadata_test.go`

- [ ] **Step 1: 写失败测试，锁定 `gpt-5.4` 的关键映射字段**

测试最小样例应表达：

```go
func TestConvertOpenCodeOpenAIModel_GPT54Capabilities(t *testing.T) {
    src := map[string]any{
        "id": "gpt-5.4",
        "name": "GPT-5.4",
        "attachment": true,
        "reasoning": true,
        "tool_call": true,
        "modalities": map[string]any{
            "input": []any{"text", "image", "pdf"},
            "output": []any{"text"},
        },
        "limit": map[string]any{
            "context": 1050000,
            "input": 922000,
            "output": 128000,
        },
        "cost": map[string]any{
            "input": 2.5,
            "output": 15.0,
            "cache_read": 0.25,
            "context_over_200k": map[string]any{
                "input": 5.0,
                "output": 22.5,
                "cache_read": 0.5,
            },
        },
        "release_date": "2026-03-05",
    }

    model, err := convertOpenCodeOpenAIModel(src)

    require.NoError(t, err)
    require.Equal(t, "gpt-5.4", model.ID)
    require.True(t, model.Attachment)
    require.Contains(t, model.Modalities.Input, "image")
    require.Contains(t, model.Modalities.Input, "pdf")
    require.Equal(t, 5.0, model.Cost.ContextOver200K.Input)
}
```

- [ ] **Step 2: 再写失败测试，锁定 built-in `openai` provider 过滤逻辑**

```go
func TestExtractOpenCodeOpenAIModels_UsesBuiltInOpenAIProvider(t *testing.T) {
    payload := map[string]any{
        "openai": map[string]any{
            "models": map[string]any{
                "gpt-5.4": map[string]any{"id": "gpt-5.4", "name": "GPT-5.4"},
            },
        },
        "anthropic": map[string]any{
            "models": map[string]any{
                "claude-sonnet-4": map[string]any{"id": "claude-sonnet-4", "name": "Claude Sonnet 4"},
            },
        },
    }

    models, err := extractOpenCodeOpenAIModels(payload)

    require.NoError(t, err)
    require.Contains(t, models, "gpt-5.4")
    require.NotContains(t, models, "claude-sonnet-4")
}
```

- [ ] **Step 3: 跑测试确认先失败（RED）**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run '^Test(ConvertOpenCodeOpenAIModel_GPT54Capabilities|ExtractOpenCodeOpenAIModels_UsesBuiltInOpenAIProvider)$' -count=1
```

Expected: FAIL，因为相关转换函数和结构体尚不存在。

## Task 2: 实现后端 models.dev 拉取/缓存/转换接口

**Files:**
- Create: `backend/internal/service/opencode_openai_metadata.go`
- Modify: `backend/internal/handler/...`
- Modify: `backend/internal/server/routes/...`

- [ ] **Step 1: 在 service 层实现 `models.dev -> openai models` 拉取与 TTL cache**

要求实现以下最小单元：

```go
type OpenCodeOpenAIModel struct {
    ID          string
    Name        string
    Attachment  bool
    Reasoning   bool
    ToolCall    bool
    Interleaved any
    Modalities  struct {
        Input  []string
        Output []string
    }
    Limit struct {
        Context int
        Input   int
        Output  int
    }
    Cost struct {
        Input          float64
        Output         float64
        CacheRead      float64
        ContextOver200K struct {
            Input     float64
            Output    float64
            CacheRead float64
        }
    }
    ReleaseDate string
}

func convertOpenCodeOpenAIModel(raw map[string]any) (OpenCodeOpenAIModel, error)
func extractOpenCodeOpenAIModels(payload map[string]any) (map[string]OpenCodeOpenAIModel, error)
func (s *OpenCodeMetadataService) GetOpenAIModels(ctx context.Context) (map[string]OpenCodeOpenAIModel, error)
```

行为要求：
- 后端请求 `https://models.dev/api.json`
- 只提取 built-in `openai` provider
- 增加短 TTL 缓存，拉取失败时优先返回最近成功结果

- [ ] **Step 2: 增加推荐配置专用接口**

推荐接口契约示意：

```go
// GET /api/v1/keys/opencode/openai-models
{
  "success": true,
  "data": {
    "models": {
      "gpt-5.4": {
        "attachment": true,
        "modalities": { "input": ["text", "image", "pdf"], "output": ["text"] }
      }
    }
  }
}
```

要求：
- 不复用 `/v1/models`
- 路由命名明确为推荐配置元数据用途
- 不掺入账号白名单/可调度模型逻辑

- [ ] **Step 3: 跑后端测试确认通过（GREEN）**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run '^Test(ConvertOpenCodeOpenAIModel_GPT54Capabilities|ExtractOpenCodeOpenAIModels_UsesBuiltInOpenAIProvider)$' -count=1
```

Expected: PASS

## Task 3: 让前端推荐配置消费后端元数据并保留现有自定义变体

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

- [ ] **Step 1: 扩展前端测试，要求推荐配置来自后端元数据并保留自定义变体**

把现有测试升级成：

```ts
expect(gpt54.attachment).toBe(true)
expect(gpt54.modalities.input).toEqual(expect.arrayContaining(['text', 'image', 'pdf']))
expect(gpt54Sys.attachment).toBe(true)
expect(gpt54Variants['xhigh-fast'].serviceTier).toBe('priority')
expect(gpt54SysVariants['xhigh-fast'].serviceTier).toBe('priority')
```

同时 mock 新的后端元数据接口返回值，而不是再依赖本地手写模型常量。

- [ ] **Step 2: 修改 `UseKeyModal.vue` 生成链路**

实现方向：

```ts
const openaiMeta = await adminAPI.keys.getOpenCodeOpenAIModels()

const base = Object.fromEntries(
  Object.entries(openaiMeta.models).map(([id, model]) => [
    id,
    {
      ...model,
      options: { ...model.options, store: false },
      variants: buildReasoningVariants(reasoningLevels(id), id === 'gpt-5.4'),
    },
  ]),
)

const openaiModels = withSysVariants(base)
```

要求：
- 删除对手写 `openCodeOpenAIBaseModels` 的依赖
- 继续保留 `-Sys` 变体
- 继续保留 `low-fast / medium-fast / high-fast / xhigh-fast`
- 不影响其他平台（Gemini/Claude/Antigravity）推荐配置

- [ ] **Step 3: 跑前端测试，确认通过（GREEN）**

Run:

```powershell
pnpm test:run "src/components/keys/__tests__/UseKeyModal.spec.ts"
```

Expected: PASS

## Task 4: 构建与最小能力验证

**Files:**
- Modify: `frontend/src/components/keys/UseKeyModal.vue`

- [ ] **Step 1: 运行前端构建**

Run:

```powershell
pnpm build
```

Expected: PASS（允许现有 chunk-size warnings）

- [ ] **Step 2: 做最小能力验证**

至少确认两点：

1. 新的推荐配置 JSON 中 `gpt-5.4 / gpt-5.4-Sys` 都具备：

```json
"attachment": true,
"modalities": { "input": ["text", "image", "pdf"], "output": ["text"] }
```

2. 现有 `-Sys` 与 `xhigh-fast` 不回退。

- [ ] **Step 3: 交付说明**

最终总结必须明确：
- 推荐配置现在来自后端 `models.dev` 元数据链
- 服务端 `/v1/models` 和账号白名单逻辑没有变化
- 本机 `opencode.jsonc` 仍然需要人工复制推荐配置结果

## 自查清单

- Spec 覆盖：
  - 后端拉取/缓存 `models.dev`：Task 2
  - 不动 `/v1/models` 和账号白名单：任务范围已约束
  - 前端消费后端元数据：Task 3
  - 保留 `-Sys` / `*-fast` 变体：Task 3 + Task 4
  - 最小能力验证：Task 4
- Placeholder scan：无 `TODO/TBD/implement later`
- Type consistency：统一使用 `OpenCodeOpenAIModel` / 推荐配置专用接口，不混用服务端运行时模型列表类型
