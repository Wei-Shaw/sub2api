# AI Relay Codex Diff Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve Codex custom tool calls through the AI Relay OpenCode Responses bridge so Desktop/IDE and CLI clients recognize and display native file changes.

**Architecture:** Add a request-scoped bridge context that records declared tool names and whether each is a Responses `custom` tool. Downgrade custom tools to same-name Chat Completions functions with an `input` string wrapper, then use the context to restore non-streaming and streaming responses to the Codex `custom_tool_call` lifecycle.

**Tech Stack:** Go 1.26, standard `encoding/json`, `testing`, OpenAI Responses API, Chat Completions SSE.

---

## File Structure

- Modify `plugin-service/plugins/ai-relay/backend/opencode_responses.go`: request context, custom-tool request conversion, non-streaming restoration, streaming lifecycle.
- Modify `plugin-service/plugins/ai-relay/backend/opencode_responses_test.go`: focused request, continuation, non-streaming, streaming, and fallback regression tests.
- Modify `plugin-service/plugins/ai-relay/backend/platform_handlers.go`: carry request-scoped context through the upstream call and response converters.
- Modify `plugin-service/plugins/ai-relay/backend/handler_test.go`: end-to-end Responses bridge assertion for custom tools.

### Task 1: Preserve Custom Tools in Requests and Continuations

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/opencode_responses.go:13-330`
- Test: `plugin-service/plugins/ai-relay/backend/opencode_responses_test.go`

- [ ] **Step 1: Write failing request-conversion tests**

Add tests that call a new context-aware conversion entry point and assert the same-name wrapper schema and context:

```go
func TestResponsesRequestToChatCompletionsWrapsCustomToolInput(t *testing.T) {
	body, context, err := responsesRequestToChatCompletionsWithContext([]byte(`{
		"model":"deepseek-v4-flash-free",
		"input":"update the file",
		"tools":[{"type":"custom","name":"apply_patch","description":"Apply a patch"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !context.customTools["apply_patch"] || !context.declaredTools["apply_patch"] {
		t.Fatalf("context = %#v", context)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	function := payload["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "apply_patch" {
		t.Fatalf("function = %#v", function)
	}
	parameters := function["parameters"].(map[string]any)
	if parameters["required"].([]any)[0] != "input" {
		t.Fatalf("parameters = %#v", parameters)
	}
}

func TestResponsesRequestToChatCompletionsPreservesCustomToolLoop(t *testing.T) {
	body, _, err := responsesRequestToChatCompletionsWithContext([]byte(`{
		"model":"deepseek-v4-flash-free",
		"input":[
			{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":"*** Begin Patch\n*** End Patch"},
			{"type":"custom_tool_call_output","call_id":"call_1","output":"Done!"}
		],
		"tools":[{"type":"custom","name":"apply_patch"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"arguments":"{\"input\":\"*** Begin Patch\\n*** End Patch\"}"`)) {
		t.Fatalf("custom call was not wrapped: %s", body)
	}
	if !bytes.Contains(body, []byte(`"role":"tool"`)) {
		t.Fatalf("custom output was not converted: %s", body)
	}
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
go test ./plugins/ai-relay/backend -run 'TestResponsesRequestToChatCompletions(WrapsCustomToolInput|PreservesCustomToolLoop)$'
```

Expected: build failure because `responsesRequestToChatCompletionsWithContext` and the bridge context do not exist.

- [ ] **Step 3: Add the request-scoped context and freeform wrapper**

Add these units near the request converter:

```go
const customToolInputDescription = "The raw input for this tool, passed through verbatim."

type responsesBridgeContext struct {
	customTools   map[string]bool
	declaredTools map[string]bool
}

func newResponsesBridgeContext() responsesBridgeContext {
	return responsesBridgeContext{
		customTools:   map[string]bool{},
		declaredTools: map[string]bool{},
	}
}

func customToolParameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string", "description": customToolInputDescription},
		},
		"required": []any{"input"},
	}
}

func wrapCustomToolInput(input string) string {
	encoded, _ := json.Marshal(map[string]string{"input": input})
	return string(encoded)
}
```

Keep the existing compatibility entry point and move its body into the context-aware function:

```go
func responsesRequestToChatCompletions(body []byte) ([]byte, error) {
	converted, _, err := responsesRequestToChatCompletionsWithContext(body)
	return converted, err
}

func responsesRequestToChatCompletionsWithContext(body []byte) ([]byte, responsesBridgeContext, error) {
	context := newResponsesBridgeContext()
	// Existing request conversion, returning context on every path.
}
```

Update `normalizeChatTools` so `type: custom` keeps `tool["name"]`, uses `customToolParameters()`, and records both maps. Record ordinary function names only in `declaredTools`.

Update `responsesInputMessages`:

```go
case "function_call_output", "custom_tool_call_output":
	// Existing output handling.
case "function_call", "custom_tool_call":
	arguments := normalizeChatToolArguments(item["arguments"])
	if itemType == "custom_tool_call" {
		arguments = wrapCustomToolInput(stringValue(item["input"]))
	}
	// Existing pending call handling with arguments.
```

- [ ] **Step 4: Run focused and existing request tests**

Run:

```bash
go test ./plugins/ai-relay/backend -run 'TestResponsesRequestToChatCompletions'
```

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

```bash
git add plugin-service/plugins/ai-relay/backend/opencode_responses.go plugin-service/plugins/ai-relay/backend/opencode_responses_test.go
git commit -m "fix(ai-relay): preserve Codex custom tools in requests"
```

### Task 2: Restore Non-Streaming Custom Tool Calls

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/opencode_responses.go:434-490`
- Test: `plugin-service/plugins/ai-relay/backend/opencode_responses_test.go`

- [ ] **Step 1: Write failing non-streaming tests**

```go
func TestChatCompletionToResponsesRestoresCustomToolCall(t *testing.T) {
	context := newResponsesBridgeContext()
	context.customTools["apply_patch"] = true
	context.declaredTools["apply_patch"] = true
	result, err := chatCompletionToResponsesWithContext([]byte(`{
		"id":"chatcmpl_1","created":1,"model":"deepseek",
		"choices":[{"message":{"role":"assistant","tool_calls":[{
			"id":"call_1","type":"function","function":{
				"name":"apply_patch",
				"arguments":"{\"input\":\"*** Begin Patch\\n*** End Patch\"}"
			}
		}]}}]
	}`), context)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatal(err)
	}
	item := payload["output"].([]any)[0].(map[string]any)
	if item["type"] != "custom_tool_call" || item["name"] != "apply_patch" || item["input"] != "*** Begin Patch\n*** End Patch" {
		t.Fatalf("item = %#v", item)
	}
}

func TestExtractCustomToolInputFallsBackToRawArguments(t *testing.T) {
	if got := extractCustomToolInput(`{"wrong":true}`); got != `{"wrong":true}` {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
go test ./plugins/ai-relay/backend -run 'Test(ChatCompletionToResponsesRestoresCustomToolCall|ExtractCustomToolInputFallsBackToRawArguments)$'
```

Expected: build failure because the context-aware response converter and extractor do not exist.

- [ ] **Step 3: Implement non-streaming restoration**

Add:

```go
func extractCustomToolInput(arguments string) string {
	var wrapped map[string]any
	if json.Unmarshal([]byte(arguments), &wrapped) == nil {
		if input, ok := wrapped["input"].(string); ok {
			return input
		}
	}
	return arguments
}

func chatCompletionToResponses(body []byte) ([]byte, error) {
	return chatCompletionToResponsesWithContext(body, newResponsesBridgeContext())
}
```

Move the existing implementation to `chatCompletionToResponsesWithContext`. When a call name is in `context.customTools`, append:

```go
map[string]any{
	"id": functionItemID(callID), "type": "custom_tool_call", "status": "completed",
	"call_id": callID, "name": name, "input": extractCustomToolInput(arguments),
}
```

Otherwise retain the existing `function_call` item unchanged.

- [ ] **Step 4: Run non-streaming regression tests**

Run:

```bash
go test ./plugins/ai-relay/backend -run 'Test(ChatCompletionToResponses|ExtractCustomToolInput)'
```

Expected: PASS, including existing function-call tests.

- [ ] **Step 5: Commit Task 2**

```bash
git add plugin-service/plugins/ai-relay/backend/opencode_responses.go plugin-service/plugins/ai-relay/backend/opencode_responses_test.go
git commit -m "fix(ai-relay): restore non-streaming custom tool calls"
```

### Task 3: Emit the Streaming Custom Tool Lifecycle

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/opencode_responses.go:530-730`
- Test: `plugin-service/plugins/ai-relay/backend/opencode_responses_test.go`

- [ ] **Step 1: Write a failing streaming lifecycle test**

Add a test that parses event text and checks both presence and absence:

```go
func TestChatCompletionSSEToResponsesRestoresCustomToolLifecycle(t *testing.T) {
	context := newResponsesBridgeContext()
	context.customTools["apply_patch"] = true
	context.declaredTools["apply_patch"] = true
	stream := "data: {\"id\":\"chatcmpl_1\",\"model\":\"deepseek\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"apply_patch\",\"arguments\":\"{\\\"input\\\":\\\"*** Begin Patch\\\\n*** End Patch\\\"}\"}}]}}]}\n\n" +
		"data: [DONE]\n\n"
	converted := chatCompletionSSEToResponsesWithContext([]byte(stream), context)
	for _, event := range []string{
		"response.output_item.added",
		"response.custom_tool_call_input.delta",
		"response.custom_tool_call_input.done",
		"response.output_item.done",
		"response.completed",
	} {
		if !bytes.Contains(converted, []byte("event: "+event)) {
			t.Fatalf("missing %s in %s", event, converted)
		}
	}
	if bytes.Contains(converted, []byte("response.function_call_arguments")) {
		t.Fatalf("custom call used function lifecycle: %s", converted)
	}
	if !bytes.Contains(converted, []byte(`"type":"custom_tool_call"`)) ||
		!bytes.Contains(converted, []byte(`"input":"*** Begin Patch\\n*** End Patch"`)) {
		t.Fatalf("custom output missing: %s", converted)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```bash
go test ./plugins/ai-relay/backend -run TestChatCompletionSSEToResponsesRestoresCustomToolLifecycle
```

Expected: build failure because `chatCompletionSSEToResponsesWithContext` does not exist.

- [ ] **Step 3: Track and announce the correct tool type**

Extend `responseToolCall`:

```go
type responseToolCall struct {
	callID, name, arguments string
	outputIndex             int
	announced               bool
	custom                  bool
}
```

Store `responsesBridgeContext` in `responseStreamState`. Add a helper that emits `response.output_item.added` only after the accumulated name matches a declared tool, or at finish for an unknown tool. For custom tools emit an item with `type: custom_tool_call`, `name`, and empty `input`; for ordinary tools retain `function_call`, `name`, and empty `arguments`.

For custom calls, buffer Chat JSON arguments without emitting `response.function_call_arguments.delta`. At finish, extract the freeform input and emit:

```go
s.emit(w, "response.custom_tool_call_input.delta", map[string]any{
	"type": "response.custom_tool_call_input.delta", "output_index": call.outputIndex,
	"item_id": itemID, "delta": input,
})
s.emit(w, "response.custom_tool_call_input.done", map[string]any{
	"type": "response.custom_tool_call_input.done", "output_index": call.outputIndex,
	"item_id": itemID, "call_id": call.callID, "name": call.name, "input": input,
})
```

Use a context-aware entry point while preserving existing tests:

```go
func chatCompletionSSEToResponses(stream []byte) []byte {
	return chatCompletionSSEToResponsesWithContext(stream, newResponsesBridgeContext())
}
```

Ensure `response.output_item.done` and `response.completed.response.output` contain `custom_tool_call` with `input`; ordinary calls remain unchanged.

- [ ] **Step 4: Run all stream conversion tests**

Run:

```bash
go test ./plugins/ai-relay/backend -run TestChatCompletionSSEToResponses
```

Expected: PASS, including the existing text, reasoning, indexing, and single-completion assertions.

- [ ] **Step 5: Commit Task 3**

```bash
git add plugin-service/plugins/ai-relay/backend/opencode_responses.go plugin-service/plugins/ai-relay/backend/opencode_responses_test.go
git commit -m "fix(ai-relay): emit Codex custom tool stream events"
```

### Task 4: Carry Context Through the Relay Handler

**Files:**
- Modify: `plugin-service/plugins/ai-relay/backend/platform_handlers.go:15-43`
- Test: `plugin-service/plugins/ai-relay/backend/handler_test.go`

- [ ] **Step 1: Write a failing end-to-end handler test**

Add a test modeled after `TestOpenCodeResponsesBridgeUsesChatCompletions`. The fake upstream must assert that a Responses `custom apply_patch` tool arrives as a same-name Chat function with an `input` parameter, then return an `apply_patch` tool call. Assert the relay response contains:

```go
require.Contains(t, recorder.Body.String(), `"type":"custom_tool_call"`)
require.Contains(t, recorder.Body.String(), `"name":"apply_patch"`)
require.Contains(t, recorder.Body.String(), `"input":"*** Begin Patch`)
```

- [ ] **Step 2: Run the handler test and verify RED**

Run:

```bash
go test ./plugins/ai-relay/backend -run TestOpenCodeResponsesBridgeRestoresCodexCustomTool
```

Expected: FAIL because `OpenCodeAdapter.Handle` still calls the context-free response converters.

- [ ] **Step 3: Thread the request context through `OpenCodeAdapter.Handle`**

Replace the Responses branch conversion calls with:

```go
body, bridgeContext, err := responsesRequestToChatCompletionsWithContext(request.Body)
// Existing forwarding and error handling.
if strings.Contains(strings.ToLower(response.Headers.Get("Content-Type")), "text/event-stream") {
	response.Body = chatCompletionSSEToResponsesWithContext(response.Body, bridgeContext)
	return response, nil
}
response.Body, err = chatCompletionToResponsesWithContext(response.Body, bridgeContext)
```

Do not store the context globally or on `OpenCodeAdapter`; it belongs to one request and must remain race-free.

- [ ] **Step 4: Run focused and package-level tests**

Run:

```bash
go test ./plugins/ai-relay/backend
```

Expected: PASS.

- [ ] **Step 5: Commit Task 4**

```bash
git add plugin-service/plugins/ai-relay/backend/platform_handlers.go plugin-service/plugins/ai-relay/backend/handler_test.go
git commit -m "fix(ai-relay): carry custom tool context through responses"
```

### Task 5: Format and Verify the Plugin Service

**Files:**
- Modify only files changed in Tasks 1-4 through formatting.

- [ ] **Step 1: Format changed Go files**

Run:

```bash
gofmt -w plugins/ai-relay/backend/opencode_responses.go plugins/ai-relay/backend/opencode_responses_test.go plugins/ai-relay/backend/platform_handlers.go plugins/ai-relay/backend/handler_test.go
```

Expected: exit 0.

- [ ] **Step 2: Run all plugin-service tests**

Run:

```bash
go test ./...
```

Expected: PASS with no failing packages.

- [ ] **Step 3: Inspect the final diff**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only the intended AI Relay files are modified or committed.

- [ ] **Step 4: Commit formatting corrections if needed**

If `gofmt` changed files after the task commits:

```bash
git add plugin-service/plugins/ai-relay/backend/opencode_responses.go plugin-service/plugins/ai-relay/backend/opencode_responses_test.go plugin-service/plugins/ai-relay/backend/platform_handlers.go plugin-service/plugins/ai-relay/backend/handler_test.go
git commit -m "style(ai-relay): format custom tool bridge"
```

If formatting produced no diff, do not create an empty commit.
