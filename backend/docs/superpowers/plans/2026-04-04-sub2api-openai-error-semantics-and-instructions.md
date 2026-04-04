# sub2api OpenAI Error Semantics And Instructions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正 `sub2api` 在 OpenAI `/v1/responses` 路径上的错误包装与 `instructions` 兼容，让 OpenCode 不再把 upstream 400 坏图错误误判为可重试 502，同时避免 rich system prompt 被弱 fallback 稀释。

**Architecture:** 这次实现拆成两条并行但低耦合的修复线。第一条线收敛非 passthrough 的 upstream 错误语义：保留 upstream HTTP 状态码，并在 `sub2api` 错误外壳里完整携带 upstream 原始字段。第二条线统一 `instructions` 补齐逻辑：非 passthrough 和 passthrough 都不再硬塞弱 fallback，而是优先从已有 system message 中提升出同源指令文本。

**Tech Stack:** Go, Gin, sub2api OpenAI gateway service/handler tests, JSON error mapping, local OpenCode compatibility reasoning.

---

## 文件结构

- Modify: `backend/internal/service/openai_gateway_service.go`
  OpenAI 非 passthrough / passthrough 错误返回与 `instructions` 兼容的主实现位置。
- Modify: `backend/internal/service/openai_gateway_service_test.go`
  追加 service 级回归测试，锁定错误语义和 passthrough `instructions` 行为。
- Modify: `backend/internal/service/error_passthrough_runtime_test.go`
  补当前“上游 400 不该被包装成 502”的行为测试。
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`
  如有必要，锁定 handler 侧最终出站错误体形态，保证客户端看到的是新契约。

## 任务拆分

### Task 1: 先写失败测试，锁定非 passthrough 400 错误语义

**Files:**
- Modify: `backend/internal/service/error_passthrough_runtime_test.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 给非 passthrough 上游 400 写一个失败测试**

在 `backend/internal/service/error_passthrough_runtime_test.go` 增加一个测试，意图如下：

```go
func TestOpenAIHandleErrorResponse_PreservesStructuredUpstream400(t *testing.T) {
  // 上游返回 400 invalid image
  // 期望客户端响应仍是 400，而不是 502
  // 期望 error.type 仍然标记为 upstream_error
  // 期望 body 中能拿到 upstream.status/code/type/message/param/raw
}
```

- [ ] **Step 2: 跑测试确认当前实现确实失败**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run TestOpenAIHandleErrorResponse_PreservesStructuredUpstream400 -count=1
```

Expected: FAIL，当前实现仍返回 502 / `Upstream request failed`。

- [ ] **Step 3: 补一个客户端可见错误体结构测试**

在 `backend/internal/service/openai_gateway_service_test.go` 增加一个更小的结构测试，锁定最终 JSON 形态：

```go
func TestBuildOpenAIUpstreamErrorEnvelope(t *testing.T) {
  // 输入 upstream status/body
  // 断言输出 envelope 保留 upstream.status/code/type/message/param/raw
}
```

- [ ] **Step 4: 跑测试确认第二个测试也失败**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run TestBuildOpenAIUpstreamErrorEnvelope -count=1
```

Expected: FAIL，相关 helper 尚不存在。

### Task 2: 最小实现非 passthrough 错误语义修复

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/error_passthrough_runtime_test.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 实现统一的 upstream error envelope helper**

在 `backend/internal/service/openai_gateway_service.go` 增加一个小 helper，职责单一：

```go
type openAIUpstreamErrorEnvelope struct {
  Error struct {
    Type     string         `json:"type"`
    Message  string         `json:"message"`
    Upstream map[string]any `json:"upstream,omitempty"`
  } `json:"error"`
}

func buildOpenAIUpstreamErrorEnvelope(status int, body []byte, fallback string) (int, openAIUpstreamErrorEnvelope) {
  // 若 body 是可解析 OpenAI 错误，保留 status 和原始字段
  // 否则回落到现有 fallback 语义
}
```

- [ ] **Step 2: 在 `handleErrorResponse(...)` 中改用新 helper**

要求：

```go
// 结构化 4xx/5xx upstream error:
// HTTP status 直接沿用 upstream
// body = sub2api envelope + upstream 原始字段
```

仍然保留这些行为不变：

1. `rateLimitService.HandleUpstreamError(...)`
2. `appendOpsUpstreamError(...)`
3. `UpstreamFailoverError` 分支

- [ ] **Step 3: 如有必要，在 handler 测试中锁最终出站 body**

