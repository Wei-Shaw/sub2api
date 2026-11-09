package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSignedThinkingModel(t *testing.T) {
	signed := []string{
		"claude-opus-5-5",
		"claude-sonnet-5-5",
		"claude-haiku-5-5",
		"gemini-3.8-flash",
		"gemini-3.8-flash-high",
		"gemini-3.8-flash-medium",
		"gemini-3-pro",
		"gemini-4-argon",
	}
	for _, m := range signed {
		assert.True(t, IsSignedThinkingModel(m), "expected %s to be signed thinking model", m)
	}

	unsigned := []string{
		"gpt-4o",
		"gpt-5.6-luna",
		"deepseek-chat",
		"unknown-model",
	}
	for _, m := range unsigned {
		assert.False(t, IsSignedThinkingModel(m), "expected %s to NOT be signed thinking model", m)
	}
}

func TestAnthropicToResponsesResponse_GeminiPreservesThinkingSignature(t *testing.T) {
	resp := &AnthropicResponse{
		ID:    "msg_123",
		Model: "gemini-3.8-flash",
		Content: []AnthropicContentBlock{
			{
				Type:      "thinking",
				Thinking:  "Thinking about arithmetic...",
				Signature: "sig_gemini_test_123",
			},
			{
				Type:  "tool_use",
				ID:    "toolu_calc_1",
				Name:  "calc",
				Input: json.RawMessage(`{"expr":"37*41"}`),
			},
		},
	}

	out := AnthropicToResponsesResponse(resp)
	require.NotNil(t, out)
	require.Len(t, out.Output, 2)

	reasoning := out.Output[0]
	assert.Equal(t, "reasoning", reasoning.Type)
	assert.Contains(t, reasoning.EncryptedContent, anthropicThinkingEnvelopePrefix)
	require.Len(t, reasoning.Summary, 1)
	assert.Equal(t, "Thinking about arithmetic...", reasoning.Summary[0].Text)

	fn := out.Output[1]
	assert.Equal(t, "function_call", fn.Type)
	assert.Equal(t, "calc", fn.Name)
}

func TestAnthropicToResponsesResponse_GeminiToolUseSignaturePreserved(t *testing.T) {
	// When Gemini produces a function call whose signature is attached directly
	// to tool_use without a separate thinking block, a reasoning item must be emitted.
	resp := &AnthropicResponse{
		ID:    "msg_456",
		Model: "gemini-3.8-flash-high",
		Content: []AnthropicContentBlock{
			{
				Type:      "tool_use",
				ID:        "toolu_calc_2",
				Name:      "calc",
				Input:     json.RawMessage(`{"expr":"2+2"}`),
				Signature: "sig_on_tool_use_789",
			},
		},
	}

	out := AnthropicToResponsesResponse(resp)
	require.NotNil(t, out)
	require.Len(t, out.Output, 2)

	reasoning := out.Output[0]
	assert.Equal(t, "reasoning", reasoning.Type)
	assert.Contains(t, reasoning.EncryptedContent, anthropicThinkingEnvelopePrefix)

	fn := out.Output[1]
	assert.Equal(t, "function_call", fn.Type)
	assert.Equal(t, "calc", fn.Name)
}

func TestAnthropicEventToResponsesEvents_GeminiStreamingPreservesSignatures(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "gemini-3.8-flash"

	// 1. message_start
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:    "msg_stream_1",
			Model: "gemini-3.8-flash",
		},
	}, state)

	// 2. thinking block start, deltas with signature, stop
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:         "content_block_start",
		Index:        intPtr(0),
		ContentBlock: &AnthropicContentBlock{Type: "thinking"},
	}, state)
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(0),
		Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: "calculating"},
	}, state)
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(0),
		Delta: &AnthropicDelta{Type: "signature_delta", Signature: "sig_stream_xyz"},
	}, state)
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: intPtr(0),
	}, state)

	// 3. tool_use block start, delta, stop
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:         "content_block_start",
		Index:        intPtr(1),
		ContentBlock: &AnthropicContentBlock{Type: "tool_use", ID: "call_1", Name: "calc"},
	}, state)
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:  "content_block_delta",
		Index: intPtr(1),
		Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"expr":"1+1"}`},
	}, state)
	_ = AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: intPtr(1),
	}, state)

	_ = FinalizeAnthropicResponsesStream(state)

	require.Len(t, state.Outputs, 2)
	assert.Equal(t, "reasoning", state.Outputs[0].Type)
	assert.Contains(t, state.Outputs[0].EncryptedContent, anthropicThinkingEnvelopePrefix)
	assert.Equal(t, "function_call", state.Outputs[1].Type)
}

func TestResponsesToAnthropicRequest_GeminiDecodesEncryptedThinkingAndPropagatesToToolUse(t *testing.T) {
	// Test the return trip: client sends back reasoning item with envelope, followed by function_call.
	block := AnthropicContentBlock{
		Type:      "thinking",
		Thinking:  "Thinking step...",
		Signature: "sig_gemini_echo_123",
	}
	encrypted := encodeAnthropicThinking(block)

	inputJSON := []byte(`[
		{"type":"reasoning","id":"rs_1","encrypted_content":"` + encrypted + `"},
		{"type":"function_call","call_id":"call_calc_1","name":"calc","arguments":"{\"expr\":\"5*5\"}"},
		{"type":"function_call_output","call_id":"call_calc_1","output":"25"}
	]`)

	req := &ResponsesRequest{
		Model: "gemini-3.8-flash",
		Input: inputJSON,
	}

	converted, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)

	// Assistant turn: should contain thinking block AND tool_use block with Signature set
	var assistantBlocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[0].Content, &assistantBlocks))
	require.Len(t, assistantBlocks, 2)

	assert.Equal(t, "thinking", assistantBlocks[0].Type)
	assert.Equal(t, "sig_gemini_echo_123", assistantBlocks[0].Signature)

	assert.Equal(t, "tool_use", assistantBlocks[1].Type)
	assert.Equal(t, "calc", assistantBlocks[1].Name)
	assert.Equal(t, "sig_gemini_echo_123", assistantBlocks[1].Signature)

	// User turn: should contain tool_result block
	var userBlocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(converted.Messages[1].Content, &userBlocks))
	require.Len(t, userBlocks, 1)
	assert.Equal(t, "tool_result", userBlocks[0].Type)
}
