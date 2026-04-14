# OpenAI Built-in Tools Augmentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 OpenAI HTTP 主链上新增 body 顶层私有参数 `builtin_tools`，第一阶段仅支持稳定补齐 `web_search`，且不影响用户原始 `tools`/`tool_choice` 语义。

**Architecture:** 在 OpenAI 请求进入最终 upstream body 构建前，先把 `builtin_tools` 解析成一个最小 built-in augmentation 结果，再与原始 `tools` 合并并移除私有字段。实现只覆盖 `/v1/responses` 与 `/v1/chat/completions -> responses`，显式排除 passthrough、compact 和客户端 WS/realtime 入口。

**Tech Stack:** Go, Gin, JSON request transformation, Go tests

---

### Task 1: 提取 `builtin_tools` 归一化 helper

**Files:**
- Create: `backend/internal/service/openai_builtin_tools.go`
- Test: `backend/internal/service/openai_builtin_tools_test.go`

- [ ] **Step 1: 写失败测试，锁定 phase 1 归一化语义**

```go
func TestNormalizeOpenAIBuiltinTools(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want []map[string]any
	}{
		{
			name: "bool true adds web_search",
			raw:  true,
			want: []map[string]any{{"type": "web_search"}},
		},
		{
			name: "list keeps only web_search",
			raw:  []any{"web_search", "code_interpreter"},
			want: []map[string]any{{"type": "web_search"}},
		},
		{
			name: "object form keeps explicit true",
			raw:  map[string]any{"web_search": true, "image_generation": true},
			want: []map[string]any{{"type": "web_search"}},
		},
		{
			name: "false or empty disables augmentation",
			raw:  false,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeOpenAIBuiltinTools(tt.raw)
			require.Equal(t, tt.want, got)
		})
	}
}
```

- [ ] **Step 2: 跑测试并确认失败**

Run: `go test ./internal/service -run "NormalizeOpenAIBuiltinTools" -count=1`

Expected: FAIL because helper does not exist yet.

- [ ] **Step 3: 实现最小 helper**

```go
func normalizeOpenAIBuiltinTools(raw any) []map[string]any {
	addWebSearch := false
	switch v := raw.(type) {
	case bool:
		addWebSearch = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) == "web_search" {
				addWebSearch = true
				break
			}
		}
	case []string:
		for _, item := range v {
			if strings.TrimSpace(item) == "web_search" {
				addWebSearch = true
				break
			}
		}
	case map[string]any:
		if b, ok := v["web_search"].(bool); ok && b {
			addWebSearch = true
		}
	}
	if !addWebSearch {
		return nil
	}
	return []map[string]any{{"type": "web_search"}}
}
```

- [ ] **Step 4: 复跑测试并确认通过**

Run: `go test ./internal/service -run "NormalizeOpenAIBuiltinTools" -count=1`

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/openai_builtin_tools.go backend/internal/service/openai_builtin_tools_test.go
git commit -m "feat(openai): 增加 builtin_tools 归一化 helper"
```

### Task 2: 在 `/v1/responses` 主链接入 built-in augmentation

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 写失败测试，锁定 `/v1/responses` augment + strip 行为**

```go
func TestForwardResponsesRequest_AugmentsBuiltinToolsAndStripsPrivateField(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4",
		"builtin_tools": true,
		"tools": []any{
			map[string]any{"type": "function", "name": "get_weather", "parameters": map[string]any{"type": "object"}},
		},
	}

	changed := applyOpenAIBuiltinToolsAugmentation(reqBody)
	require.True(t, changed)
	require.NotContains(t, reqBody, "builtin_tools")
	tools := reqBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
	require.Equal(t, "web_search", tools[1].(map[string]any)["type"])
}
```

- [ ] **Step 2: 跑测试并确认失败**

Run: `go test ./internal/service -run "ForwardResponsesRequest_AugmentsBuiltinToolsAndStripsPrivateField" -count=1`

Expected: FAIL because augmentation hook does not exist.

- [ ] **Step 3: 在 `openai_gateway_service.go` 接入 augmentation**

在普通 body 解析完成后、真正序列化和上游构建前加入，且显式 guard 非 compact 主链：

```go
if !isOpenAIResponsesCompactPath(c) && applyOpenAIBuiltinToolsAugmentation(reqBody) {
	bodyModified = true
	disablePatch()
}
```

并新增 helper：

```go
func applyOpenAIBuiltinToolsAugmentation(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	raw, ok := reqBody["builtin_tools"]
	if !ok {
		return false
	}
	delete(reqBody, "builtin_tools")
	augmented := normalizeOpenAIBuiltinTools(raw)
	if len(augmented) == 0 {
		return true // stripped private field only
	}
	existing, _ := reqBody["tools"].([]any)
	if hasOpenAIBuiltinTool(existing, "web_search") {
		if existing != nil {
			reqBody["tools"] = existing
		}
		return true
	}
	reqBody["tools"] = append(existing, any(augmented[0]))
	return true
}

