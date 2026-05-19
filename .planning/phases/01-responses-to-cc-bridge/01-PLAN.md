---
phase: 01
plan: 01
title: "Responses → Chat Completions Bridge"
requirements: []
dependencies: []
estimated_tasks: 3
verification:
  - "cd /Users/wille/projects/sub2api/backend && go build ./..."
  - "cd /Users/wille/projects/sub2api/backend && go test ./internal/pkg/apicompat/... -run TestResponsesToCC -v"
  - "curl -s -X POST http://localhost:8080/v1/responses -H 'Authorization: Bearer sk-cli-switch-codex-v1' -H 'Content-Type: application/json' -d '{\"model\":\"gpt-4o\",\"input\":\"say hello\",\"max_output_tokens\":50}' | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get(\"status\",\"ERROR\"), d.get(\"output\",[])[0] if d.get(\"output\") else d.get(\"error\",{}))'"
---

# Plan 01: Responses → Chat Completions Bridge

## Background

Sub2API 的 `/v1/responses` 端点只支持 Anthropic 上游（Responses → Anthropic Messages 转换）。
Codex CLI 只用 Responses API，所以通过 Sub2API 调用 OpenAI 兼容上游（MiMo/GLM/DeepSeek 等）会失败。

Chat Completions 端点 (`/v1/chat/completions`) 已有完整的分流逻辑：
- `ShouldUseResponsesAPI(extra) == true` → CC → Responses → Anthropic
- `ShouldUseResponsesAPI(extra) == false` → `forwardAsRawChatCompletions` 直转

Responses 端点缺少反向分流：对不支持 Responses API 的 OpenAI 上游，应走 Responses 请求 → CC 请求直转。

## Task 01: 实现 `ResponsesToChatCompletionsRequest` 转换函数

**Type:** auto  
**Read first:** `internal/pkg/apicompat/types.go`, `internal/pkg/apicompat/chatcompletions_to_responses.go`  
**Acceptance criteria:**
1. 新文件 `internal/pkg/apicompat/responses_to_chatcompletions_request.go`
2. 函数 `ResponsesToChatCompletionsRequest(req *ResponsesRequest) (*ChatCompletionsRequest, error)`
3. 转换规则：
   - `model` → `model`
   - `instructions` → system message (role=system, content=instructions)
   - `input` → `messages[]`:
     - string → user message
     - []ResponsesInputItem → 按 role 拆分为 ChatMessage[]:
       - role=developer → system message
       - role=user → user message
       - role=assistant → assistant message (含 output_text content)
       - type=function_call → assistant message with tool_calls
       - type=function_call_output → tool message with tool_call_id
   - `max_output_tokens` → `max_completion_tokens`
   - `temperature`, `top_p` → 透传
   - `tools[]` → `tools[]` (function type only, ResponsesTool → ChatTool 映射)
   - `tool_choice` → 透传
   - `stream` → `stream`
   - `reasoning.effort` → `reasoning_effort`
4. 测试文件 `internal/pkg/apicompat/responses_to_chatcompletions_request_test.go`
5. `go build ./...` 通过

## Task 02: 实现 `forwardAsRawResponses` 直转函数

**Type:** auto  
**Read first:** `internal/service/openai_gateway_chat_completions_raw.go` (CC 直转参考实现), `internal/service/gateway_forward_as_responses.go` (现有 Responses 转发)  
**Acceptance criteria:**
1. 新文件 `internal/service/gateway_forward_as_responses_raw.go`
2. 函数 `forwardAsRawResponses(ctx, c, account, body, defaultMappedModel) (*ForwardResult, error)`
3. 流程：
   - 解析 ResponsesRequest
   - 调用 `ResponsesToChatCompletionsRequest` 转换为 CC 请求
   - Model mapping (account.GetMappedModel)
   - 序列化 CC 请求体
   - 发送到上游 `{base_url}/v1/chat/completions`
   - 上游响应转回 Responses 格式：
     - 非流式：CC response → Responses response (用已有的 `ChatCompletionsToResponses` 逆向逻辑)
     - 流式：CC SSE chunks → Responses SSE events (用已有的 `ResponsesEventToChatState` 和 `ChatChunkToSSE`)
4. 参照 `forwardAsRawChatCompletions` 的 header 白名单和错误处理模式
5. `go build ./...` 通过

## Task 03: 在 Responses handler 添加 OpenAI 上游分流

**Type:** auto  
**Read first:** `internal/handler/gateway_handler_responses.go` (第 227 行 ForwardAsResponses 调用), `internal/service/openai_gateway_chat_completions.go` (ForwardAsChatCompletions 顶部分流逻辑)  
**Acceptance criteria:**
1. 修改 `gateway_handler_responses.go` 第 227 行附近
2. 判断条件：`account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(account.ExtraAsMap())`
3. 条件满足时调用 `forwardAsRawResponses`，否则走原有 `ForwardAsResponses`
4. 注意：`forwardAsRawResponses` 在 `GatewayService` 上（与 `ForwardAsResponses` 同层），不在 `OpenAIGatewayService` 上
5. `go build ./...` 通过
6. 端到端验证：`curl -X POST http://localhost:8080/v1/responses` 用 MiMo 账号能正常返回
