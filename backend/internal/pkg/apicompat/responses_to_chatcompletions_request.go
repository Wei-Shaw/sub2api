package apicompat

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// ConvertResponsesOptions controls optional behaviour during Responses→ChatCompletions conversion.
type ConvertResponsesOptions struct {
	// StripReasoningEffort omits reasoning_effort from the Chat Completions output.
	// Enable for upstreams that reject reasoning_effort + tools together (e.g. b.ai /v1/chat/completions).
	StripReasoningEffort bool
	// PreserveInstructionsField keeps the non-standard instructions field in the
	// Chat Completions request. By default instructions are represented only as a
	// leading system message because strict Chat-Completions-only upstreams often
	// reject the Responses-only instructions field.
	PreserveInstructionsField bool
}

// ResponsesToChatCompletionsRequest converts an OpenAI Responses API request
// into a Chat Completions request. This is the reverse of
// ChatCompletionsToResponses and enables Chat-Completions-only upstreams to
// accept Responses API traffic by translating it back to the native
// /v1/chat/completions format before forwarding.
//
// Field mapping:
//   - input  (array)        → messages          (system/user/assistant/tool 角色还原)
//   - tools                 → tools
//   - tool_choice           → tool_choice
//   - temperature           → temperature
//   - top_p                 → top_p
//   - max_output_tokens     → max_tokens
//   - stream                → stream
//   - reasoning.effort      → reasoning_effort
//   - instructions          → instructions     (前置为 system 消息保留语义同时透传字段)
//
// Unsupported fields (previous_response_id / prompt_cache_key /
// service_tier-only / parallel_tool_calls / store / include / text 等) 不会丢失
// 关键语义但会以 debug 日志形式记录被忽略，便于上游排障。
func ResponsesToChatCompletionsRequest(req *ResponsesRequest) (*ChatCompletionsRequest, error) {
	return ResponsesToChatCompletionsRequestWithOptions(req, ConvertResponsesOptions{})
}

// ResponsesToChatCompletionsRequestWithOptions is like ResponsesToChatCompletionsRequest
// but accepts options to control conversion behaviour.
func ResponsesToChatCompletionsRequestWithOptions(req *ResponsesRequest, opts ConvertResponsesOptions) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("nil ResponsesRequest")
	}

	messages, err := convertResponsesInputToChatMessages(req.Input)
	if err != nil {
		return nil, fmt.Errorf("convert input: %w", err)
	}

	// instructions 在 Chat Completions 中没有等价独立字段。为保持语义完整，
	// 默认在最前置插入一条 system 消息；非标准 instructions 字段仅在显式
	// PreserveInstructionsField 时透传，避免严格 CC-only upstream 拒绝请求。
	if req.Instructions != "" {
		raw, err := json.Marshal(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("marshal instructions: %w", err)
		}
		sysMsg := ChatMessage{Role: "system", Content: raw}
		messages = append([]ChatMessage{sysMsg}, messages...)
	}

	out := &ChatCompletionsRequest{
		Model:             req.Model,
		Messages:          messages,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		Stop:              req.Stop,
		User:              req.User,
		Metadata:          req.Metadata,
		Seed:              req.Seed,
		PresencePenalty:   req.PresencePenalty,
		FrequencyPenalty:  req.FrequencyPenalty,
		Logprobs:          req.Logprobs,
		TopLogprobs:       req.TopLogprobs,
		Stream:            req.Stream,
		StreamOptions:     req.StreamOptions,
		ServiceTier:       req.ServiceTier,
		ParallelToolCalls: req.ParallelToolCalls,
	}
	if opts.PreserveInstructionsField {
		out.Instructions = req.Instructions
	}

	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		v := *req.MaxOutputTokens
		out.MaxTokens = &v
	}

	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		if opts.StripReasoningEffort {
			slog.Debug("apicompat: stripping reasoning_effort per account config")
		} else {
			out.ReasoningEffort = req.Reasoning.Effort
		}
	}

	if len(req.Tools) > 0 {
		out.Tools = convertResponsesToolsToChat(req.Tools)
	}

	if len(req.ToolChoice) > 0 {
		toolChoice, err := convertResponsesToolChoiceToChat(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
		}
		out.ToolChoice = toolChoice
	}

	logIgnoredResponsesFields(req)

	return out, nil
}

