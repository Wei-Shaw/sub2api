package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// GLM reports an interrupted inference with a provider-specific finish_reason
// instead of dropping the connection. The bridge must mark the stream as
// failed, not let the finalizer mask it as a clean completed response.
func TestChatCompletionsChunkToResponsesEvents_GLMNetworkErrorMarksInterrupted(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("glm-5.3-flash")
	chunk := &ChatCompletionsChunk{
		ID: "chatcmpl_glm_cut",
		Choices: []ChatChunkChoice{
			{Index: 0, FinishReason: strPtr("network_error")},
		},
	}

	ChatCompletionsChunkToResponsesEvents(chunk, state)

	require.True(t, state.UpstreamInterrupted)
	require.False(t, state.CompletedSent)
}

// GLM's model_context_window_exceeded means the output hit the length limit,
// which maps to the canonical length finish_reason.
func TestChatCompletionsChunkToResponsesEvents_GLMContextWindowMapsToLength(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("glm-5.3-flash")
	chunk := &ChatCompletionsChunk{
		Choices: []ChatChunkChoice{
			{Index: 0, FinishReason: strPtr("model_context_window_exceeded")},
		},
	}

	ChatCompletionsChunkToResponsesEvents(chunk, state)

	require.Equal(t, "length", state.FinishReason)
	require.False(t, state.UpstreamInterrupted)
}

// GLM's sensitive maps to the canonical content_filter finish_reason.
func TestChatCompletionsChunkToResponsesEvents_GLMSensitiveMapsToContentFilter(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("glm-5.3-flash")
	chunk := &ChatCompletionsChunk{
		Choices: []ChatChunkChoice{
			{Index: 0, FinishReason: strPtr("sensitive")},
		},
	}

	ChatCompletionsChunkToResponsesEvents(chunk, state)

	require.Equal(t, "content_filter", state.FinishReason)
	require.False(t, state.UpstreamInterrupted)
}

// An interrupted stream must end with response.failed, never a fake
// response.completed whose status looks healthy while the text is truncated.
func TestFinalizeChatCompletionsResponsesStream_InterruptedEmitsFailed(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("glm-5.3-flash")
	state.UpstreamInterrupted = true

	events := FinalizeChatCompletionsResponsesStream(state)

	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, "response.failed", last.Type)
	require.NotNil(t, last.Response)
	require.Equal(t, "failed", last.Response.Status)
	require.NotNil(t, last.Response.Error)
}

// A content-filtered finish must surface as an incomplete response so Codex
// can tell the answer did not complete naturally.
func TestFinalizeChatCompletionsResponsesStream_ContentFilterEmitsIncomplete(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("glm-5.3-flash")
	state.FinishReason = "content_filter"

	events := FinalizeChatCompletionsResponsesStream(state)

	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, "response.completed", last.Type)
	require.Equal(t, "incomplete", last.Response.Status)
	require.NotNil(t, last.Response.IncompleteDetails)
	require.Equal(t, "content_filter", last.Response.IncompleteDetails.Reason)
}

// Non-streaming conversions apply the same normalization.
func TestChatCompletionsResponseToResponses_GLMInterruptedMapsToFailed(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID: "chatcmpl_glm_cut",
		Choices: []ChatChoice{{
			Index:        0,
			FinishReason: "network_error",
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage("\"partial\"")},
		}},
	}

	out := ChatCompletionsResponseToResponsesWithToolMapping(resp, "glm-5.3-flash", ResponsesClientToolMapping{})

	require.Equal(t, "failed", out.Status)
	require.NotNil(t, out.Error)
}

func TestChatCompletionsResponseToResponses_GLMContextWindowMapsToIncomplete(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID: "chatcmpl_glm_length",
		Choices: []ChatChoice{{
			Index:        0,
			FinishReason: "model_context_window_exceeded",
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage("\"partial\"")},
		}},
	}

	out := ChatCompletionsResponseToResponsesWithToolMapping(resp, "glm-5.3-flash", ResponsesClientToolMapping{})

	require.Equal(t, "incomplete", out.Status)
	require.NotNil(t, out.IncompleteDetails)
	require.Equal(t, "max_output_tokens", out.IncompleteDetails.Reason)
}

func TestChatCompletionsResponseToResponses_GLMSensitiveMapsToContentFilter(t *testing.T) {
	resp := &ChatCompletionsResponse{
		ID: "chatcmpl_glm_filter",
		Choices: []ChatChoice{{
			Index:        0,
			FinishReason: "sensitive",
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage("\"partial\"")},
		}},
	}

	out := ChatCompletionsResponseToResponsesWithToolMapping(resp, "glm-5.3-flash", ResponsesClientToolMapping{})

	require.Equal(t, "incomplete", out.Status)
	require.NotNil(t, out.IncompleteDetails)
	require.Equal(t, "content_filter", out.IncompleteDetails.Reason)
}
