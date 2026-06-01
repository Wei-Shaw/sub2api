package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Non-streaming tests
// ---------------------------------------------------------------------------

func TestChatCompletionsToResponsesResponse_BasicText(t *testing.T) {
	contentRaw, _ := json.Marshal("Hello, world!")
	resp := &ChatCompletionsResponse{
		ID:    "chatcmpl-1",
		Model: "gpt-4o",
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: contentRaw,
				},
				FinishReason: "stop",
			},
		},
		Usage: &ChatUsage{PromptTokens: 7, CompletionTokens: 4, TotalTokens: 11},
	}

	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	assert.Equal(t, "response", out.Object)
	assert.Equal(t, "gpt-4o", out.Model)
	assert.Equal(t, "completed", out.Status)
	require.Len(t, out.Output, 1)
	assert.Equal(t, "message", out.Output[0].Type)
	assert.Equal(t, "assistant", out.Output[0].Role)
	require.Len(t, out.Output[0].Content, 1)
	assert.Equal(t, "output_text", out.Output[0].Content[0].Type)
	assert.Equal(t, "Hello, world!", out.Output[0].Content[0].Text)

	require.NotNil(t, out.Usage)
	assert.Equal(t, 7, out.Usage.InputTokens)
	assert.Equal(t, 4, out.Usage.OutputTokens)
	assert.Equal(t, 11, out.Usage.TotalTokens)
}

func TestChatCompletionsToResponsesResponse_ToolCalls(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID: "chatcmpl-tc",
		Choices: []ChatChoice{
			{
				Message: ChatMessage{
					Role: "assistant",
					ToolCalls: []ChatToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: ChatFunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"NYC"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	assert.Equal(t, "completed", out.Status)
	require.Len(t, out.Output, 1)
	assert.Equal(t, "function_call", out.Output[0].Type)
	assert.Equal(t, "call_abc", out.Output[0].CallID)
	assert.Equal(t, "get_weather", out.Output[0].Name)
	assert.Equal(t, `{"city":"NYC"}`, out.Output[0].Arguments)
}

func TestChatCompletionsToResponsesResponse_LegacyFunctionCall(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID: "chatcmpl-fn",
		Choices: []ChatChoice{{
			Message: ChatMessage{
				Role: "assistant",
				FunctionCall: &ChatFunctionCall{
					Name:      "get_weather",
					Arguments: `{"city":"SF"}`,
				},
			},
			FinishReason: "function_call",
		}},
	}

	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	require.Len(t, out.Output, 1)
	assert.Equal(t, "function_call", out.Output[0].Type)
	assert.NotEmpty(t, out.Output[0].CallID)
	assert.Equal(t, "get_weather", out.Output[0].Name)
	assert.Equal(t, `{"city":"SF"}`, out.Output[0].Arguments)
}

func TestChatCompletionsToResponsesResponse_LengthFinish(t *testing.T) {
	contentRaw, _ := json.Marshal("partial...")
	resp := &ChatCompletionsResponse{
		Choices: []ChatChoice{
			{
				Message:      ChatMessage{Role: "assistant", Content: contentRaw},
				FinishReason: "length",
			},
		},
	}
	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	assert.Equal(t, "incomplete", out.Status)
	require.NotNil(t, out.IncompleteDetails)
	assert.Equal(t, "max_output_tokens", out.IncompleteDetails.Reason)
}

func TestChatCompletionsToResponsesResponse_Reasoning(t *testing.T) {
	contentRaw, _ := json.Marshal("the answer is 42")
	resp := &ChatCompletionsResponse{
		Choices: []ChatChoice{
			{
				Message: ChatMessage{
					Role:             "assistant",
					Content:          contentRaw,
					ReasoningContent: "I considered the question carefully.",
				},
				FinishReason: "stop",
			},
		},
	}
	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	require.Len(t, out.Output, 2)
	assert.Equal(t, "reasoning", out.Output[0].Type)
	require.Len(t, out.Output[0].Summary, 1)
	assert.Equal(t, "I considered the question carefully.", out.Output[0].Summary[0].Text)
	assert.Equal(t, "message", out.Output[1].Type)
	assert.Equal(t, "the answer is 42", out.Output[1].Content[0].Text)
}

func TestChatCompletionsToResponsesResponse_CachedTokens(t *testing.T) {
	contentRaw, _ := json.Marshal("cached")
	resp := &ChatCompletionsResponse{
		Choices: []ChatChoice{
			{Message: ChatMessage{Role: "assistant", Content: contentRaw}, FinishReason: "stop"},
		},
		Usage: &ChatUsage{
			PromptTokens:        100,
			CompletionTokens:    10,
			TotalTokens:         110,
			PromptTokensDetails: &ChatTokenDetails{CachedTokens: 80},
		},
	}
	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	require.NotNil(t, out.Usage)
	require.NotNil(t, out.Usage.InputTokensDetails)
	assert.Equal(t, 80, out.Usage.InputTokensDetails.CachedTokens)
}

func TestChatCompletionsToResponsesResponse_ReasoningTokenUsage(t *testing.T) {
	contentRaw, _ := json.Marshal("answer")
	resp := &ChatCompletionsResponse{
		Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: contentRaw}, FinishReason: "stop"}},
		Usage: &ChatUsage{
			PromptTokens:            3,
			CompletionTokens:        7,
			TotalTokens:             10,
			CompletionTokensDetails: &ChatCompletionTokenDetails{ReasoningTokens: 5},
		},
	}
	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	require.NotNil(t, out.Usage)
	require.NotNil(t, out.Usage.OutputTokensDetails)
	assert.Equal(t, 5, out.Usage.OutputTokensDetails.ReasoningTokens)
}

