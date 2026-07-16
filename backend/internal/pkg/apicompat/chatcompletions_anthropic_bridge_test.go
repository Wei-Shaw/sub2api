package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// collectAnthropicStreamEvents feeds CC chunks through the direct bridge and
// appends finalize events, returning the full Anthropic event sequence.
func collectAnthropicStreamEvents(t *testing.T, chunks []string) []AnthropicStreamEvent {
	t.Helper()
	state := NewChatCompletionsToAnthropicStreamState("deepseek-v4-pro")
	var events []AnthropicStreamEvent
	for _, payload := range chunks {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
		events = append(events, ChatCompletionsChunkToAnthropicEvents(&chunk, state)...)
	}
	events = append(events, FinalizeChatCompletionsAnthropicStream(state)...)
	return events
}

// anthropicEventTypes extracts the sequence of event types for concise assertions.
func anthropicEventTypes(events []AnthropicStreamEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Type)
	}
	return out
}

// ---------------------------------------------------------------------------
// Request: AnthropicToChatCompletionsRequest
// ---------------------------------------------------------------------------

func TestAnthropicToChatCompletionsRequest_BasicText(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Equal(t, "claude-sonnet-4-20250514", out.Model)
	require.Len(t, out.Messages, 1)
	require.Equal(t, "user", out.Messages[0].Role)
	require.Equal(t, `"hello"`, string(out.Messages[0].Content))
	require.NotNil(t, out.MaxCompletionTokens)
	require.Equal(t, 1024, *out.MaxCompletionTokens)
}

func TestAnthropicToChatCompletionsRequest_SystemPrompt(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		System:    json.RawMessage(`"You are helpful"`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 2)
	require.Equal(t, "system", out.Messages[0].Role)
	require.Equal(t, `"You are helpful"`, string(out.Messages[0].Content))
}

func TestAnthropicToChatCompletionsRequest_ToolUseInAssistant(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"check weather"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny"}]`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	// user + assistant(with tool_calls) + tool reply
	require.GreaterOrEqual(t, len(out.Messages), 2)
	// Find the assistant message with tool_calls
	var assistant *ChatMessage
	for i := range out.Messages {
		if out.Messages[i].Role == "assistant" && len(out.Messages[i].ToolCalls) > 0 {
			assistant = &out.Messages[i]
		}
	}
	require.NotNil(t, assistant, "assistant message with tool_calls should survive normalization")
	require.Len(t, assistant.ToolCalls, 1)
	require.Equal(t, "toolu_1", assistant.ToolCalls[0].ID)
	require.Equal(t, "function", assistant.ToolCalls[0].Type)
	require.Equal(t, "get_weather", assistant.ToolCalls[0].Function.Name)
	require.Equal(t, `{"city":"SF"}`, assistant.ToolCalls[0].Function.Arguments)
}

func TestAnthropicToChatCompletionsRequest_ToolResultBecomesToolMessage(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"check weather"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny, 72F"}]`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	// Find the tool reply message
	var toolMsg *ChatMessage
	for i := range out.Messages {
		if out.Messages[i].Role == "tool" {
			toolMsg = &out.Messages[i]
		}
	}
	require.NotNil(t, toolMsg, "tool_result should become a tool role message")
	require.Equal(t, "toolu_1", toolMsg.ToolCallID)
	require.Equal(t, `"sunny, 72F"`, string(toolMsg.Content))
}

