package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the fix for codex (and newer Responses clients) sending
// function_call.arguments as a JSON object and function_call_output.output as
// a JSON array. Before the fix, ResponsesInputItem.Arguments / .Output were
// typed `string`, so json.Unmarshal failed:
//   - Responses→Anthropic path (ResponsesToAnthropicRequest): HTTP 502
//   - Responses→ChatCompletions path (ResponsesToChatCompletionsRequest):
//     silent data loss (rawString returned "" for non-string values)

// --- helper-level tests ---------------------------------------------------

func TestNormalizeResponsesArguments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"object", `{"x":1}`, `{"x":1}`},
		{"stringified", `"{\"x\":1}"`, `{"x":1}`},
		{"empty string", `""`, `{}`},
		{"empty raw", ``, `{}`},
		{"null", `null`, `{}`},
		{"non-json string", `"not json"`, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeResponsesArguments(json.RawMessage(tc.in))
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestExtractResponsesOutputText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain string", `"result"`, "result"},
		{"array one part", `[{"type":"output_text","text":"result"}]`, "result"},
		{"array two parts", `[{"type":"output_text","text":"a"},{"type":"output_text","text":"b"}]`, "a\n\nb"},
		{"empty raw", ``, ""},
		{"null", `null`, ""},
		{"empty array", `[]`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractResponsesOutputText(json.RawMessage(tc.in))
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- Responses→Anthropic path: must not 502 ----------------------------

func TestResponsesToAnthropicRequest_FunctionCallObjectArguments(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-4-8",
		"input": [
			{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": {"x": 1}},
			{"type": "function_call_output", "call_id": "c1", "output": "ok"}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	anth, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err) // before fix: "cannot unmarshal object ... arguments of type string"
	require.NotNil(t, anth)

	require.Len(t, anth.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anth.Messages[0].Content, &blocks))
	require.Len(t, blocks, 1)
	assert.Equal(t, "tool_use", blocks[0].Type)
	assert.Equal(t, "foo", blocks[0].Name)
	assert.JSONEq(t, `{"x":1}`, string(blocks[0].Input))
}

func TestResponsesToAnthropicRequest_FunctionCallStringifiedArguments(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-4-8",
		"input": [
			{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": "{\"x\":1}"},
			{"type": "function_call_output", "call_id": "c1", "output": "ok"}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	anth, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)

	require.Len(t, anth.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anth.Messages[0].Content, &blocks))
	require.Len(t, blocks, 1)
	assert.JSONEq(t, `{"x":1}`, string(blocks[0].Input))
}

func TestResponsesToAnthropicRequest_FunctionCallNonObjectArgumentsFallback(t *testing.T) {
	cases := []string{
		`"not-json-object"`,
		`[{"x":1}]`,
		`123`,
	}
	for _, arguments := range cases {
		t.Run(arguments, func(t *testing.T) {
			body := []byte(`{
				"model": "claude-opus-4-8",
				"input": [
					{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": ` + arguments + `},
					{"type": "function_call_output", "call_id": "c1", "output": "ok"}
				]
			}`)
			var req ResponsesRequest
			require.NoError(t, json.Unmarshal(body, &req))

			anth, err := ResponsesToAnthropicRequest(&req)
			require.NoError(t, err)

			require.Len(t, anth.Messages, 2)
			var blocks []AnthropicContentBlock
			require.NoError(t, json.Unmarshal(anth.Messages[0].Content, &blocks))
			require.Len(t, blocks, 1)
			assert.JSONEq(t, `{}`, string(blocks[0].Input))
		})
	}
}

func TestResponsesToAnthropicRequest_FunctionCallOutputArray(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-4-8",
		"input": [
			{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "c1",
			 "output": [{"type": "output_text", "text": "result"}]}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	anth, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err) // before fix: "cannot unmarshal array ... output of type string"
	require.NotNil(t, anth)

	require.Len(t, anth.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anth.Messages[1].Content, &blocks))
	require.Len(t, blocks, 1)
	assert.Equal(t, "tool_result", blocks[0].Type)
	assert.Equal(t, "toolu_c1", blocks[0].ToolUseID) // call_id is namespaced for Anthropic
	assert.JSONEq(t, `"result"`, string(blocks[0].Content))
}

func TestResponsesToAnthropicRequest_FunctionCallOutputString(t *testing.T) {
	// Backward compatibility: older clients send output as a plain string.
	body := []byte(`{
		"model": "claude-opus-4-8",
		"input": [
			{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "c1", "output": "result"}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	anth, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)

	require.Len(t, anth.Messages, 2)
	var blocks []AnthropicContentBlock
	require.NoError(t, json.Unmarshal(anth.Messages[1].Content, &blocks))
	require.Len(t, blocks, 1)
	assert.JSONEq(t, `"result"`, string(blocks[0].Content))
}

func TestResponsesToAnthropicRequest_SkipsUnknownResponsesItems(t *testing.T) {
	body := []byte(`{
		"model": "claude-opus-4-8",
		"input": [
			{"role": "user", "content": [{"type": "input_text", "text": "search and summarize"}]},
			{"type": "web_search_call", "id": "ws_1", "content": [{"type": "output_text", "text": "internal search marker"}]},
			{"type": "function_call", "call_id": "call_1", "name": "foo", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "ok"}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	anth, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	assertAnthropicPairing(t, anth.Messages)

	for _, msg := range anth.Messages {
		for _, block := range parseContentBlocks(msg.Content) {
			assert.NotEqual(t, "input_text", block.Type)
			assert.NotEqual(t, "output_text", block.Type)
			assert.NotContains(t, block.Text, "internal search marker")
		}
	}
}

// --- Responses→ChatCompletions path: must not drop data ----------------

func TestResponsesToChatCompletionsRequest_FunctionCallObjectArguments(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [
			{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": {"x": 1}},
			{"type": "function_call_output", "call_id": "c1", "output": "ok"}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	cc, err := ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)
	require.Len(t, cc.Messages, 2)
	require.Len(t, cc.Messages[0].ToolCalls, 1)
	// Chat Completions requires arguments to be a stringified JSON object;
	// before the fix rawString returned "" and it degraded to "{}".
	assert.JSONEq(t, `{"x":1}`, cc.Messages[0].ToolCalls[0].Function.Arguments)
}

func TestResponsesToChatCompletionsRequest_FunctionCallOutputArray(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [
			{"type": "function_call", "call_id": "c1", "name": "foo", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "c1",
			 "output": [{"type": "output_text", "text": "result"}]}
		]
	}`)
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal(body, &req))

	cc, err := ResponsesToChatCompletionsRequest(&req)
	require.NoError(t, err)
	require.Len(t, cc.Messages, 2)
	assert.Equal(t, "tool", cc.Messages[1].Role)
	// before the fix rawString returned "" → tool result content lost.
	assert.JSONEq(t, `"result"`, string(cc.Messages[1].Content))
}