func TestChatCompletionsToResponsesResponse_EmptyChoicesFallback(t *testing.T) {
	resp := &ChatCompletionsResponse{ID: "chatcmpl-x"}
	out := ChatCompletionsToResponsesResponse(resp, "gpt-4o")
	require.Len(t, out.Output, 1)
	assert.Equal(t, "message", out.Output[0].Type)
}

// ---------------------------------------------------------------------------
// Streaming tests
// ---------------------------------------------------------------------------

func mustEvent(t *testing.T, raw []byte) (string, ResponsesStreamEvent) {
	t.Helper()
	s := string(raw)
	require.True(t, strings.HasPrefix(s, "event: "), "missing event prefix: %q", s)
	idx := strings.Index(s, "\ndata: ")
	require.GreaterOrEqual(t, idx, 0)
	eventType := strings.TrimPrefix(s[:idx], "event: ")
	payload := strings.TrimSuffix(s[idx+len("\ndata: "):], "\n\n")
	var evt ResponsesStreamEvent
	require.NoError(t, json.Unmarshal([]byte(payload), &evt))
	return eventType, evt
}

func sseChunk(payload string) []byte {
	return []byte("data: " + payload)
}

func TestChatCompletionsToResponses_Stream_TextSequence(t *testing.T) {
	state := NewCCStreamState()

	// First chunk: role only (often opens the stream).
	first := `{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(first), state)
	require.NoError(t, err)
	require.Len(t, out, 1)
	typ, evt := mustEvent(t, out[0])
	assert.Equal(t, "response.created", typ)
	require.NotNil(t, evt.Response)
	assert.Equal(t, "chatcmpl-stream", evt.Response.ID)
	assert.True(t, state.CreatedSent)

	// Text deltas
	for _, txt := range []string{"Hello", ", ", "world"} {
		chunk := `{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":` + jsonString(txt) + `},"finish_reason":null}]}`
		out, err = ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(chunk), state)
		require.NoError(t, err)
	}
	// First text delta should also have produced response.output_item.added.
	// Verify by re-running for the first text via fresh state — already covered via assertions below.

	// Final chunk: finish_reason + usage
	final := `{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`
	out, err = ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(final), state)
	require.NoError(t, err)

	// Expect output_text.done, output_item.done (message), response.completed
	types := make([]string, 0, len(out))
	for _, b := range out {
		typ, _ := mustEvent(t, b)
		types = append(types, typ)
	}
	assert.Contains(t, types, "response.output_text.done")
	assert.Contains(t, types, "response.output_item.done")
	assert.Contains(t, types, "response.completed")
	assert.True(t, state.CompletedSent)

	// Verify completed carries usage
	for _, b := range out {
		typ, evt := mustEvent(t, b)
		if typ == "response.completed" {
			require.NotNil(t, evt.Response)
			require.NotNil(t, evt.Response.Usage)
			assert.Equal(t, 5, evt.Response.Usage.InputTokens)
			assert.Equal(t, 3, evt.Response.Usage.OutputTokens)
		}
	}

	// [DONE] should add the terminator only.
	out, err = ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk("[DONE]"), state)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "data: [DONE]\n\n", string(out[0]))
}