func TestAnthropicToChatCompletionsRequest_ThinkingDropped(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"secret thoughts"},{"type":"text","text":"answer"}]`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)
	// Only text survives; thinking is dropped
	require.Equal(t, `"answer"`, string(out.Messages[0].Content))
}

func TestAnthropicToChatCompletionsRequest_ToolChoiceAuto(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Tools: []AnthropicTool{
			{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		ToolChoice: json.RawMessage(`{"type":"auto"}`),
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 1)
	require.Equal(t, `"auto"`, string(out.ToolChoice))
}

func TestAnthropicToChatCompletionsRequest_ToolChoiceAny(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Tools: []AnthropicTool{
			{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		ToolChoice: json.RawMessage(`{"type":"any"}`),
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Equal(t, `"required"`, string(out.ToolChoice))
}

func TestAnthropicToChatCompletionsRequest_ToolChoiceSpecificTool(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Tools: []AnthropicTool{
			{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		ToolChoice: json.RawMessage(`{"type":"tool","name":"get_weather"}`),
		Messages:   []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	var tc map[string]any
	require.NoError(t, json.Unmarshal(out.ToolChoice, &tc))
	require.Equal(t, "function", tc["type"])
	fn, ok := tc["function"].(map[string]any)
	require.True(t, ok, "tool_choice function should be a map")
	require.Equal(t, "get_weather", fn["name"])
}

func TestAnthropicToChatCompletionsRequest_TemperatureStrippedForReasoningModel(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &AnthropicRequest{
		Model:       "gpt-5.4",
		MaxTokens:   100,
		Temperature: &temp,
		TopP:        &topP,
		Messages:    []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Nil(t, out.Temperature, "temperature should be stripped for reasoning models")
	require.Nil(t, out.TopP, "top_p should be stripped for reasoning models")
}

func TestAnthropicToChatCompletionsRequest_TemperaturePreservedForNonReasoningModel(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &AnthropicRequest{
		Model:       "deepseek-v4-pro",
		MaxTokens:   100,
		Temperature: &temp,
		TopP:        &topP,
		Messages:    []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out.Temperature)
	require.Equal(t, 0.7, *out.Temperature)
	require.NotNil(t, out.TopP)
	require.Equal(t, 0.9, *out.TopP)
}

func TestAnthropicToChatCompletionsRequest_MaxTokensFloor(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 10, // below minMaxOutputTokens (128)
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out.MaxCompletionTokens)
	require.Equal(t, minMaxOutputTokens, *out.MaxCompletionTokens)
}

func TestAnthropicToChatCompletionsRequest_ReasoningEffortMapping(t *testing.T) {
	req := &AnthropicRequest{
		Model:        "gpt-5.4",
		MaxTokens:    100,
		OutputConfig: &AnthropicOutputConfig{Effort: "max"},
		Messages:     []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Equal(t, "xhigh", out.ReasoningEffort)
}

func TestAnthropicToChatCompletionsRequest_ReasoningEffortDefaultMedium(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.4",
		MaxTokens: 100,
		Messages:  []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Equal(t, "medium", out.ReasoningEffort)
}

func TestAnthropicToChatCompletionsRequest_ServerToolDropped(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Tools: []AnthropicTool{
			{Type: "web_search_20250305", Name: "web_search"},
			{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		Messages: []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 1, "web_search server tool should be dropped")
	require.Equal(t, "get_weather", out.Tools[0].Function.Name)
}

// ---------------------------------------------------------------------------
// Non-streaming response: ChatCompletionsResponseToAnthropic
// ---------------------------------------------------------------------------

func TestChatCompletionsResponseToAnthropic_TextOnly(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-1",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"hello world"`)},
			FinishReason: "stop",
		}},
		Usage: &ChatUsage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	require.Equal(t, "chatcmpl-1", out.ID)
	require.Equal(t, "claude-sonnet-4-20250514", out.Model)
	require.Len(t, out.Content, 1)
	require.Equal(t, "text", out.Content[0].Type)
	require.Equal(t, "hello world", out.Content[0].Text)
	require.Equal(t, "end_turn", out.StopReason)
	require.Equal(t, 5, out.Usage.InputTokens)
	require.Equal(t, 2, out.Usage.OutputTokens)
}

func TestChatCompletionsResponseToAnthropic_ToolUse(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-2",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role: "assistant",
				ToolCalls: []ChatToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: ChatFunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"SF"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	require.Len(t, out.Content, 1)
	require.Equal(t, "tool_use", out.Content[0].Type)
	require.Equal(t, "call_1", out.Content[0].ID)
	require.Equal(t, "get_weather", out.Content[0].Name)
	require.Equal(t, `{"city":"SF"}`, string(out.Content[0].Input))
	require.Equal(t, "tool_use", out.StopReason)
}