// convertResponsesInputToChatMessages 把 Responses API 的 input 还原为 Chat
// Completions 的 messages 数组。input 既可能是字符串（等价于一条 user 消息），
// 也可能是 ResponsesInputItem 数组。
func convertResponsesInputToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Plain string input → single user message.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		content, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		return []ChatMessage{{Role: "user", Content: content}}, nil
	}

	var items []ResponsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("input is neither string nor array: %w", err)
	}

	out := make([]ChatMessage, 0, len(items))
	// 多个 assistant function_call 在 Chat Completions 中需要合并到同一条
	// assistant 消息的 tool_calls 数组里；用游标跟踪最后一条 assistant 消息。
	var lastAssistantIdx int = -1

	for _, it := range items {
		switch it.Type {
		case "function_call":
			tc := ChatToolCall{
				ID:   it.CallID,
				Type: "function",
				Function: ChatFunctionCall{
					Name:      it.Name,
					Arguments: it.Arguments,
				},
			}
			if lastAssistantIdx >= 0 && out[lastAssistantIdx].Role == "assistant" {
				out[lastAssistantIdx].ToolCalls = append(out[lastAssistantIdx].ToolCalls, tc)
			} else {
				out = append(out, ChatMessage{
					Role:      "assistant",
					ToolCalls: []ChatToolCall{tc},
				})
				lastAssistantIdx = len(out) - 1
			}
		case "function_call_output":
			content, err := json.Marshal(it.Output)
			if err != nil {
				return nil, err
			}
			out = append(out, ChatMessage{
				Role:       "tool",
				ToolCallID: it.CallID,
				Content:    content,
			})
			lastAssistantIdx = -1
		case "", "message":
			role := it.Role
			if role == "" {
				role = "user"
			}
			// Responses API 把 system 角色叫 "developer"，回退兼容。
			if role == "developer" {
				role = "system"
			}
			content, err := convertResponsesContentToChat(role, it.Content)
			if err != nil {
				return nil, err
			}
			msg := ChatMessage{Role: role, Content: content}
			out = append(out, msg)
			if role == "assistant" {
				lastAssistantIdx = len(out) - 1
			} else {
				lastAssistantIdx = -1
			}
		default:
			// 未识别 type 兜底为 user 文本，避免静默丢失。
			slog.Debug("apicompat: unknown Responses input item type, fallback to user message",
				slog.String("type", it.Type))
			content, err := convertResponsesContentToChat("user", it.Content)
			if err != nil {
				return nil, err
			}
			out = append(out, ChatMessage{Role: "user", Content: content})
			lastAssistantIdx = -1
		}
	}

	return normalizeChatMessagesForToolCallPairs(out), nil
}

// normalizeChatMessagesForToolCallPairs enforces the Chat Completions invariant
// required by strict upstreams: an assistant message containing tool_calls must
// be immediately followed by tool messages for those tool_call IDs. Responses
// histories can contain pending function_call items without corresponding
// function_call_output items; forwarding those verbatim causes upstream 400s
// such as "An assistant message with 'tool_calls' must be followed by tool
// messages". Unanswered tool calls are removed from the assistant message, and
// orphan tool messages are preserved as user-visible text instead of illegal
// role=tool messages.
func normalizeChatMessagesForToolCallPairs(messages []ChatMessage) []ChatMessage {
	out := make([]ChatMessage, 0, len(messages))

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role == "tool" {
			// Only degrade to a user message when there is no call_id — a tool
			// message that carries a proper ToolCallID may appear at the start
			// of a conversation history (e.g. a lone function_call_output from
			// the Responses API) and should be forwarded as-is.
			if msg.ToolCallID == "" {
				out = append(out, orphanToolMessageAsUser(msg))
			} else {
				out = append(out, msg)
			}
			continue
		}
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			out = append(out, msg)
			continue
		}

		j := i + 1
		for j < len(messages) && messages[j].Role == "tool" {
			j++
		}
		toolMessages := messages[i+1 : j]
		matchingToolMessages := make(map[string][]ChatMessage, len(toolMessages))
		for _, toolMsg := range toolMessages {
			if toolMsg.ToolCallID == "" {
				continue
			}
			matchingToolMessages[toolMsg.ToolCallID] = append(matchingToolMessages[toolMsg.ToolCallID], toolMsg)
		}

		validIDs := make(map[string]bool, len(msg.ToolCalls))
		validCalls := make([]ChatToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			if tc.ID == "" {
				slog.Debug("apicompat: dropping assistant tool_call without id")
				continue
			}
			if len(matchingToolMessages[tc.ID]) == 0 {
				slog.Debug("apicompat: dropping unanswered assistant tool_call",
					slog.String("tool_call_id", tc.ID),
					slog.String("name", tc.Function.Name))
				continue
			}
			validIDs[tc.ID] = true
			validCalls = append(validCalls, tc)
		}

		if len(validCalls) > 0 {
			msg.ToolCalls = validCalls
			out = append(out, msg)
		} else if chatMessageHasNonEmptyContent(msg) {
			msg.ToolCalls = nil
			out = append(out, msg)
		}

		for _, toolMsg := range toolMessages {
			if validIDs[toolMsg.ToolCallID] {
				out = append(out, toolMsg)
			} else {
				out = append(out, orphanToolMessageAsUser(toolMsg))
			}
		}
		i = j - 1
	}

	return out
}

func chatMessageHasNonEmptyContent(msg ChatMessage) bool {
	if len(msg.Content) == 0 || string(msg.Content) == "null" {
		return false
	}
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return s != ""
	}
	return true
}

