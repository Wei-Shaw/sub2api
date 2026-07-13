# Native Responses Namespace Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make native OpenAI Responses OAuth passthrough accept Codex namespace tools by flattening requests and restoring namespace identity in JSON and SSE responses.

**Architecture:** Add a focused, request-local namespace adapter in `internal/pkg/apicompat`. The passthrough service transforms the request once, carries the returned flat-name mapping through the response handlers, and restores mapped function calls immediately before writing downstream bytes.

**Tech Stack:** Go, Gin, `encoding/json`, `tidwall/gjson`, `testify`.

## Global Constraints

- Target only native HTTP Responses OAuth passthrough demonstrated by issue #4135.
- Reuse the chat bridge's collision-safe flattened naming convention.
- Preserve unrelated request fields, ordinary tools, and unmapped response calls.
- Support JSON and SSE response restoration, including terminal completed output.
- Return a client-visible 400-class error for ambiguous flattened names.
- Do not change WebSocket forwarding, connector security policy, or account scheduling.

---

## File Structure

- Create `backend/internal/pkg/apicompat/responses_namespace.go`: transport-independent request flattening, mapping, and response restoration.
- Create `backend/internal/pkg/apicompat/responses_namespace_test.go`: unit tests for transformations and collisions.
- Modify `backend/internal/service/openai_gateway_passthrough.go`: apply the adapter and thread its mapping through HTTP response handling.
- Modify `backend/internal/service/openai_oauth_passthrough_test.go`: end-to-end regression tests for OAuth passthrough request and SSE response behavior.

### Task 1: Request Namespace Adapter

**Files:**
- Create: `backend/internal/pkg/apicompat/responses_namespace.go`
- Create: `backend/internal/pkg/apicompat/responses_namespace_test.go`

**Interfaces:**
- Produces: `type ResponsesNamespaceName struct { Namespace string; Name string REDACTED`
- Produces: `func FlattenResponsesNamespaces(req map[string]any) (map[string]ResponsesNamespaceName, bool, error)`

- [ ] **Step 1: Write failing request transformation tests**

Add table-driven tests that construct a request with a namespace declaration, a historical `function_call`, and a namespace `tool_choice`. Assert that the namespace child becomes an ordinary top-level function named `collaboration__spawn_agent`, historical input uses the same name without `namespace`, the choice is rewritten, and unrelated tools/fields are unchanged. Add collision cases for a top-level flat-name conflict and two namespace pairs that flatten identically.

- [ ] **Step 2: Run the tests and verify RED**

Run: `cd backend && go test ./internal/pkg/apicompat -run 'TestFlattenResponsesNamespaces' -count=1`

Expected: build failure because `FlattenResponsesNamespaces` does not exist.

- [ ] **Step 3: Implement the minimal request adapter**

Decode `tools` and optional `children`/nested `tools` values from `map[string]any`, call the package's existing `flattenNamespaceToolName`, and build a flat-name ownership map. Replace each namespace declaration with its function children, rewrite matching `input` function calls and namespace `tool_choice`, and return a descriptive error when ownership is ambiguous.

- [ ] **Step 4: Run the tests and verify GREEN**

Run: `cd backend && go test ./internal/pkg/apicompat -run 'TestFlattenResponsesNamespaces' -count=1`

Expected: PASS.

### Task 2: Response Namespace Restoration

**Files:**
- Modify: `backend/internal/pkg/apicompat/responses_namespace.go`
- Modify: `backend/internal/pkg/apicompat/responses_namespace_test.go`

**Interfaces:**
- Consumes: `map[string]ResponsesNamespaceName`
- Produces: `func RestoreResponsesNamespaceCalls(payload []byte, names map[string]ResponsesNamespaceName) ([]byte, bool, error)`

- [ ] **Step 1: Write failing response restoration tests**

Cover a direct `function_call`, `response.output_item.added`, `response.output_item.done`, and `response.completed.response.output`. Assert that a mapped name is restored to the child name and gains `namespace`, while an ordinary call and unknown JSON fields remain unchanged.

- [ ] **Step 2: Run the tests and verify RED**

Run: `cd backend && go test ./internal/pkg/apicompat -run 'TestRestoreResponsesNamespaceCalls' -count=1`

Expected: build failure because `RestoreResponsesNamespaceCalls` does not exist.

- [ ] **Step 3: Implement recursive, type-constrained restoration**

Unmarshal the payload, visit maps and arrays, and rewrite only maps whose `type` is `function_call` and whose string `name` exists in the request mapping. Set the original child `name` and `namespace`, then marshal only when a change occurred.

- [ ] **Step 4: Run the tests and verify GREEN**