func TestChatCompletionsResponseToAnthropic_ReasoningOnlyFallback(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-3",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:             "assistant",
				ReasoningContent: "I should think about this",
			},
			FinishReason: "stop",
		}},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	// thinking block + text block (fallback uses reasoning as visible text)
	require.Len(t, out.Content, 2)
	require.Equal(t, "thinking", out.Content[0].Type)
	require.Equal(t, "I should think about this", out.Content[0].Thinking)
	require.Equal(t, "text", out.Content[1].Type)
	require.Equal(t, "I should think about this", out.Content[1].Text)
}

func TestChatCompletionsResponseToAnthropic_FinishReasonLength(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-4",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"truncated"`)},
			FinishReason: "length",
		}},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	require.Equal(t, "max_tokens", out.StopReason)
}

func TestChatCompletionsResponseToAnthropic_EmptyChoices(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:      "chatcmpl-5",
		Model:   "deepseek-v4-pro",
		Choices: []ChatChoice{},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	require.Len(t, out.Content, 1)
	require.Equal(t, "text", out.Content[0].Type)
	require.Equal(t, "", out.Content[0].Text)
	require.Equal(t, "end_turn", out.StopReason, "empty choices must not produce an empty stop_reason")
}

func TestChatCompletionsResponseToAnthropic_CacheTokens(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-6",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			FinishReason: "stop",
		}},
		Usage: &ChatUsage{
			PromptTokens:     100,
			CompletionTokens: 5,
			TotalTokens:      105,
			PromptTokensDetails: &ChatTokenDetails{
				CachedTokens:        30,
				CacheCreationTokens: 10,
			},
		},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	// input = prompt(100) - cached(30) - cacheCreation(10) = 60
	require.Equal(t, 60, out.Usage.InputTokens)
	require.Equal(t, 5, out.Usage.OutputTokens)
	require.Equal(t, 30, out.Usage.CacheReadInputTokens)
	require.Equal(t, 10, out.Usage.CacheCreationInputTokens)
}

func TestChatCompletionsResponseToAnthropic_NilResponse(t *testing.T) {
	out := ChatCompletionsResponseToAnthropic(nil, "claude-sonnet-4-20250514")
	require.Len(t, out.Content, 1)
	require.Equal(t, "text", out.Content[0].Type)
	require.Equal(t, "end_turn", out.StopReason, "nil response must not produce an empty stop_reason")
	require.NotEmpty(t, out.ID)
}

// ---------------------------------------------------------------------------
// Streaming: ChatCompletionsChunkToAnthropicEvents
// ---------------------------------------------------------------------------

func TestChatCompletionsChunkToAnthropicEvents_TextOnly(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
	})

	types := anthropicEventTypes(events)
	// message_start → content_block_start(text) → 2× content_block_delta → content_block_stop → message_delta → message_stop
	require.Equal(t, []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}, types)

	// Verify deltas
	var texts []string
	for _, e := range events {
		if e.Type == "content_block_delta" && e.Delta != nil {
			texts = append(texts, e.Delta.Text)
		}
	}
	require.Equal(t, []string{"hello", " world"}, texts)

	// Verify stop reason
	for _, e := range events {
		if e.Type == "message_delta" {
			require.Equal(t, "end_turn", e.Delta.StopReason)
			require.Equal(t, 5, e.Usage.InputTokens)
			require.Equal(t, 2, e.Usage.OutputTokens)
		}
	}
}

func TestChatCompletionsChunkToAnthropicEvents_ReasoningThenContent(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"reasoning_content":"thinking..."}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"answer"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	types := anthropicEventTypes(events)
	// message_start → thinking block start → thinking_delta → thinking block stop
	// → text block start → text_delta → text block stop → message_delta → message_stop
	require.Equal(t, []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}, types)

	// First content_block_start should be thinking, second text
	var blockTypes []string
	for _, e := range events {
		if e.Type == "content_block_start" && e.ContentBlock != nil {
			blockTypes = append(blockTypes, e.ContentBlock.Type)
		}
	}
	require.Equal(t, []string{"thinking", "text"}, blockTypes)
}