func orphanToolMessageAsUser(msg ChatMessage) ChatMessage {
	toolOutput := ""
	if len(msg.Content) > 0 {
		if err := json.Unmarshal(msg.Content, &toolOutput); err != nil {
			toolOutput = string(msg.Content)
		}
	}
	text := "Tool result"
	if msg.ToolCallID != "" {
		text += " for " + msg.ToolCallID
	}
	if toolOutput != "" {
		text += ": " + toolOutput
	}
	content, _ := json.Marshal(text)
	return ChatMessage{Role: "user", Content: content}
}

// convertResponsesContentToChat 把 Responses 的 message.content 还原为 Chat
// Completions 的 content。两种形式：字符串原样透传；对象数组转换为
// ChatContentPart 数组（input_text/output_text → text；input_image → image_url）。
func convertResponsesContentToChat(role string, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("parse content as string or parts array: %w", err)
	}

	// assistant 角色：仅取文本拼接为字符串，保持与 Chat Completions 习惯一致。
	if role == "assistant" {
		var b string
		for _, p := range parts {
			if p.Type == "output_text" || p.Type == "input_text" {
				b += p.Text
			}
		}
		return json.Marshal(b)
	}

	chatParts := make([]ChatContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "input_text", "output_text", "text":
			if p.Text == "" {
				continue
			}
			chatParts = append(chatParts, ChatContentPart{Type: "text", Text: p.Text})
		case "input_image":
			if p.ImageURL == "" {
				continue
			}
			chatParts = append(chatParts, ChatContentPart{
				Type:     "image_url",
				ImageURL: &ChatImageURL{URL: p.ImageURL},
			})
		default:
			slog.Debug("apicompat: unknown Responses content part type",
				slog.String("type", p.Type))
		}
	}

	// 单一 text part 折叠为字符串，以贴近 Chat Completions 的常见形态。
	if len(chatParts) == 1 && chatParts[0].Type == "text" {
		return json.Marshal(chatParts[0].Text)
	}
	return json.Marshal(chatParts)
}

// convertResponsesToolsToChat 把 Responses 的 tool 定义还原为 Chat Completions
// 形态（type=function, function={name,description,parameters,strict}）。
func convertResponsesToolsToChat(tools []ResponsesTool) []ChatTool {
	out := make([]ChatTool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "" && t.Type != "function" {
			// 内置 server-side 工具（web_search / local_shell 等）在 Chat
			// Completions 中没有对应表达，跳过并记录。
			slog.Debug("apicompat: drop non-function Responses tool",
				slog.String("type", t.Type),
				slog.String("name", t.Name))
			continue
		}
		params := t.Parameters
		if len(params) == 0 || string(params) == "null" {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
				Strict:      t.Strict,
			},
		})
	}
	return out
}

// convertResponsesToolChoiceToChat maps Responses tool_choice variants to Chat
// Completions variants. String choices are compatible. Responses object form
// {"type":"function","name":"foo"} becomes Chat's
// {"type":"function","function":{"name":"foo"}}. Already-Chat-shaped
// objects are preserved.
func convertResponsesToolChoiceToChat(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}

	var typ string
	if v := obj["type"]; len(v) > 0 {
		_ = json.Unmarshal(v, &typ)
	}
	if typ != "function" {
		// Built-in/server-side tool choices have no Chat Completions equivalent;
		// pass through so permissive upstreams can still decide what to do.
		return raw, nil
	}
	if _, ok := obj["function"]; ok {
		return raw, nil
	}
	var name string
	if v := obj["name"]; len(v) > 0 {
		_ = json.Unmarshal(v, &name)
	}
	if name == "" {
		return raw, nil
	}
	return json.Marshal(map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": name,
		},
	})
}

// logIgnoredResponsesFields 输出对未承载到 Chat Completions 的字段的 debug 日志，
// 便于上游侧排障，但不影响请求的实际效果。
func logIgnoredResponsesFields(req *ResponsesRequest) {
	if req.PreviousResponseID != "" {
		slog.Debug("apicompat: ignoring previous_response_id (no Chat Completions equivalent)",
			slog.String("previous_response_id", req.PreviousResponseID))
	}
	if req.PromptCacheKey != "" {
		slog.Debug("apicompat: ignoring prompt_cache_key",
			slog.String("prompt_cache_key", req.PromptCacheKey))
	}
	if len(req.Include) > 0 {
		slog.Debug("apicompat: ignoring Responses include[]",
			slog.Any("include", req.Include))
	}
	if req.Store != nil {
		slog.Debug("apicompat: ignoring store flag",
			slog.Bool("store", *req.Store))
	}
	if req.ParallelToolCalls != nil {
		slog.Debug("apicompat: mapping parallel_tool_calls to Chat Completions",
			slog.Bool("parallel_tool_calls", *req.ParallelToolCalls))
	}
	if req.Text != nil {
		slog.Debug("apicompat: ignoring text verbosity",
			slog.String("verbosity", req.Text.Verbosity))
	}
	if req.Reasoning != nil && req.Reasoning.Summary != "" {
		slog.Debug("apicompat: ignoring reasoning.summary",
			slog.String("summary", req.Reasoning.Summary))
	}
}