Run: `cd backend && go test ./internal/pkg/apicompat -run 'TestRestoreResponsesNamespaceCalls' -count=1`

Expected: PASS.

### Task 3: OAuth Passthrough Integration

**Files:**
- Modify: `backend/internal/service/openai_gateway_passthrough.go`
- Modify: `backend/internal/service/openai_oauth_passthrough_test.go`

**Interfaces:**
- Consumes: `apicompat.FlattenResponsesNamespaces` and `apicompat.RestoreResponsesNamespaceCalls`.
- Changes the private passthrough response handlers to accept the request-local namespace mapping.

- [ ] **Step 1: Write a failing end-to-end streaming regression test**

Send an OAuth passthrough body containing `tools:[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent",...REDACTED]REDACTED]` and a historical input call tagged with `namespace:"collaboration"`. Return SSE added/done/completed events using `collaboration__spawn_agent`. Assert the recorded upstream request has no namespace declarations or input namespace fields and the downstream SSE restores `name:"spawn_agent"` plus `namespace:"collaboration"` in every function-call item.

- [ ] **Step 2: Run the streaming regression and verify RED**

Run: `cd backend && go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_Namespace' -count=1`

Expected: FAIL because the upstream body still contains namespace fields or the downstream body retains the flat name.

- [ ] **Step 3: Integrate request flattening and SSE restoration**

After OAuth passthrough normalization, unmarshal the body, call `FlattenResponsesNamespaces`, marshal with the repository's non-HTML-escaping JSON helper, and retain the mapping. Pass it into `handleStreamingResponsePassthrough`; after existing argument/model normalization, call `RestoreResponsesNamespaceCalls` on each SSE data payload and rebuild the `data:` line.

- [ ] **Step 4: Run the streaming regression and verify GREEN**

Run: `cd backend && go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_Namespace' -count=1`

Expected: PASS.

- [ ] **Step 5: Write a failing non-stream JSON regression test**

Exercise the non-stream handler with a completed Responses JSON body whose output contains `collaboration__spawn_agent`, then assert the client receives `spawn_agent` and `namespace:"collaboration"`.

- [ ] **Step 6: Run the JSON regression and verify RED**

Run: `cd backend && go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_NamespaceNonStreaming' -count=1`

Expected: FAIL because JSON response restoration is not wired.

- [ ] **Step 7: Integrate JSON and SSE-to-JSON restoration**

Pass the mapping into `handleNonStreamingResponsePassthrough` and `handlePassthroughSSEToJSON`. Restore namespace calls after model correction and before writing the body. Return transformation errors instead of writing partially transformed output.

- [ ] **Step 8: Run all namespace regressions and verify GREEN**

Run: `cd backend && go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_Namespace' -count=1`

Expected: PASS.

### Task 4: Client Error and Regression Verification

**Files:**
- Modify: `backend/internal/service/openai_gateway_passthrough.go`
- Modify: `backend/internal/service/openai_oauth_passthrough_test.go`

- [ ] **Step 1: Write a failing service collision test**

Send an OAuth passthrough request whose namespace child conflicts with an existing top-level function. Assert no upstream request occurs and the client receives status 400 with `invalid_request_error` and a collision explanation.

- [ ] **Step 2: Run the collision test and verify RED**

Run: `cd backend && go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_NamespaceCollision' -count=1`

Expected: FAIL because the adapter error is not yet mapped to a 400 response.

- [ ] **Step 3: Map adapter failures to the existing invalid-request response shape**

When request flattening returns an error, set the ops upstream error and write `{"error":{"type":"invalid_request_error","message":err.Error(),"param":"tools"REDACTEDREDACTED` with HTTP 400, then return the same error without contacting upstream.

- [ ] **Step 4: Run targeted suites**

Run: `cd backend && go test ./internal/pkg/apicompat ./internal/service -run 'Namespace|OAuthPassthrough_StreamKeepsToolNameAndBodyNormalized' -count=1`

Expected: PASS.

- [ ] **Step 5: Format and run full relevant package tests**

Run: `cd backend && gofmt -w internal/pkg/apicompat/responses_namespace.go internal/pkg/apicompat/responses_namespace_test.go internal/service/openai_gateway_passthrough.go internal/service/openai_oauth_passthrough_test.go`

Run: `cd backend && go test ./internal/pkg/apicompat ./internal/service -count=1`

Expected: PASS with zero failures.

- [ ] **Step 6: Review the final diff and commit**

Run: `git diff --check && git status -sb && git diff --stat origin/main...HEAD`

Expected: no whitespace errors and only the design, plan, namespace adapter, passthrough integration, and regression tests in scope.

Commit with: `git commit -m "fix: support namespace tools in native responses"`.