func TestChatCompletionsChunkToAnthropicEvents_ToolCallAggregation(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"SF\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	})

	types := anthropicEventTypes(events)
	// message_start → content_block_start(tool_use) → 2× input_json_delta (empty first arg skipped) → content_block_stop → message_delta(tool_use) → message_stop
	require.Equal(t, []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}, types)

	// Verify tool_use block
	for _, e := range events {
		if e.Type == "content_block_start" && e.ContentBlock != nil {
			require.Equal(t, "tool_use", e.ContentBlock.Type)
			require.Equal(t, "call_1", e.ContentBlock.ID)
			require.Equal(t, "get_weather", e.ContentBlock.Name)
		}
		if e.Type == "message_delta" {
			require.Equal(t, "tool_use", e.Delta.StopReason)
		}
	}

	// Verify arguments assembled (empty first fragment skipped)
	var partials []string
	for _, e := range events {
		if e.Type == "content_block_delta" && e.Delta != nil {
			partials = append(partials, e.Delta.PartialJSON)
		}
	}
	require.Equal(t, []string{`{"city":`, `"SF"}`}, partials)
}

func TestChatCompletionsChunkToAnthropicEvents_LengthMapsToMaxTokens(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	for _, e := range events {
		if e.Type == "message_delta" {
			require.Equal(t, "max_tokens", e.Delta.StopReason)
		}
	}
}

func TestChatCompletionsChunkToAnthropicEvents_EmptyStream(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
	})

	types := anthropicEventTypes(events)
	// Even with no content, message_start + message_delta + message_stop should fire.
	require.Contains(t, types, "message_start")
	require.Contains(t, types, "message_stop")
}

func TestChatCompletionsChunkToAnthropicEvents_MessageStartEmittedOnce(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"content":"a"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"b"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	count := 0
	for _, e := range events {
		if e.Type == "message_start" {
			count++
		}
	}
	require.Equal(t, 1, count, "message_start should only be emitted once")
}

func TestChatCompletionsChunkToAnthropicEvents_ParallelToolCalls(t *testing.T) {
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"tool_a","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"tool_b","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	// Two tool_use blocks should be opened
	var toolBlocks []string
	for _, e := range events {
		if e.Type == "content_block_start" && e.ContentBlock != nil && e.ContentBlock.Type == "tool_use" {
			toolBlocks = append(toolBlocks, e.ContentBlock.Name)
		}
	}
	require.Equal(t, []string{"tool_a", "tool_b"}, toolBlocks)

	for _, e := range events {
		if e.Type == "message_delta" {
			require.Equal(t, "tool_use", e.Delta.StopReason)
		}
	}
}

func TestFinalizeChatCompletionsAnthropicStream_NoOpAfterStop(t *testing.T) {
	state := NewChatCompletionsToAnthropicStreamState("test")
	state.MessageStopSent = true

	events := FinalizeChatCompletionsAnthropicStream(state)
	require.Nil(t, events, "finalize should be a no-op after message_stop")
}

func TestFinalizeChatCompletionsAnthropicStream_EmitsMessageStartIfMissing(t *testing.T) {
	state := NewChatCompletionsToAnthropicStreamState("test")
	// Never fed any chunks — message_start not yet sent

	events := FinalizeChatCompletionsAnthropicStream(state)
	types := anthropicEventTypes(events)
	require.Contains(t, types, "message_start")
	require.Contains(t, types, "message_stop")
}

// ---------------------------------------------------------------------------
// Equivalence: direct bridge matches the double-conversion bridge
// ---------------------------------------------------------------------------

