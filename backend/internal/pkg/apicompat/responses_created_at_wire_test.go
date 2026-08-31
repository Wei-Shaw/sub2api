package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// issue #5601：严格的 Responses 客户端（Rust serde 系，如 Codex / Grok CLI）把
// created_at 声明为必填字段，缺失即 `missing field 'created_at'` 反序列化失败。
// 网关合成的 Responses 对象（Chat→Responses、Anthropic→Responses 两座桥）此前从不
// 写这个字段——尽管两个流式 state 早就采集好了 Created 时间戳，只是没有出口。
// 原生 Responses 透传走 gjson/sjson 字节级改写，不受影响。

// responseObjectOf 取出事件里的 response 子对象（按线格式，而不是按 Go 结构体）。
func responseObjectOf(t *testing.T, evt ResponsesStreamEvent) map[string]any {
	t.Helper()
	m := marshalEvent(t, evt)
	resp, ok := m["response"].(map[string]any)
	require.True(t, ok, "event must carry a response object: %v", m)
	return resp
}

func requireCreatedAt(t *testing.T, resp map[string]any) int64 {
	t.Helper()
	raw, ok := resp["created_at"]
	require.True(t, ok, "response 对象必须带 created_at，否则严格客户端直接反序列化失败")
	value, ok := raw.(float64)
	require.True(t, ok, "created_at 必须是数字，得到 %T", raw)
	require.Greater(t, int64(value), int64(0), "created_at 必须是有效的 unix 时间戳")
	return int64(value)
}

// omitempty 陷阱守卫：created_at 为 0 时也必须出现在线格式里，
// 否则「字段存在」这件事就依赖于运行时恰好非零。
func TestWire_CreatedAtPresentEvenAtZero(t *testing.T) {
	resp := responseObjectOf(t, ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_1", Object: "response", Status: "in_progress"},
	})
	require.Contains(t, resp, "created_at", "created_at 不得带 omitempty")
	require.EqualValues(t, 0, resp["created_at"])
}

// ---------------------------------------------------------------------------
// Chat Completions → Responses
// ---------------------------------------------------------------------------

func TestChatCompletionsResponseToResponses_CarriesCreatedAt(t *testing.T) {
	t.Run("uses_upstream_created_when_present", func(t *testing.T) {
		out := ChatCompletionsResponseToResponses(&ChatCompletionsResponse{
			ID:      "chatcmpl_1",
			Created: 1700000000,
			Model:   "deepseek-v4-flash",
			Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)}}},
		}, "deepseek-v4-flash", nil, nil, false, nil)
		require.EqualValues(t, 1700000000, out.CreatedAt, "上游给了 created 就照搬，不要另起时间")
	})

	t.Run("stamps_now_when_upstream_omits_created", func(t *testing.T) {
		out := ChatCompletionsResponseToResponses(&ChatCompletionsResponse{
			ID:      "chatcmpl_2",
			Model:   "deepseek-v4-flash",
			Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)}}},
		}, "deepseek-v4-flash", nil, nil, false, nil)
		require.Greater(t, out.CreatedAt, int64(0))
	})

	t.Run("nil_upstream_response_still_stamps", func(t *testing.T) {
		out := ChatCompletionsResponseToResponses(nil, "deepseek-v4-flash", nil, nil, false, nil)
		require.Greater(t, out.CreatedAt, int64(0), "空上游响应也必须产出可解析的对象")
	})
}

// 同一条流里 response.created 与终止事件必须报同一个 created_at
// （官方语义：created_at 是这次 response 的创建时刻，不随事件变化）。
func TestChatCompletionsToResponsesStream_CreatedAtStableAcrossEvents(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-flash")
	require.Greater(t, state.Created, int64(0), "前提：state 早就采集了时间戳")

	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal(
		[]byte(`{"choices":[{"index":0,"delta":{"content":"hi"}}]}`), &chunk))

	events := ChatCompletionsChunkToResponsesEvents(&chunk, state)
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	seen := map[string]int64{}
	for _, evt := range events {
		if evt.Response == nil {
			continue
		}
		seen[evt.Type] = requireCreatedAt(t, responseObjectOf(t, evt))
	}

	require.Contains(t, seen, "response.created")
	require.Contains(t, seen, "response.completed")
	require.Equal(t, state.Created, seen["response.created"])
	require.Equal(t, seen["response.created"], seen["response.completed"],
		"同一条流的 created_at 必须恒定")
}

// ---------------------------------------------------------------------------
// Anthropic → Responses
// ---------------------------------------------------------------------------

func TestAnthropicToResponsesResponse_StampsCreatedAt(t *testing.T) {
	out := AnthropicToResponsesResponse(&AnthropicResponse{
		ID:      "msg_1",
		Type:    "message",
		Role:    "assistant",
		Model:   "claude-sonnet-4-20250514",
		Content: []AnthropicContentBlock{{Type: "text", Text: "hi"}},
	})
	require.Greater(t, out.CreatedAt, int64(0),
		"Anthropic 响应不带时间戳，网关必须自己盖一个")
}

func TestAnthropicEventToResponsesStream_CreatedAtStableAcrossEvents(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-20250514"
	require.Greater(t, state.Created, int64(0), "前提：state 早就采集了时间戳")

	var events []ResponsesStreamEvent
	for _, raw := range []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	} {
		var evt AnthropicStreamEvent
		require.NoError(t, json.Unmarshal([]byte(raw), &evt))
		events = append(events, AnthropicEventToResponsesEvents(&evt, state)...)
	}
	events = append(events, FinalizeAnthropicResponsesStream(state)...)

	seen := map[string]int64{}
	for _, evt := range events {
		if evt.Response == nil {
			continue
		}
		seen[evt.Type] = requireCreatedAt(t, responseObjectOf(t, evt))
	}

	require.Contains(t, seen, "response.created")
	require.Contains(t, seen, "response.completed")
	require.Equal(t, state.Created, seen["response.created"])
	require.Equal(t, seen["response.created"], seen["response.completed"],
		"同一条流的 created_at 必须恒定")
}

// ResponsesClientToolStreamRestorer 对部分事件走 unmarshal→re-marshal。
// 结构体没有该字段时，上游带来的 created_at 会在这一步被静默抹掉。
func TestResponsesStreamEvent_CreatedAtSurvivesUnmarshalRemarshal(t *testing.T) {
	upstream := []byte(`{"type":"response.completed","response":{"id":"resp_9","object":"response",` +
		`"created_at":1700000123,"model":"gpt-5.5","status":"completed","output":[]}}`)

	var evt ResponsesStreamEvent
	require.NoError(t, json.Unmarshal(upstream, &evt))
	require.EqualValues(t, 1700000123, evt.Response.CreatedAt)

	require.EqualValues(t, 1700000123, requireCreatedAt(t, responseObjectOf(t, evt)))
}