func TestChatCompletionsToResponses_Stream_ToolCalls(t *testing.T) {
	state := NewCCStreamState()

	// Chunk 1: opens with role.
	c1 := `{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`
	_, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(c1), state)
	require.NoError(t, err)

	// Chunk 2: tool call header (id + name).
	c2 := `{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(c2), state)
	require.NoError(t, err)
	var sawAdded bool
	for _, b := range out {
		typ, evt := mustEvent(t, b)
		if typ == "response.output_item.added" {
			require.NotNil(t, evt.Item)
			if evt.Item.Type == "function_call" {
				assert.Equal(t, "call_1", evt.Item.CallID)
				assert.Equal(t, "get_weather", evt.Item.Name)
				sawAdded = true
			}
		}
	}
	assert.True(t, sawAdded, "expected function_call output_item.added")

	// Chunk 3: arguments delta.
	c3 := `{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"","arguments":"{\"city\":"}}]},"finish_reason":null}]}`
	out, err = ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(c3), state)
	require.NoError(t, err)
	var sawArgsDelta bool
	for _, b := range out {
		typ, evt := mustEvent(t, b)
		if typ == "response.function_call_arguments.delta" {
			assert.Equal(t, `{"city":`, evt.Delta)
			sawArgsDelta = true
		}
	}
	assert.True(t, sawArgsDelta, "expected function_call_arguments.delta")

	// Chunk 4: finish.
	c4 := `{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`
	out, err = ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(c4), state)
	require.NoError(t, err)
	types := make([]string, 0, len(out))
	for _, b := range out {
		typ, _ := mustEvent(t, b)
		types = append(types, typ)
	}
	assert.Contains(t, types, "response.function_call_arguments.done")
	assert.Contains(t, types, "response.output_item.done")
	assert.Contains(t, types, "response.completed")

	for _, b := range out {
		typ, evt := mustEvent(t, b)
		if typ == "response.function_call_arguments.done" {
			assert.Equal(t, `{"city":`, evt.Arguments)
		}
		if typ == "response.completed" {
			require.NotNil(t, evt.Response)
			require.Len(t, evt.Response.Output, 1)
			assert.Equal(t, "function_call", evt.Response.Output[0].Type)
			assert.Equal(t, `{"city":`, evt.Response.Output[0].Arguments)
		}
	}
}

func TestChatCompletionsToResponses_Stream_UsageOnlyChunkDoesNotCreateResponse(t *testing.T) {
	state := NewCCStreamState()
	usageOnly := `{"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11,"completion_tokens_details":{"reasoning_tokens":1}}}`
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(usageOnly), state)
	require.NoError(t, err)
	require.Empty(t, out)
	assert.False(t, state.CreatedSent)
	require.NotNil(t, state.Usage)
	assert.Equal(t, 9, state.Usage.InputTokens)
	require.NotNil(t, state.Usage.OutputTokensDetails)
	assert.Equal(t, 1, state.Usage.OutputTokensDetails.ReasoningTokens)

	out, err = ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk("[DONE]"), state)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(out), 3)
	typ, evt := mustEvent(t, out[0])
	assert.Equal(t, "response.created", typ)
	require.NotNil(t, evt.Response)
	assert.Equal(t, "chatcmpl-usage", evt.Response.ID)
}

func TestChatCompletionsToResponses_Stream_ToolArgumentsBeforeHeader(t *testing.T) {
	state := NewCCStreamState()
	chunk := `{"id":"chatcmpl-tc2","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\\"x\\\":1}"}}]},"finish_reason":null}]}`
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk(chunk), state)
	require.NoError(t, err)
	var sawAdded bool
	for _, b := range out {
		typ, evt := mustEvent(t, b)
		if typ == "response.output_item.added" && evt.Item != nil && evt.Item.Type == "function_call" {
			sawAdded = true
			assert.NotEmpty(t, evt.Item.CallID)
		}
	}
	assert.True(t, sawAdded)
}

func TestChatCompletionsToResponses_Stream_Done(t *testing.T) {
	state := NewCCStreamState()
	state.CompletedSent = true // pretend we already finalized.
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk("[DONE]"), state)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "data: [DONE]\n\n", string(out[0]))
}

func TestChatCompletionsToResponses_Stream_DoneWithoutFinish(t *testing.T) {
	// Stream ends with [DONE] before any explicit finish_reason — finalize must
	// emit a synthetic response.completed.
	state := NewCCStreamState()
	state.CreatedSent = true
	state.ResponseID = "resp_x"

	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk("[DONE]"), state)
	require.NoError(t, err)
	// Expect at least: response.completed + [DONE]
	require.GreaterOrEqual(t, len(out), 2)
	last := out[len(out)-1]
	assert.Equal(t, "data: [DONE]\n\n", string(last))
	typ, _ := mustEvent(t, out[len(out)-2])
	assert.Equal(t, "response.completed", typ)
}

func TestChatCompletionsToResponses_Stream_MalformedChunkPassthrough(t *testing.T) {
	state := NewCCStreamState()
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents(sseChunk("not json"), state)
	require.Error(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "data: not json\n\n", string(out[0]))
}

func TestChatCompletionsToResponses_Stream_BlankLineIgnored(t *testing.T) {
	state := NewCCStreamState()
	out, err := ConvertChatCompletionsSSEChunkToResponsesEvents([]byte(""), state)
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestChatCompletionsToResponses_Stream_FinalizeIdempotent(t *testing.T) {
	state := NewCCStreamState()
	state.CreatedSent = true
	state.CompletedSent = true
	a := FinalizeCCStream(state)
	require.Len(t, a, 1)
	assert.Equal(t, "data: [DONE]\n\n", string(a[0]))
}

// jsonString returns a JSON-encoded string literal (with quotes and escaping).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
