package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func intPtr(i int) *int { return &i }

func anthChatTextChunk(s string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: stringPtr(s)}}}}
}

func anthChatReasoningChunk(s string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: stringPtr(s)}}}}
}

func anthChatFinishChunk(reason string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{FinishReason: stringPtr(reason)}}}
}

func anthChatConcatDelta(events []AnthropicStreamEvent, deltaType string) string {
	var b strings.Builder
	for _, e := range events {
		if e.Type != "content_block_delta" || e.Delta == nil || e.Delta.Type != deltaType {
			continue
		}
		switch deltaType {
		case "text_delta":
			_, _ = b.WriteString(e.Delta.Text)
		case "thinking_delta":
			_, _ = b.WriteString(e.Delta.Thinking)
		case "input_json_delta":
			_, _ = b.WriteString(e.Delta.PartialJSON)
		}
	}
	return b.String()
}

// anthChatInputJSONByIndex concatenates input_json_delta partials per block index.
func anthChatInputJSONByIndex(events []AnthropicStreamEvent) map[int]string {
	m := map[int]string{}
	for _, e := range events {
		if e.Type == "content_block_delta" && e.Index != nil && e.Delta != nil && e.Delta.Type == "input_json_delta" {
			m[*e.Index] += e.Delta.PartialJSON
		}
	}
	return m
}

// anthChatStartedBlocks maps each started content block index to its type.
func anthChatStartedBlocks(events []AnthropicStreamEvent) map[int]string {
	m := map[int]string{}
	for _, e := range events {
		if e.Type == "content_block_start" && e.Index != nil && e.ContentBlock != nil {
			m[*e.Index] = e.ContentBlock.Type
		}
	}
	return m
}

func anthChatMessageDelta(events []AnthropicStreamEvent) *AnthropicStreamEvent {
	for i := range events {
		if events[i].Type == "message_delta" {
			return &events[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// AnthropicToChatCompletions (request) tests
// ---------------------------------------------------------------------------

func TestAnthropicToChatCompletions_SystemAndToolResultOrdering(t *testing.T) {
	req := &AnthropicRequest{
		Model:  "claude",
		System: json.RawMessage(`"You are helpful"`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"get_weather","input":{"location":"Paris"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"sunny"},{"type":"text","text":"thanks"}]`)},
		},
	}

	out, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 5)

	assert.Equal(t, "system", out.Messages[0].Role)
	assert.JSONEq(t, `"You are helpful"`, string(out.Messages[0].Content))

	assert.Equal(t, "user", out.Messages[1].Role)
	assert.JSONEq(t, `"hi"`, string(out.Messages[1].Content))

	assert.Equal(t, "assistant", out.Messages[2].Role)
	require.Len(t, out.Messages[2].ToolCalls, 1)
	assert.Equal(t, "call_1", out.Messages[2].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", out.Messages[2].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"location":"Paris"}`, out.Messages[2].ToolCalls[0].Function.Arguments)

	// The tool reply must come before any new user text from the same turn.
	assert.Equal(t, "tool", out.Messages[3].Role)
	assert.Equal(t, "call_1", out.Messages[3].ToolCallID)
	assert.JSONEq(t, `"sunny"`, string(out.Messages[3].Content))

	assert.Equal(t, "user", out.Messages[4].Role)
	assert.JSONEq(t, `"thanks"`, string(out.Messages[4].Content))
}

func TestAnthropicToChatCompletions_ToolsAndToolChoice(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude",
		Tools: []AnthropicTool{
			{Name: "get_weather", Description: "Get weather", InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`)},
			{Type: "web_search_20250305", Name: "web_search", InputSchema: json.RawMessage(`{}`)},
			{Type: "bash_20250124", Name: "bash"},
		},
		ToolChoice: json.RawMessage(`{"type":"any"}`),
	}

	out, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)

	require.Len(t, out.Tools, 1, "all typed built-in tools (web_search, bash) should be skipped; only the custom tool survives")
	assert.Equal(t, "function", out.Tools[0].Type)
	require.NotNil(t, out.Tools[0].Function)
	assert.Equal(t, "get_weather", out.Tools[0].Function.Name)
	require.NotNil(t, out.Tools[0].Function.Strict)
	assert.False(t, *out.Tools[0].Function.Strict)
	assert.Equal(t, `"required"`, string(out.ToolChoice))
}

func TestAnthropicToChatCompletions_ReasoningEffort(t *testing.T) {
	enabled, err := AnthropicToChatCompletions(&AnthropicRequest{Model: "c", Thinking: &AnthropicThinking{Type: "enabled"}})
	require.NoError(t, err)
	assert.Equal(t, "medium", enabled.ReasoningEffort)

	maxed, err := AnthropicToChatCompletions(&AnthropicRequest{Model: "c", OutputConfig: &AnthropicOutputConfig{Effort: "max"}})
	require.NoError(t, err)
	assert.Equal(t, "xhigh", maxed.ReasoningEffort)
}

func TestAnthropicToChatCompletions_ThinkingDisabled(t *testing.T) {
	// thinking:disabled must forward {type:"disabled"} to the upstream and drop
	// reasoning_effort — even when output_config.effort would otherwise set one —
	// so reasoning models (GLM/...) stop thinking instead of burning the token
	// budget, and strict upstreams never see a disable+effort conflict.
	out, err := AnthropicToChatCompletions(&AnthropicRequest{
		Model:        "c",
		Thinking:     &AnthropicThinking{Type: "disabled"},
		OutputConfig: &AnthropicOutputConfig{Effort: "high"},
	})
	require.NoError(t, err)
	require.NotNil(t, out.Thinking)
	assert.Equal(t, "disabled", out.Thinking.Type)
	assert.Equal(t, "", out.ReasoningEffort, "reasoning_effort must be dropped when thinking is disabled")

	// Anthropic-only budget_tokens must never leak into the chat request.
	b, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"thinking":{"type":"disabled"}`)
	assert.NotContains(t, string(b), "budget_tokens")

	// enabled keeps the existing reasoning_effort mapping and emits no thinking field.
	enabled, err := AnthropicToChatCompletions(&AnthropicRequest{Model: "c", Thinking: &AnthropicThinking{Type: "enabled"}})
	require.NoError(t, err)
	assert.Nil(t, enabled.Thinking)
	assert.Equal(t, "medium", enabled.ReasoningEffort)
}

// ---------------------------------------------------------------------------
// Streaming chat -> Anthropic SSE tests
// ---------------------------------------------------------------------------

func TestChatCompletionsChunkToAnthropicEvents_TextAndUsage(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("deepseek")
	var all []AnthropicStreamEvent
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{ID: "id1", Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: stringPtr("Hello")}}}}, st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatTextChunk(" world"), st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{FinishReason: stringPtr("stop")}},
		Usage:   &ChatUsage{PromptTokens: 100, CompletionTokens: 20, PromptTokensDetails: &ChatTokenDetails{CachedTokens: 30}},
	}, st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	require.Equal(t, "message_start", all[0].Type)
	require.NotNil(t, all[0].Message)
	assert.Equal(t, "assistant", all[0].Message.Role)
	assert.Equal(t, "text", anthChatStartedBlocks(all)[0])
	assert.Equal(t, "Hello world", anthChatConcatDelta(all, "text_delta"))
	assert.Equal(t, "message_stop", all[len(all)-1].Type)

	md := anthChatMessageDelta(all)
	require.NotNil(t, md)
	assert.Equal(t, "end_turn", md.Delta.StopReason)
	require.NotNil(t, md.Usage)
	// Anthropic input_tokens excludes cached; cached surfaces as cache_read.
	assert.Equal(t, 70, md.Usage.InputTokens)
	assert.Equal(t, 30, md.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, md.Usage.OutputTokens)
}

