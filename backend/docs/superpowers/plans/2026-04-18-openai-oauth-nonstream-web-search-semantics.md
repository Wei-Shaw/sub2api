# OpenAI OAuth 非流式 Web Search 语义补齐 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `OpenAI OAuth + 非 compact + 非流式 /responses` 在 SSE 折叠成最终 JSON 时补回缺失的 `web_search_call` 语义、保住 `action.sources`、并把 `tool_usage.web_search.num_requests` 修正到与最终输出一致，同时不误伤 OpenCode 本地兼容、APIKey SSE fallback 与 `/responses/compact`。

**Architecture:** `apicompat` 层只扩 typed schema 与 indexed SSE 视图能力，不改变现有 `SupplementResponseOutput()` 的 empty-output-only 语义；`openai_gateway_service.go` 新增 `/responses` 主链专用 merge helper，在 raw JSON 上把 terminal response 与 SSE canonical output 合并，再做最小的 `tool_usage` 修正；OpenCode 和 APIKey 分支通过显式 gating 与回归测试锁住不外溢。修复顺序必须先补 typed/action 与 indexed 视图，再接 handler merge，最后补 OpenCode/APIKey 负向回归与 focused verification。

**Tech Stack:** Go, Gin, sjson, gjson, testify

---

### Task 1: 扩 typed schema 与 indexed SSE 视图，但不改变旧补齐语义

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go`
- Modify: `backend/internal/pkg/apicompat/responses_to_chatcompletions.go`
- Test: `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`

- [ ] **Step 1: 先写红灯测试，锁 `sources` typed 保真与 indexed output 视图**

在 `backend/internal/pkg/apicompat/chatcompletions_responses_test.go` 新增两个测试。

第一个测试锁 `WebSearchAction` 的 typed unmarshal / round-trip：

```go
func TestResponsesStreamEvent_WebSearchActionRetainsSources(t *testing.T) {
	var evt ResponsesStreamEvent
	err := json.Unmarshal([]byte(`{
		"type":"response.output_item.done",
		"output_index":0,
		"item":{
			"type":"web_search_call",
			"id":"ws_1",
			"status":"completed",
			"action":{
				"type":"search",
				"query":"openai pricing",
				"sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]
			}
		}
	}`), &evt)
	require.NoError(t, err)
	require.NotNil(t, evt.Item)
	require.NotNil(t, evt.Item.Action)
	assert.Equal(t, "search", evt.Item.Action.Type)
	assert.Equal(t, "openai pricing", evt.Item.Action.Query)
	assert.JSONEq(t, `[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]`, string(evt.Item.Action.Sources))

	raw, err := json.Marshal(evt)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"sources":[{"type":"url_citation"`)
}
```

第二个测试锁 indexed SSE 视图：

```go
func TestBufferedResponseAccumulator_BuildIndexedOutputPreservesSlots(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: 0,
		Item: &ResponsesOutput{
			Type:   "web_search_call",
			ID:     "ws_1",
			Status: "completed",
			Action: &WebSearchAction{
				Type:    "search",
				Query:   "openai pricing",
				Sources: json.RawMessage(`[{"type":"url_citation","url":"https://www.reuters.com/example"}]`),
			},
		},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", OutputIndex: 1, Delta: "search strategy"})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: 2, Delta: "pricing result"})

	indexed := acc.BuildIndexedOutput()
	require.Len(t, indexed, 3)
	assert.Equal(t, 0, indexed[0].OutputIndex)
	assert.Equal(t, "web_search_call", indexed[0].Item.Type)
	assert.Equal(t, 1, indexed[1].OutputIndex)
	assert.Equal(t, "reasoning", indexed[1].Item.Type)
	assert.Equal(t, 2, indexed[2].OutputIndex)
	assert.Equal(t, "message", indexed[2].Item.Type)
	assert.Equal(t, "pricing result", indexed[2].Item.Content[0].Text)
}
```

- [ ] **Step 2: 跑红灯，确认当前能力缺失**

Run:
`go test -tags unit ./internal/pkg/apicompat -run "TestResponsesStreamEvent_WebSearchActionRetainsSources|TestBufferedResponseAccumulator_BuildIndexedOutputPreservesSlots" -count=1`

Expected: FAIL。当前 `WebSearchAction` 会吃掉 `sources`，`BufferedResponseAccumulator` 也还没有 `BuildIndexedOutput()`。

- [ ] **Step 3: 做最小实现，只补 schema 与 indexed 视图**

在 `backend/internal/pkg/apicompat/types.go` 把 `WebSearchAction` 扩成：

```go
type WebSearchAction struct {
	Type    string          `json:"type,omitempty"`
	Query   string          `json:"query,omitempty"`
	Sources json.RawMessage `json:"sources,omitempty"`
}
```

在 `backend/internal/pkg/apicompat/responses_to_chatcompletions.go`：

1. 保留现有 `BuildOutput()` / `SupplementResponseOutput()` 行为不变。
2. 新增 indexed 视图结构：

```go
type IndexedResponsesOutput struct {
	OutputIndex int
	Item        ResponsesOutput
}
```

3. 在 accumulator 内新增按 `output_index` 收集完整 item 的能力，至少覆盖：
- `response.output_item.added`
- `response.output_item.done`
- `response.output_text.delta`
- `response.reasoning_summary_text.delta`
- `response.function_call_arguments.delta`

4. 新增方法：

```go
func (a *BufferedResponseAccumulator) BuildIndexedOutput() []IndexedResponsesOutput
```

实现要求：
- 先按 `output_index` 排序
- `response.output_item.done` 覆盖占位项
- `message` / `reasoning` / `function_call` 在只有 delta 时仍可合成
- 只提供新视图，不改老 `BuildOutput()` 的顺序约束

- [ ] **Step 4: 重跑 apicompat 红灯用例**

Run:
`go test -tags unit ./internal/pkg/apicompat -run "TestResponsesStreamEvent_WebSearchActionRetainsSources|TestBufferedResponseAccumulator_BuildIndexedOutputPreservesSlots" -count=1`

Expected: PASS

- [ ] **Step 5: 复跑旧 accumulator 回归，确认 empty-output-only 语义没动**

Run:
`go test -tags unit ./internal/pkg/apicompat -run "TestBufferedResponseAccumulator_TextOnly|TestBufferedResponseAccumulator_ToolCalls|TestBufferedResponseAccumulator_Reasoning|TestBufferedResponseAccumulator_Mixed|TestBufferedResponseAccumulator_SupplementEmptyOutput|TestBufferedResponseAccumulator_NoSupplementWhenOutputExists" -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/pkg/apicompat/types.go backend/internal/pkg/apicompat/responses_to_chatcompletions.go backend/internal/pkg/apicompat/chatcompletions_responses_test.go
git commit -m "fix(apicompat): 保留 web search sources 并补 indexed sse 视图"
```

### Task 2: 在 OAuth 非 compact 主链上补 terminal output，并修正 `tool_usage`

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 先写 handler 级红灯测试，锁主修复路径**

在 `backend/internal/service/openai_gateway_service_test.go` 新增两个核心失败用例。

第一个用例：message-only terminal 被补齐为完整 output，且最小 schema 仍完整。

```go
func TestHandleSSEToJSON_OAuthNonCompactSupplementsWebSearchCallAndToolUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	body := []byte(strings.Join([]string{
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
		``,
		`event: response.reasoning_summary_text.delta`,
		`data: {"type":"response.reasoning_summary_text.delta","output_index":1,"summary_index":0,"delta":"search strategy"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","output_index":2,"content_index":0,"delta":"pricing result"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_oauth_1","model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0,"debug":"keep-me"},"other_tool":{"count":7}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
		``,
		`data: [DONE]`,
	}, "\n"))

	usage, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, usage)
	bodyText := rec.Body.String()
	assert.Contains(t, bodyText, `"type":"web_search_call"`)
	assert.Contains(t, bodyText, `"sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]`)
	assert.Contains(t, bodyText, `"num_requests":1`)
	assert.Contains(t, bodyText, `"debug":"keep-me"`)
	assert.Contains(t, bodyText, `"other_tool":{"count":7}`)
	assert.Contains(t, bodyText, `"id":"msg_`)
	assert.Contains(t, bodyText, `"annotations":[]`)
}
```

第二个用例：terminal 已有同 `id` 的弱字段 `web_search_call`，最终补强但不重复。

```go
func TestHandleSSEToJSON_OAuthNonCompactStrengthensExistingWebSearchCallWithoutDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	body := []byte(strings.Join([]string{
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_same","status":"completed","action":{"type":"search","query":"openai pricing","sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]}}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_oauth_2","model":"gpt-5.4","output":[{"type":"web_search_call","id":"ws_same","status":"completed","action":{"type":"search","query":"openai pricing"}},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pricing result"}]}],"tool_usage":{"web_search":{"num_requests":0}},"usage":{"input_tokens":1,"output_tokens":2}}}`,
		``,
		`data: [DONE]`,
	}, "\n"))

	_, err := svc.handleSSEToJSON(resp, c, body, "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	bodyText := rec.Body.String()
	assert.Equal(t, 1, strings.Count(bodyText, `"id":"ws_same"`))
	assert.Contains(t, bodyText, `"sources":[{"type":"url_citation","title":"Reuters","url":"https://www.reuters.com/example"}]`)
	assert.Contains(t, bodyText, `"num_requests":1`)
}
```

