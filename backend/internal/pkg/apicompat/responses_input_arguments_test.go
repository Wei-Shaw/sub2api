package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicRequest_AllowsToolSearchCallObjectArgumentsWithoutTools(t *testing.T) {
	// Compaction / follow-up requests often omit `tools`, so AdaptResponsesClientTools
	// does not lower historical tool_search_call items whose `arguments` is a JSON object.
	req := &ResponsesRequest{
		Model: "claude-opus-5-5",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"find git tools"}]},
			{"type":"tool_search_call","id":"tsc_1","call_id":"call_ts1","execution":"client","status":"completed","arguments":{"query":"git","limit":5}},
			{"type":"tool_search_output","id":"tso_1","call_id":"call_ts1","execution":"client","status":"completed","tools":[{"type":"function","name":"git_status","parameters":{"type":"object"}}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]`),
	}

	got, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, got.Messages)
	// Both user turns survive (merged into one user message because tool_search items are skipped).
	require.Len(t, got.Messages, 1)
	require.Equal(t, "user", got.Messages[0].Role)
	require.Contains(t, string(got.Messages[0].Content), "find git tools")
	require.Contains(t, string(got.Messages[0].Content), "continue")
}

func TestResponsesToAnthropicRequest_AllowsFunctionCallObjectArguments(t *testing.T) {
	req := &ResponsesRequest{
		Model: "claude-sonnet-5-5",
		Input: json.RawMessage(`[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
			{"type":"function_call","call_id":"call_obj","name":"lookup","arguments":{"city":"Phoenix","limit":3}},
			{"type":"function_call_output","call_id":"call_obj","output":"sunny"}
		]`),
	}

	got, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)

	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(got.Messages[1].Content, &blocks))
	require.Len(t, blocks, 1)
	require.Equal(t, "tool_use", blocks[0].Type)
	require.Equal(t, "lookup", blocks[0].Name)
	require.JSONEq(t, `{"city":"Phoenix","limit":3}`, string(blocks[0].Input))
}

func TestResponsesInputItem_UnmarshalArgumentsStringAndNull(t *testing.T) {
	var strItem ResponsesInputItem
	require.NoError(t, json.Unmarshal([]byte(`{"type":"function_call","call_id":"c1","name":"f","arguments":"{\"a\":1}"}`), &strItem))
	require.Equal(t, `{"a":1}`, strItem.Arguments)

	var objItem ResponsesInputItem
	require.NoError(t, json.Unmarshal([]byte(`{"type":"tool_search_call","call_id":"c2","arguments":{"query":"x"}}`), &objItem))
	require.JSONEq(t, `{"query":"x"}`, objItem.Arguments)

	var nullItem ResponsesInputItem
	require.NoError(t, json.Unmarshal([]byte(`{"type":"tool_search_call","call_id":"c3","arguments":null}`), &nullItem))
	require.Equal(t, "", nullItem.Arguments)
}