func TestChatCompletionsChunkToAnthropicEvents_ReasoningThenText(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	var all []AnthropicStreamEvent
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatReasoningChunk("thinking "), st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatReasoningChunk("more"), st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatTextChunk("answer"), st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	started := anthChatStartedBlocks(all)
	assert.Equal(t, "thinking", started[0])
	assert.Equal(t, "text", started[1])
	assert.Equal(t, "thinking more", anthChatConcatDelta(all, "thinking_delta"))
	assert.Equal(t, "answer", anthChatConcatDelta(all, "text_delta"))
}

// TestChatCompletionsChunkToAnthropicEvents_ParallelToolsNoOrphan is the key
// regression guard: routing chat -> responses -> anthropic produced an orphan
// input_json_delta at a phantom block index for parallel tools. The direct path
// must open every block it deltas into, and never double an argument fragment.
func TestChatCompletionsChunkToAnthropicEvents_ParallelToolsNoOrphan(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	var all []AnthropicStreamEvent
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{
		ID: "id",
		Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
			{Index: intPtr(0), ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "get_weather", Arguments: `{"location":"Paris"}`}},
			{Index: intPtr(1), ID: "call_b", Type: "function", Function: ChatFunctionCall{Name: "get_time", Arguments: `{"tz":"UTC"}`}},
		}}}},
	}, st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatFinishChunk("tool_calls"), st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	started := anthChatStartedBlocks(all)
	require.Len(t, started, 2)
	assert.Equal(t, "tool_use", started[0])
	assert.Equal(t, "tool_use", started[1])

	ij := anthChatInputJSONByIndex(all)
	require.Len(t, ij, 2)
	// Every block that receives input_json_delta must have been opened (no orphan).
	for idx := range ij {
		_, ok := started[idx]
		assert.Truef(t, ok, "input_json_delta at unopened block index %d (orphan)", idx)
	}
	// Args appear exactly once each (no doubling).
	assert.JSONEq(t, `{"location":"Paris"}`, ij[0])
	assert.JSONEq(t, `{"tz":"UTC"}`, ij[1])

	md := anthChatMessageDelta(all)
	require.NotNil(t, md)
	assert.Equal(t, "tool_use", md.Delta.StopReason)
}