- [ ] **Step 2: 跑红灯确认当前失败**

Run:
`go test ./internal/service -run "TestHandleSSEToJSON_OAuthNonCompactSupplementsWebSearchCallAndToolUsage|TestHandleSSEToJSON_OAuthNonCompactStrengthensExistingWebSearchCallWithoutDuplicate" -count=1`

Expected: FAIL。当前实现不会在 message-only terminal 上补齐 `web_search_call`，也不会把弱字段 item 补强，更不会修正 `num_requests`。

- [ ] **Step 3: 在 service 层实现主链专用 merge helper**

在 `backend/internal/service/openai_gateway_service.go` 新增一组主链专用 helper，推荐按下面签名收口：

```go
func shouldSupplementOAuthNonCompactResponses(c *gin.Context, account *Account) bool
func mergeCompletedResponsesOutputFromSSE(finalResponse []byte, bodyText string) ([]byte, bool, error)
func buildCanonicalOutputMapsFromSSE(bodyText string) ([]map[string]any, bool, error)
func mergeTerminalOutputWithCanonical(finalResponse map[string]any, canonical []map[string]any) (bool, error)
func reconcileWebSearchToolUsage(finalResponse map[string]any) bool
func responsesOutputToMap(item apicompat.ResponsesOutput) (map[string]any, error)
```

