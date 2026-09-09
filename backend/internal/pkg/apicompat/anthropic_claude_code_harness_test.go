//go:build unit

package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicToResponses_ClaudeCodeReplaysCodexReasoningAndToolError(t *testing.T) {
	var req AnthropicRequest
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gpt-5.6-sol",
		"max_tokens":1024,
		"system":[{"type":"text","text":"sanitized Claude Code system fixture"}],
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"sanitized summary","signature":"gAAAA-sanitized-codex-ciphertext"},
				{"type":"tool_use","id":"call_fixture_1","name":"Bash","input":{"command":"false","description":"fixture"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"call_fixture_1","is_error":true,"content":"exit code 1"},
				{"type":"text","text":"recover and continue"}
			]}
		],
		"tools":[{"name":"Bash","description":"run a command","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}]
	}`), &req))

	converted, err := AnthropicToResponses(&req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(converted.Input, &items))
	require.Len(t, items, 5)
	require.Equal(t, "developer", items[0].Role)
	require.Equal(t, "reasoning", items[1].Type)
	require.Equal(t, "gAAAA-sanitized-codex-ciphertext", items[1].EncryptedContent)
	require.Equal(t, "function_call", items[2].Type)
	require.Equal(t, "call_fixture_1", items[2].CallID)
	require.Equal(t, "function_call_output", items[3].Type)
	require.Equal(t, "call_fixture_1", items[3].CallID)
	require.Equal(t, "Error: exit code 1", items[3].Output)
	require.Equal(t, "user", items[4].Role)
	require.NotNil(t, converted.ParallelToolCalls)
	require.True(t, *converted.ParallelToolCalls)
	require.Contains(t, converted.Include, "reasoning.encrypted_content")
}

func TestResponsesToAnthropic_RefusalRemainsVisibleToClaudeCode(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_refusal",
		Model:  "gpt-5.6-sol",
		Status: "completed",
		Output: []ResponsesOutput{{
			Type: "message",
			Content: []ResponsesContentPart{{
				Type:    "refusal",
				Refusal: "sanitized refusal reason",
			}},
		}},
	}

	converted := ResponsesToAnthropic(resp, "claude-opus-5")
	require.Len(t, converted.Content, 1)
	require.Equal(t, "text", converted.Content[0].Type)
	require.Equal(t, "Refusal: sanitized refusal reason", converted.Content[0].Text)
}

func TestResponsesToAnthropic_StreamingRefusalRemainsVisibleToClaudeCode(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.Model = "claude-opus-5"
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:  "response.refusal.delta",
		Delta: "sanitized refusal reason",
	}, state)

	require.Len(t, events, 2)
	require.Equal(t, "content_block_start", events[0].Type)
	require.Equal(t, "content_block_delta", events[1].Type)
	require.Equal(t, "text_delta", events[1].Delta.Type)
	require.Equal(t, "Refusal: sanitized refusal reason", events[1].Delta.Text)
}