// TestDirectBridge_NonStreamingMatchesDoubleConversion verifies that
// ChatCompletionsResponseToAnthropic produces the same Anthropic response as the
// existing ChatCompletionsResponseToResponses + ResponsesToAnthropic chain.
func TestDirectBridge_NonStreamingMatchesDoubleConversion(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-eq",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:             "assistant",
				Content:          json.RawMessage(`"hello"`),
				ReasoningContent: "reasoning text",
				ToolCalls: []ChatToolCall{{
					ID:   "call_eq",
					Type: "function",
					Function: ChatFunctionCall{
						Name:      "search",
						Arguments: `{"q":"test"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: &ChatUsage{
			PromptTokens:        50,
			CompletionTokens:    10,
			TotalTokens:         60,
			PromptTokensDetails: &ChatTokenDetails{CachedTokens: 5},
		},
	}

	// Direct bridge
	direct := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")

	// Double-conversion bridge
	responsesResp := ChatCompletionsResponseToResponses(resp, "claude-sonnet-4-20250514", nil, false, nil)
	double := ResponsesToAnthropic(responsesResp, "claude-sonnet-4-20250514")

	// Compare key fields
	require.Equal(t, direct.StopReason, double.StopReason)
	require.Equal(t, direct.Model, double.Model)
	require.Len(t, direct.Content, len(double.Content))
	for i := range direct.Content {
		require.Equal(t, double.Content[i].Type, direct.Content[i].Type, "block %d type mismatch", i)
		require.Equal(t, double.Content[i].Text, direct.Content[i].Text, "block %d text mismatch", i)
		require.Equal(t, double.Content[i].Thinking, direct.Content[i].Thinking, "block %d thinking mismatch", i)
		require.Equal(t, double.Content[i].Name, direct.Content[i].Name, "block %d name mismatch", i)
		require.Equal(t, double.Content[i].ID, direct.Content[i].ID, "block %d id mismatch", i)
		require.Equal(t, string(double.Content[i].Input), string(direct.Content[i].Input), "block %d input mismatch", i)
	}
	require.Equal(t, double.Usage.InputTokens, direct.Usage.InputTokens)
	require.Equal(t, double.Usage.OutputTokens, direct.Usage.OutputTokens)
	require.Equal(t, double.Usage.CacheReadInputTokens, direct.Usage.CacheReadInputTokens)
	require.Equal(t, double.Usage.CacheCreationInputTokens, direct.Usage.CacheCreationInputTokens)
}

// TestDirectBridge_RequestMatchesDoubleConversion verifies that
// AnthropicToChatCompletionsRequest produces an equivalent Chat Completions
// request as the AnthropicToResponses + ResponsesToChatCompletionsRequest chain.
func TestDirectBridge_RequestMatchesDoubleConversion(t *testing.T) {
	temp := 0.5
	req := &AnthropicRequest{
		Model:       "deepseek-v4-pro",
		MaxTokens:   500,
		Temperature: &temp,
		System:      json.RawMessage(`"be helpful"`),
		Tools: []AnthropicTool{
			{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)},
		},
		ToolChoice: json.RawMessage(`{"type":"auto"}`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"what's the weather?"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny"}]`)},
		},
	}

	// Direct bridge
	direct, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)

	// Double-conversion bridge
	responsesReq, err := AnthropicToResponses(req)
	require.NoError(t, err)
	double, err := ResponsesToChatCompletionsRequest(responsesReq)
	require.NoError(t, err)

	// Compare key fields
	require.Equal(t, double.Model, direct.Model)
	require.Equal(t, double.Temperature, direct.Temperature)
	require.Equal(t, double.MaxCompletionTokens, direct.MaxCompletionTokens)
	require.Equal(t, double.ReasoningEffort, direct.ReasoningEffort)
	require.Equal(t, string(double.ToolChoice), string(direct.ToolChoice))
	require.Len(t, direct.Tools, len(double.Tools))

	// Compare messages — same count, same roles, same content
	require.Len(t, direct.Messages, len(double.Messages), "message count mismatch")
	for i := range direct.Messages {
		require.Equal(t, double.Messages[i].Role, direct.Messages[i].Role, "msg %d role mismatch", i)
		// Normalize content for comparison (both should be valid JSON)
		var dContent, dblContent any
		_ = json.Unmarshal(double.Messages[i].Content, &dblContent)
		_ = json.Unmarshal(direct.Messages[i].Content, &dContent)
		require.Equal(t, dblContent, dContent, "msg %d content mismatch", i)
		require.Equal(t, double.Messages[i].ToolCallID, direct.Messages[i].ToolCallID, "msg %d tool_call_id mismatch", i)
		require.Len(t, direct.Messages[i].ToolCalls, len(double.Messages[i].ToolCalls), "msg %d tool_calls count mismatch", i)
		for j := range direct.Messages[i].ToolCalls {
			require.Equal(t, double.Messages[i].ToolCalls[j].ID, direct.Messages[i].ToolCalls[j].ID, "msg %d tool %d id mismatch", i, j)
			require.Equal(t, double.Messages[i].ToolCalls[j].Function.Name, direct.Messages[i].ToolCalls[j].Function.Name, "msg %d tool %d name mismatch", i, j)
			require.Equal(t, double.Messages[i].ToolCalls[j].Function.Arguments, direct.Messages[i].ToolCalls[j].Function.Arguments, "msg %d tool %d arguments mismatch", i, j)
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestChatCompletionsChunkToAnthropicEvents_ImageInToolResult(t *testing.T) {
	// A multi-turn conversation: assistant calls a tool, user replies with a
	// tool_result containing text + an image. The image should be lifted into
	// a follow-up user message as an image_url part.
	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"check this image"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"let me look"},{"type":"tool_use","id":"toolu_1","name":"analyze","input":{"x":1}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"result"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]}]`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	// user + assistant(tool_use) + tool + user(image)
	require.GreaterOrEqual(t, len(out.Messages), 3)

	// Find the user message with image content
	var foundImage bool
	for _, m := range out.Messages {
		if m.Role != "user" {
			continue
		}
		var parts []ChatContentPart
		if err := json.Unmarshal(m.Content, &parts); err == nil {
			for _, p := range parts {
				if p.Type == "image_url" && p.ImageURL != nil {
					foundImage = true
					require.True(t, strings.HasPrefix(p.ImageURL.URL, "data:image/png;base64,"))
				}
			}
		}
	}
	require.True(t, foundImage, "image from tool_result should appear in user message")
}

func TestChatCompletionsToAnthropicStreamState_ToolCallNameArrivesLate(t *testing.T) {
	// Some upstreams send the tool_call index + arguments before the name.
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_late"}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"late_tool"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	})

	// The tool_use block should still be opened with the correct name.
	var toolName string
	for _, e := range events {
		if e.Type == "content_block_start" && e.ContentBlock != nil && e.ContentBlock.Type == "tool_use" {
			toolName = e.ContentBlock.Name
		}
	}
	require.Equal(t, "late_tool", toolName)
}

// assembleToolUseBlocks rebuilds tool_use blocks from a stream the way an
// Anthropic client does: content_block_start announces id/name, input_json_delta
// fragments concatenate into the input JSON.
type assembledToolUse struct {
	ID    string
	Name  string
	Input string
}

func assembleToolUseBlocks(events []AnthropicStreamEvent) []assembledToolUse {
	blockByIdx := map[int]int{} // anthropic block index → position in out
	var out []assembledToolUse
	for _, e := range events {
		switch e.Type {
		case "content_block_start":
			if e.ContentBlock != nil && e.ContentBlock.Type == "tool_use" && e.Index != nil {
				blockByIdx[*e.Index] = len(out)
				out = append(out, assembledToolUse{ID: e.ContentBlock.ID, Name: e.ContentBlock.Name})
			}
		case "content_block_delta":
			if e.Delta != nil && e.Delta.Type == "input_json_delta" && e.Index != nil {
				if pos, ok := blockByIdx[*e.Index]; ok {
					out[pos].Input += e.Delta.PartialJSON
				}
			}
		}
	}
	return out
}

func TestChatCompletionsToAnthropicStreamState_ToolCallArgsArriveBeforeName(t *testing.T) {
	// Some upstreams stream argument fragments before the tool name. The
	// fragments buffered while the announcement is deferred must be flushed
	// with the content_block_start, so the client rebuilds complete JSON.
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_early","function":{"arguments":"{\"city\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"SF\""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"get_weather","arguments":"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	})

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 1)
	require.Equal(t, "call_early", tools[0].ID)
	require.Equal(t, "get_weather", tools[0].Name)
	require.JSONEq(t, `{"city":"SF"}`, tools[0].Input)

	// No delta may precede the block's content_block_start.
	started := map[int]bool{}
	for _, e := range events {
		switch e.Type {
		case "content_block_start":
			started[*e.Index] = true
		case "content_block_delta":
			require.True(t, started[*e.Index], "delta before content_block_start on index %d", *e.Index)
		}
	}
}

func TestChatCompletionsToAnthropicStreamState_ToolCallNameNeverArrives(t *testing.T) {
	// If the name never arrives, the tool is announced at finalize with an
	// empty name (like the double-conversion path) so its arguments are not
	// silently dropped — stop_reason still reports tool_use.
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_anon","function":{"arguments":"{\"a\":1}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	})

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 1)
	require.Equal(t, "call_anon", tools[0].ID)
	require.Equal(t, "", tools[0].Name)
	require.JSONEq(t, `{"a":1}`, tools[0].Input)

	// Block lifecycle must stay balanced and terminate before message_stop.
	types := anthropicEventTypes(events)
	require.Equal(t, []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}, types)
}

func TestChatCompletionsToAnthropicStreamState_EmptyArgsToolEmitsPlaceholderDelta(t *testing.T) {
	// A tool call whose arguments never arrive gets a final input_json_delta
	// "{}" before its stop — the double-conversion path normalizes empty
	// arguments to "{}", and some clients assemble input only from deltas.
	events := collectAnthropicStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_empty","type":"function","function":{"name":"noop","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	})

	tools := assembleToolUseBlocks(events)
	require.Len(t, tools, 1)
	require.Equal(t, "noop", tools[0].Name)
	require.JSONEq(t, `{}`, tools[0].Input)
}

func TestAnthropicToChatCompletionsRequest_UserArrayContentFoldsToString(t *testing.T) {
	// Text-only array content folds into a single string joined with "\n\n",
	// like the double-conversion path — strict chat upstreams reject array
	// content when no image forces the parts form.
	req := &AnthropicRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`)},
		},
	}

	out, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)
	require.Equal(t, `"first\n\nsecond"`, string(out.Messages[0].Content))
}