实现要求：

1. `shouldSupplementOAuthNonCompactResponses()` 只在 `account.Type == AccountTypeOAuth` 且 `!isOpenAIResponsesCompactPath(c)` 时返回 true。
2. canonical output 先来自 `acc.BuildIndexedOutput()`，再转成 `[]map[string]any`，避免在 handler 内手写第二套 SSE 解析。
3. merge 规则必须严格按 spec 执行：
- `web_search_call` / `message` / `reasoning` 优先用 `id`
- `function_call` 优先用 `call_id`
- 缺稳定标识时才回退到唯一可判定的 canonical 槽位
- terminal array index 绝不能直接当 `output_index`
- 命中同一 dedupe key 最终只能保留一个 item
- `web_search_call` / `function_call` / `reasoning` 以 SSE full item 为 base，terminal 只补 SSE 缺的字段
- `action.sources` / `content` / `summary` 一律整段替换，不做数组拼接
- assistant `message` 继续 terminal 优先
4. `mergeCompletedResponsesOutputFromSSE()` 必须在 raw JSON / generic map 上 patch，不能把整份 completed response typed round-trip。
5. `reconcileWebSearchToolUsage()` 只改 `tool_usage.web_search.num_requests`，并且以去重后的 merged output 中 `web_search_call` 数量为下限，不得重写 sibling 字段。

- [ ] **Step 4: 在 `handleSSEToJSON()` 主链接入 helper，然后再做最小 schema 归一化**

按这个顺序接：

1. `extractCodexFinalResponse(bodyText)` 取到 terminal response raw JSON
2. 若命中 OAuth 非 compact gating，先做 `mergeCompletedResponsesOutputFromSSE()`
3. 再执行 `normalizeResponsesJSONForAISDK()`
4. 最后保留现有 model replace / tool correction 顺序

代码骨架：

```go
if ok {
	if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
		*usage = parsedUsage
	}
	if shouldSupplementOAuthNonCompactResponses(c, account) {
		if merged, changed, err := mergeCompletedResponsesOutputFromSSE(finalResponse, bodyText); err != nil {
			return nil, fmt.Errorf("merge completed responses output from sse: %w", err)
		} else if changed {
			finalResponse = merged
		}
	} else if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
		if outputJSON, reconstructed := reconstructResponseOutputFromSSE(bodyText); reconstructed {
			if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
				finalResponse = patched
			}
		}
	}
	body = finalResponse
	...
}
```

注：如果现有函数签名拿不到 `account`，先把 `handleSSEToJSON()` 调整为接收 `account *Account`，并同步只改当前 OAuth 主链调用点；不要把 passthrough/其他共享路径一起改宽。

- [ ] **Step 5: 重跑两个主修复测试**

Run:
`go test ./internal/service -run "TestHandleSSEToJSON_OAuthNonCompactSupplementsWebSearchCallAndToolUsage|TestHandleSSEToJSON_OAuthNonCompactStrengthensExistingWebSearchCallWithoutDuplicate" -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go
git commit -m "fix(openai): 补齐 oauth 非流式 web search 响应语义"
```

### Task 3: 锁 OpenCode / APIKey 负向回归，并做最终 focused verification

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 先写 OpenCode 与 APIKey 负向回归红灯**