func hasOpenAIBuiltinTool(tools []any, toolType string) bool {
	for _, tool := range tools {
		m, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(m["type"])) == toolType {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: 显式在 passthrough 分支 strip 私有字段并保持不生效**

因为 passthrough 会在主链 body augment 前提前返回，所以必须单独处理，不能只靠主链负测。

先新增一个共享 strip helper：

```go
func stripOpenAIBuiltinToolsField(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}
	if _, ok := reqBody["builtin_tools"]; !ok {
		return false
	}
	delete(reqBody, "builtin_tools")
	return true
}
```

然后在 `forwardOpenAIPassthrough(...)` 的真实 body 入口层加入，而不是只写在伪代码里的 `reqBody/bodyModified`：

```go
var passthroughReqBody map[string]any
if err := json.Unmarshal(body, &passthroughReqBody); err == nil {
	if stripOpenAIBuiltinToolsField(passthroughReqBody) {
		body, _ = json.Marshal(passthroughReqBody)
	}
}
```

要求：

- passthrough 不追加 `web_search`
- `builtin_tools` 也不能原样泄漏到 upstream

并补两条测试：

```go
func TestForwardResponsesRequest_PassthroughStripsBuiltinToolsWithoutAugmenting(t *testing.T) {}

func TestForwardResponsesRequest_CompactPathDoesNotAugmentBuiltinTools(t *testing.T) {}
```

- [ ] **Step 5: 复跑定向测试**

Run: `go test ./internal/service -run "BuiltinTools|AugmentsBuiltinTools|PassthroughDoesNotAugment|CompactPathDoesNotAugment" -count=1`

Expected: PASS.

- [ ] **Step 6: 提交**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go
git commit -m "feat(openai): 在 responses 主链补齐 web_search 内置工具"
```

### Task 3: 在 `chat/completions -> responses` 兼容链接入 augmentation

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go`
- Modify: `backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `backend/internal/service/openai_compat_prompt_cache_key.go`
- Test: `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`
- Test: `backend/internal/service/openai_compat_prompt_cache_key_test.go`

- [ ] **Step 1: 写失败测试，锁定 compat 路径也会生效**

```go
func TestChatCompletionsBuiltinTools_AugmentsWebSearchWithoutChangingToolChoice(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model:        "gpt-5.4",
		Messages:     []apicompat.ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
		BuiltinTools: true,
		ToolChoice:   json.RawMessage(`"auto"`),
	}

	changed := applyOpenAICompatBuiltinToolsAugmentation(req)
	require.True(t, changed)
	require.Nil(t, req.BuiltinTools)
	require.Len(t, req.Tools, 1)
	require.Equal(t, "web_search", req.Tools[0].Type)
	require.JSONEq(t, `"auto"`, string(req.ToolChoice))
}
```

- [ ] **Step 2: 跑测试并确认失败**

Run: `go test ./internal/pkg/apicompat ./internal/service -run "ChatCompletionsBuiltinTools" -count=1`

Expected: FAIL because compat augmentation hook does not exist.

- [ ] **Step 3: 扩展 compat 请求结构并接入 augmentation**

在 `types.go` 的 `ChatCompletionsRequest` 增加：

```go
BuiltinTools any `json:"builtin_tools,omitempty"`
```

在 `openai_gateway_chat_completions.go` 的请求反序列化后、`ChatCompletionsToResponses` 前接入：

```go
if applyOpenAICompatBuiltinToolsAugmentation(&chatReq) {
	// chatReq.Tools updated, BuiltinTools cleared
}
```

helper 示例：

```go
func applyOpenAICompatBuiltinToolsAugmentation(req *apicompat.ChatCompletionsRequest) bool {
	if req == nil {
		return false
	}
	augmented := normalizeOpenAIBuiltinTools(req.BuiltinTools)
	req.BuiltinTools = nil
	if len(augmented) == 0 {
		return true
	}
	for _, tool := range req.Tools {
		if strings.TrimSpace(tool.Type) == "web_search" {
			return true
		}
	}
	req.Tools = append(req.Tools, apicompat.ChatTool{Type: "web_search"})
	return true
}
```

- [ ] **Step 4: 让 compat `prompt_cache_key` 看到 augmentation 后的工具集**

当前 `prompt_cache_key` 是在 augmentation 前基于原始 `chatReq` 推导的，所以需要同步修正。

在 `openai_gateway_chat_completions.go` 中调整顺序：

```go
// 先应用 builtin_tools augmentation
if applyOpenAICompatBuiltinToolsAugmentation(&chatReq) {
	// chatReq.Tools updated, BuiltinTools cleared
}

// 再基于 augmentation 后的工具集推导 compat prompt cache key
if promptCacheKey == "" && account.Type == AccountTypeOAuth && shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
	promptCacheKey = deriveCompatPromptCacheKey(&chatReq, upstreamModel)
	compatPromptCacheInjected = promptCacheKey != ""
}
```

并在 `openai_compat_prompt_cache_key_test.go` 增加回归：

```go
func TestDeriveCompatPromptCacheKey_IncludesAugmentedBuiltinTools(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	}
	withBuiltin := *base
	withBuiltin.Tools = append(withBuiltin.Tools, apicompat.ChatTool{Type: "web_search"})
	require.NotEqual(t,
		deriveCompatPromptCacheKey(base, "gpt-5.4"),
		deriveCompatPromptCacheKey(&withBuiltin, "gpt-5.4"),
	)
}
```

- [ ] **Step 5: 补 compat 正向回归与 `tool_choice` 不变断言**

```go
func TestChatCompletionsToResponses_PreservesToolChoiceWhenBuiltinToolsAdded(t *testing.T) { /* tool_choice remains raw/converted normally */ }
```

- [ ] **Step 6: 复跑定向测试**

Run: `go test ./internal/pkg/apicompat ./internal/service -run "ChatCompletionsBuiltinTools|ToolChoiceWhenBuiltinToolsAdded|DeriveCompatPromptCacheKey_IncludesAugmentedBuiltinTools" -count=1`

Expected: PASS.

- [ ] **Step 7: 提交**

```bash
git add backend/internal/pkg/apicompat/types.go backend/internal/service/openai_gateway_chat_completions.go backend/internal/service/openai_compat_prompt_cache_key.go backend/internal/pkg/apicompat/chatcompletions_responses_test.go backend/internal/service/openai_gateway_service_test.go backend/internal/service/openai_compat_prompt_cache_key_test.go
git commit -m "feat(openai): 在 chat compat 链补齐 web_search built-in"
```

### Task 4: 全量验证与边界确认

**Files:**
- No new files expected

- [ ] **Step 1: 跑完整后端验证**

Run:
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`

Expected: all pass.

- [ ] **Step 2: 手工检查排除边界仍成立**

Confirm in code/tests:
- passthrough path unchanged
- compact path unchanged
- no sticky/ops/dashboard files changed
- no image/code/file built-ins accidentally enabled

- [ ] **Step 3: 提交收尾说明**

```bash
git status --short
git log -4 --oneline
```

Expected: only intended files changed; commit history shows the feature split clearly.