func TestChatCompletionsChunkToAnthropicEvents_ReadToolBuffered(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	chunk1 := &ChatCompletionsChunk{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
		{Index: intPtr(0), ID: "call_r", Type: "function", Function: ChatFunctionCall{Name: "Read", Arguments: `{"file_path":"/x",`}},
	}}}}}
	chunk2 := &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
		{Index: intPtr(0), Function: ChatFunctionCall{Arguments: `"pages":""}`}},
	}}}}}

	e1 := ChatCompletionsChunkToAnthropicEvents(chunk1, st)
	e2 := ChatCompletionsChunkToAnthropicEvents(chunk2, st)
	streamPhase := append(append([]AnthropicStreamEvent{}, e1...), e2...)
	fin := FinalizeChatCompletionsAnthropicStream(st)
	all := append(append([]AnthropicStreamEvent{}, streamPhase...), fin...)

	// "Read" args are buffered: nothing is streamed mid-flight.
	assert.Empty(t, anthChatInputJSONByIndex(streamPhase), "Read tool args must be buffered until close")

	// At close, a single sanitized delta with pages:"" stripped.
	ij := anthChatInputJSONByIndex(all)
	require.Len(t, ij, 1)
	assert.JSONEq(t, `{"file_path":"/x"}`, ij[0])
}

func TestFinalizeChatCompletionsAnthropicStream_ReasoningOnlyFallback(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	_ = ChatCompletionsChunkToAnthropicEvents(anthChatReasoningChunk("the reasoning"), st)
	_ = ChatCompletionsChunkToAnthropicEvents(anthChatFinishChunk("stop"), st)
	fin := FinalizeChatCompletionsAnthropicStream(st)

	// Reasoning-only completion echoes the reasoning as a text block so the
	// client never receives a thinking block with no message.
	assert.Equal(t, "text", anthChatStartedBlocks(fin)[1])
	assert.Equal(t, "the reasoning", anthChatConcatDelta(fin, "text_delta"))

	md := anthChatMessageDelta(fin)
	require.NotNil(t, md)
	assert.Equal(t, "end_turn", md.Delta.StopReason)
}

// ---------------------------------------------------------------------------
// ChatCompletionsStreamToAnthropicResponse (sync collapse) tests
// ---------------------------------------------------------------------------

func TestChatCompletionsStreamToAnthropicResponse_BlockOrderAndUsage(t *testing.T) {
	chunks := []*ChatCompletionsChunk{
		{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: stringPtr("r")}}}},
		anthChatTextChunk("t"),
		{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
			{Index: intPtr(0), ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "foo", Arguments: `{"a":1}`}},
		}}}}},
		{Choices: []ChatChunkChoice{{FinishReason: stringPtr("tool_calls")}}, Usage: &ChatUsage{PromptTokens: 100, CompletionTokens: 20, PromptTokensDetails: &ChatTokenDetails{CachedTokens: 30}}},
	}

	resp := ChatCompletionsStreamToAnthropicResponse(chunks, "deepseek")
	require.NotNil(t, resp)
	assert.Equal(t, "id", resp.ID)
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)

	// Block order mirrors ResponsesToAnthropic: thinking, text, tool_use.
	require.Len(t, resp.Content, 3)
	assert.Equal(t, "thinking", resp.Content[0].Type)
	assert.Equal(t, "r", resp.Content[0].Thinking)
	assert.Equal(t, "text", resp.Content[1].Type)
	assert.Equal(t, "t", resp.Content[1].Text)
	assert.Equal(t, "tool_use", resp.Content[2].Type)
	assert.Equal(t, "call_a", resp.Content[2].ID)
	assert.Equal(t, "foo", resp.Content[2].Name)
	assert.JSONEq(t, `{"a":1}`, string(resp.Content[2].Input))

	assert.Equal(t, "tool_use", resp.StopReason)
	assert.Equal(t, 70, resp.Usage.InputTokens)
	assert.Equal(t, 30, resp.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, resp.Usage.OutputTokens)
}

func TestChatCompletionsStreamToAnthropicResponse_ReasoningOnlyFallback(t *testing.T) {
	chunks := []*ChatCompletionsChunk{
		{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: stringPtr("just thinking")}}}},
		anthChatFinishChunk("stop"),
	}

	resp := ChatCompletionsStreamToAnthropicResponse(chunks, "m")
	require.Len(t, resp.Content, 2)
	assert.Equal(t, "thinking", resp.Content[0].Type)
	assert.Equal(t, "text", resp.Content[1].Type)
	assert.Equal(t, "just thinking", resp.Content[1].Text)
	assert.Equal(t, "end_turn", resp.StopReason)
}