func TestDirectBridge_RequestMatchesDoubleConversion_ArrayUserContent(t *testing.T) {
	// Array-form user content: text-only folds to a string, image-bearing stays
	// in parts form — both must match the double-conversion chain exactly.
	req := &AnthropicRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`)},
			{Role: "assistant", Content: json.RawMessage(`"ok"`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"look"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}]`)},
		},
	}

	direct, err := AnthropicToChatCompletionsRequest(req)
	require.NoError(t, err)

	responsesReq, err := AnthropicToResponses(req)
	require.NoError(t, err)
	double, err := ResponsesToChatCompletionsRequest(responsesReq)
	require.NoError(t, err)

	require.Len(t, direct.Messages, len(double.Messages), "message count mismatch")
	for i := range direct.Messages {
		require.Equal(t, double.Messages[i].Role, direct.Messages[i].Role, "msg %d role mismatch", i)
		var dContent, dblContent any
		require.NoError(t, json.Unmarshal(double.Messages[i].Content, &dblContent))
		require.NoError(t, json.Unmarshal(direct.Messages[i].Content, &dContent))
		require.Equal(t, dblContent, dContent, "msg %d content mismatch", i)
	}
}

func TestDirectBridge_NonStreamingMatchesDoubleConversion_CacheWriteTokens(t *testing.T) {
	// cache_write_tokens and cache_creation_tokens are alternate spellings, not
	// additive — when both are set, the double-conversion path prefers write.
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-cache",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			FinishReason: "stop",
		}},
		Usage: &ChatUsage{
			PromptTokens:     100,
			CompletionTokens: 10,
			TotalTokens:      110,
			PromptTokensDetails: &ChatTokenDetails{
				CachedTokens:        20,
				CacheCreationTokens: 7,
				CacheWriteTokens:    9,
			},
		},
	}

	direct := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")

	responsesResp := ChatCompletionsResponseToResponses(resp, "claude-sonnet-4-20250514", nil, false, nil)
	double := ResponsesToAnthropic(responsesResp, "claude-sonnet-4-20250514")

	require.Equal(t, double.Usage.InputTokens, direct.Usage.InputTokens)
	require.Equal(t, double.Usage.OutputTokens, direct.Usage.OutputTokens)
	require.Equal(t, double.Usage.CacheReadInputTokens, direct.Usage.CacheReadInputTokens)
	require.Equal(t, double.Usage.CacheCreationInputTokens, direct.Usage.CacheCreationInputTokens)
	require.Equal(t, 9, direct.Usage.CacheCreationInputTokens)
}