在 `backend/internal/service/openai_gateway_service_test.go` 继续补两个用例。

OpenCode 用例：复用 Task 2 的主失败 fixture，只加 `User-Agent: opencode/1.4.3`，断言：

```go
func TestHandleSSEToJSON_OpenCodeStillFiltersSupplementedWebSearchCallAndSkipsToolUsageFix(t *testing.T) {
	// fixture 复用 Task 2，但 c.Request.Header.Set("User-Agent", "opencode/1.4.3")
	...
	bodyText := rec.Body.String()
	assert.NotContains(t, bodyText, `"type":"web_search_call"`)
	assert.Contains(t, bodyText, `"pricing result"`)
	assert.Contains(t, bodyText, `"search strategy"`)
	assert.Contains(t, bodyText, `"num_requests":0`)
}
```

APIKey 用例：复用 Task 2 的主失败 fixture，只把 account 改为 `AccountTypeAPIKey`，断言：

```go
func TestHandleNonStreamingResponse_APIKeySSEFallbackDoesNotSupplementOAuthOnlySemantics(t *testing.T) {
	...
	bodyText := rec.Body.String()
	assert.NotContains(t, bodyText, `"type":"web_search_call"`)
	assert.Contains(t, bodyText, `"num_requests":0`)
}
```

- [ ] **Step 2: 跑红灯确认 gating 还没锁死**

Run:
`go test ./internal/service -run "TestHandleSSEToJSON_OpenCodeStillFiltersSupplementedWebSearchCallAndSkipsToolUsageFix|TestHandleNonStreamingResponse_APIKeySSEFallbackDoesNotSupplementOAuthOnlySemantics" -count=1`

Expected: FAIL

- [ ] **Step 3: 做最小 gating 修正**

在 `backend/internal/service/openai_gateway_service.go`：

1. OpenCode 路径继续走现有 `sanitizeOpenCodeResponsesOutput()`。
2. `tool_usage.web_search.num_requests` 的修正必须与 output 补齐同一 gating：
- 非 OpenCode + OAuth 非 compact 才修正
- OpenCode 路径保持原值
- APIKey SSE fallback 保持原行为

推荐把 gating 写成显式短路：

```go
applySupplement := shouldSupplementOAuthNonCompactResponses(c, account) && !isOpenCodeResponsesClient(c)
```

然后只在 `applySupplement` 为 true 时：
- merge missing/weak items
- reconcile `tool_usage.web_search.num_requests`

- [ ] **Step 4: 跑新增负向回归**

Run:
`go test ./internal/service -run "TestHandleSSEToJSON_OpenCodeStillFiltersSupplementedWebSearchCallAndSkipsToolUsageFix|TestHandleNonStreamingResponse_APIKeySSEFallbackDoesNotSupplementOAuthOnlySemantics" -count=1`

Expected: PASS

- [ ] **Step 5: 跑最终 focused verification**

Run:
`go test ./internal/service -run "TestHandleSSEToJSON_OAuthNonCompactSupplementsWebSearchCallAndToolUsage|TestHandleSSEToJSON_OAuthNonCompactStrengthensExistingWebSearchCallWithoutDuplicate|TestHandleSSEToJSON_OpenCodeStillFiltersSupplementedWebSearchCallAndSkipsToolUsageFix|TestHandleNonStreamingResponse_APIKeySSEFallbackDoesNotSupplementOAuthOnlySemantics|TestHandleNonStreamingResponse_NormalizesResponsesJSONForAISDK|TestHandleSSEToJSON_NormalizesCompletedResponsesJSONForAISDK|TestHandleNonStreamingResponse_EventStreamAppliesToAPIKeyAccounts|TestHandleNonStreamingResponse_OpenCodeFiltersWebSearchCallOutput|TestHandleSSEToJSON_OpenCodeFiltersWebSearchCallFromFinalResponse" -count=1`

Run:
`go test -tags unit ./internal/pkg/apicompat -run "TestResponsesStreamEvent_WebSearchActionRetainsSources|TestBufferedResponseAccumulator_BuildIndexedOutputPreservesSlots|TestBufferedResponseAccumulator_TextOnly|TestBufferedResponseAccumulator_ToolCalls|TestBufferedResponseAccumulator_Reasoning|TestBufferedResponseAccumulator_Mixed|TestBufferedResponseAccumulator_SupplementEmptyOutput|TestBufferedResponseAccumulator_NoSupplementWhenOutputExists" -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go backend/internal/pkg/apicompat/types.go backend/internal/pkg/apicompat/responses_to_chatcompletions.go backend/internal/pkg/apicompat/chatcompletions_responses_test.go
git commit -m "fix(openai): 补齐 oauth 非流式 web search 响应语义"
```
