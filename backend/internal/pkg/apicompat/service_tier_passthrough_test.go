package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// 上游响应中的 service_tier 必须如实回传，不被重写/丢弃：
// 非流式（ResponsesResponse → ChatCompletionsResponse）与流式 chunk 均覆盖。

func TestResponsesToChatCompletions_PreservesUpstreamServiceTier(t *testing.T) {
	resp := &ResponsesResponse{
		ID:          "resp_1",
		Object:      "response",
		Model:       "gpt-5.5",
		Status:      "completed",
		ServiceTier: "priority",
		Output: []ResponsesOutput{{
			Type: "message",
			Role: "assistant",
			Content: []ResponsesContentPart{{
				Type: "output_text",
				Text: "hi",
			}},
		}},
		Usage: &ResponsesUsage{InputTokens: 1, OutputTokens: 1},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-5.5")
	require.Equal(t, "priority", chat.ServiceTier)

	// 序列化后字段仍在（omitempty 不丢非空值）。
	raw, err := json.Marshal(chat)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"service_tier":"priority"`)
}

func TestResponsesToChatCompletions_OmitsMissingServiceTier(t *testing.T) {
	resp := &ResponsesResponse{ID: "resp_1", Model: "gpt-5.5", Status: "completed"}
	chat := ResponsesToChatCompletions(resp, "gpt-5.5")
	require.Empty(t, chat.ServiceTier)
	raw, err := json.Marshal(chat)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "service_tier")
}

func TestResponsesEventToChatChunks_PreservesUpstreamServiceTier(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.IncludeUsage = true

	created := &ResponsesStreamEvent{Type: "response.created"}
	require.NoError(t, json.Unmarshal([]byte(`{"type":"response.created","response":{"id":"resp_s1","model":"gpt-5.5","service_tier":"priority","status":"in_progress"}}`), created))

	chunks := ResponsesEventToChatChunks(created, state)
	require.NotEmpty(t, chunks)
	for _, chunk := range chunks {
		require.Equal(t, "priority", chunk.ServiceTier)
	}

	// 后续 delta chunk 继续携带（OpenAI 流式 chunk 的 service_tier 语义）。
	delta := &ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "hi"}
	chunks = ResponsesEventToChatChunks(delta, state)
	require.NotEmpty(t, chunks)
	require.Equal(t, "priority", chunks[0].ServiceTier)

	// 终止事件同样携带。
	completed := &ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{
		ID: "resp", Model: "gpt-5.5", Status: "completed",
		Usage: &ResponsesUsage{InputTokens: 1, OutputTokens: 1},
	}}
	chunks = ResponsesEventToChatChunks(completed, state)
	require.NotEmpty(t, chunks)
	for _, chunk := range chunks {
		require.Equal(t, "priority", chunk.ServiceTier)
	}
}

func TestResponsesEventToChatChunks_NoServiceTierStaysClean(t *testing.T) {
	state := NewResponsesEventToChatState()
	created := &ResponsesStreamEvent{Type: "response.created", Response: &ResponsesResponse{ID: "resp", Model: "gpt-5.5"}}
	chunks := ResponsesEventToChatChunks(created, state)
	require.NotEmpty(t, chunks)
	require.Empty(t, chunks[0].ServiceTier)
	raw, err := json.Marshal(chunks[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "service_tier")
}

// 上游 JSON 反序列化时 service_tier 进入 ResponsesResponse（缓冲桥读取链路）。
func TestResponsesResponse_UnmarshalPreservesServiceTier(t *testing.T) {
	var resp ResponsesResponse
	require.NoError(t, json.Unmarshal([]byte(`{"id":"resp_1","object":"response","model":"gpt-5.5","status":"completed","service_tier":"flex","output":[]}`), &resp))
	require.Equal(t, "flex", resp.ServiceTier)
}

// ---------------------------------------------------------------------------
// 反向转换（Chat-only fallback）：CC 响应/流 chunk 的 service_tier 保留到
// Responses 形态，客户端与计费都能拿到上游回显。
// ---------------------------------------------------------------------------

func TestChatCompletionsResponseToResponses_PreservesServiceTier(t *testing.T) {
	cc := &ChatCompletionsResponse{
		ID:          "chatcmpl-1",
		Model:       "gpt-5.5",
		ServiceTier: "default",
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			FinishReason: "stop",
		}},
		Usage: &ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}
	resp := ChatCompletionsResponseToResponses(cc, "gpt-5.5", nil, nil, false, nil)
	require.Equal(t, "default", resp.ServiceTier)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"service_tier":"default"`)
}

func TestChatCompletionsResponseToResponses_NilRespOmitsServiceTier(t *testing.T) {
	resp := ChatCompletionsResponseToResponses(nil, "gpt-5.5", nil, nil, false, nil)
	require.Empty(t, resp.ServiceTier)
}

func TestChatCompletionsChunkToResponsesEvents_PreservesServiceTier(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("gpt-5.5")
	chunk := &ChatCompletionsChunk{
		ID:          "chatcmpl-2",
		Model:       "gpt-5.5",
		ServiceTier: "flex",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{Content: strPtr("hi")},
		}},
	}
	events := ChatCompletionsChunkToResponsesEvents(chunk, state)
	require.NotEmpty(t, events)

	// response.created 携带 service_tier。
	created := findEvent(events, "response.created")
	require.NotNil(t, created)
	require.NotNil(t, created.Response)
	require.Equal(t, "flex", created.Response.ServiceTier)

	// 终止事件同样携带。
	final := FinalizeChatCompletionsResponsesStream(state)
	completed := findEvent(final, "response.completed")
	require.NotNil(t, completed)
	require.NotNil(t, completed.Response)
	require.Equal(t, "flex", completed.Response.ServiceTier)
}

func TestChatCompletionsChunkToResponsesEvents_NoTierStaysClean(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("gpt-5.5")
	chunk := &ChatCompletionsChunk{ID: "chatcmpl-3", Model: "gpt-5.5"}
	events := ChatCompletionsChunkToResponsesEvents(chunk, state)
	created := findEvent(events, "response.created")
	require.NotNil(t, created)
	require.Empty(t, created.Response.ServiceTier)
	raw, err := json.Marshal(created)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "service_tier")
}

func findEvent(events []ResponsesStreamEvent, eventType string) *ResponsesStreamEvent {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}