func TestChatCompletionsResponseToAnthropic_GeneratesIDWhenMissing(t *testing.T) {
	resp := &ChatCompletionsResponse{
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			FinishReason: "stop",
		}},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	require.NotEmpty(t, out.ID, "response id must be generated when the upstream omits one")
}

func TestAnthropicToChatCompletionsRequest_ToolChoiceUndeclaredDropped(t *testing.T) {
	// A named tool_choice pointing at a dropped/unknown tool is not forwarded —
	// chat upstreams 400 on tool_choice referencing an undeclared tool.
	base := AnthropicRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 100,
		Tools: []AnthropicTool{
			{Name: "get_weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "web_search_20250305", Type: "web_search_20250305", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}

	undeclared := base
	undeclared.ToolChoice = json.RawMessage(`{"type":"tool","name":"nonexistent"}`)
	out, err := AnthropicToChatCompletionsRequest(&undeclared)
	require.NoError(t, err)
	require.Empty(t, out.ToolChoice, "tool_choice for an undeclared tool must be dropped")

	droppedServerTool := base
	droppedServerTool.ToolChoice = json.RawMessage(`{"type":"tool","name":"web_search_20250305"}`)
	out, err = AnthropicToChatCompletionsRequest(&droppedServerTool)
	require.NoError(t, err)
	require.Empty(t, out.ToolChoice, "tool_choice for a dropped server tool must be dropped")

	unknownType := base
	unknownType.ToolChoice = json.RawMessage(`{"type":"mystery"}`)
	out, err = AnthropicToChatCompletionsRequest(&unknownType)
	require.NoError(t, err)
	require.Empty(t, out.ToolChoice, "unknown tool_choice types must be dropped")

	declared := base
	declared.ToolChoice = json.RawMessage(`{"type":"tool","name":"get_weather"}`)
	out, err = AnthropicToChatCompletionsRequest(&declared)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"function","function":{"name":"get_weather"}}`, string(out.ToolChoice))
}

func TestDirectBridge_NonStreamingMatchesDoubleConversion_EmptyChoices(t *testing.T) {
	// An upstream 200 with empty choices must still report a valid stop_reason,
	// matching the double-conversion chain ("end_turn").
	resp := &ChatCompletionsResponse{ID: "chatcmpl-empty", Model: "deepseek-v4-pro"}

	direct := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")

	responsesResp := ChatCompletionsResponseToResponses(resp, "claude-sonnet-4-20250514", nil, false, nil)
	double := ResponsesToAnthropic(responsesResp, "claude-sonnet-4-20250514")

	require.Equal(t, double.StopReason, direct.StopReason)
	require.Equal(t, "end_turn", direct.StopReason)
}

func TestChatCompletionsResponseToAnthropic_ContentFilterWithToolUse(t *testing.T) {
	// content_filter (and unknown finish reasons) derive stop_reason from the
	// blocks, like the double-conversion path.
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-cf",
		Model: "deepseek-v4-pro",
		Choices: []ChatChoice{{
			Message: ChatMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"partial"`),
				ToolCalls: []ChatToolCall{{
					ID:       "call_cf",
					Type:     "function",
					Function: ChatFunctionCall{Name: "search", Arguments: `{"q":"x"}`},
				}},
			},
			FinishReason: "content_filter",
		}},
	}

	out := ChatCompletionsResponseToAnthropic(resp, "claude-sonnet-4-20250514")
	require.Equal(t, "tool_use", out.StopReason)
}