如果 service 测试无法覆盖最终出站 JSON，就在 `backend/internal/handler/openai_gateway_handler_test.go` 增一个最小 handler 测试，断言：

```json
HTTP 400
{
  "error": {
    "type": "upstream_error",
    "upstream": {
      "code": "invalid_value"
    }
  }
}
```

- [ ] **Step 4: 跑相关测试确认转绿**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run "Test(OpenAIHandleErrorResponse_PreservesStructuredUpstream400|BuildOpenAIUpstreamErrorEnvelope)" -count=1
```

Expected: PASS。

### Task 3: 写失败测试并修正 instructions 兼容策略（先非 passthrough，再 passthrough）

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 先为非 passthrough 的弱 fallback 写失败测试**

在 `backend/internal/service/openai_gateway_service_test.go` 增加测试：

```go
func TestPrepareOpenAIRequestBody_PromotesSystemMessageToInstructions(t *testing.T) {
  // body 没有 instructions，但有 system message / system-like input
  // 期望最终 instructions 不是 "You are a helpful coding assistant."
  // 而是从现有 rich prompt 提升出来的内容
}
```

- [ ] **Step 2: 为 passthrough + OAuth + gpt-5.4 缺 instructions 写失败测试**

继续在同一测试文件里加：

```go
func TestNormalizeOpenAIPassthroughOAuthBody_PromotesInstructions(t *testing.T) {
  // passthrough body 初始没有 instructions
  // 期望 normalize 后会得到非空 instructions
}
```

- [ ] **Step 3: 跑测试确认当前实现失败**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run "Test(PrepareOpenAIRequestBody_PromotesSystemMessageToInstructions|NormalizeOpenAIPassthroughOAuthBody_PromotesInstructions)" -count=1
```

Expected: FAIL，当前仍补弱 fallback / passthrough 不补 `instructions`。

- [ ] **Step 4: 实现统一的 instructions 提升 helper**

在 `backend/internal/service/openai_gateway_service.go` 增加一个 helper，职责如下：

```go
func deriveOpenAIInstructions(reqBody map[string]any) string {
  // 优先用已有 instructions
  // 否则从 system message / system-like input 中提炼文本
  // 不要回落到固定的 "You are a helpful coding assistant."
}
```

要求：

1. 非 passthrough 用它补齐 `instructions`
2. passthrough `normalizeOpenAIPassthroughOAuthBody(...)` 也用同一套逻辑补齐
3. 两条路径不再各自复制 fallback 规则

- [ ] **Step 5: 跑测试确认转绿**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run "Test(PrepareOpenAIRequestBody_PromotesSystemMessageToInstructions|NormalizeOpenAIPassthroughOAuthBody_PromotesInstructions)" -count=1
```

Expected: PASS。

### Task 4: 做回归验证，确认 OpenCode 兼容性目标达成

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service_test.go`
- Modify: `backend/internal/service/error_passthrough_runtime_test.go`

- [ ] **Step 1: 跑 service 全量相关测试**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -count=1
```

Expected: PASS。

- [ ] **Step 2: 跑格式与差异检查**

Run:

```powershell
gofmt -w backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go backend/internal/service/error_passthrough_runtime_test.go
git diff --check
```

Expected: PASS。

- [ ] **Step 3: 记录一条端到端验证脚本（不在本任务中自动改生产）**

验证脚本至少要覆盖：

```text
1. 发一条带坏图的 /v1/responses 请求，确认客户端拿到的是 400 + upstream 明细，而不是 502 Upstream request failed
2. 发一条 passthrough + GPT-5.4 请求，确认不再因为缺 instructions 被 400 拦住
3. 再看 OpenCode 是否仍把第一条错误判成 retryable
```

- [ ] **Step 4: 输出交付结论**

结论里必须明确：

1. 非 passthrough 上游错误现在的客户端契约
2. passthrough instructions 的兼容策略
3. 哪些问题这次修了，哪些（如坏图精准定位/清理）还没有做

## 自查清单

- Spec 覆盖：
  - upstream status + structured body 契约：Task 1 + Task 2
  - 透传/非透传 instructions 统一兼容：Task 3
  - 不做坏图清理：计划中未加入该实现任务
  - OpenCode 重试兼容验证：Task 4
- 占位词检查：没有 `TODO/TBD/implement later` 等占位描述。
- 类型/命名一致性：本计划统一使用 `upstream error envelope`、`deriveOpenAIInstructions`、`handleErrorResponse(...)` / `normalizeOpenAIPassthroughOAuthBody(...)` 这些命名，不混用别名。
