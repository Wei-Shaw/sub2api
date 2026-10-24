package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRestoreToolNamesInBytes_StructuredRoundTrip(t *testing.T) {
	rw := buildTestToolNameRewrite([]byte(`{"tools":[{"name":"read"},{"name":"mcp__tools__read_file"}]}`))
	alias := rw.Forward["read"]
	tests := []struct {
		name, body, path string
	}{
		{"message", `{"type":"message","content":[{"type":"tool_use","name":"ALIAS","input":{"name":"ALIAS"}},{"type":"tool_use","name":"mcp__tools__read_file","input":{}},{"type":"text","text":"ALIAS"},{"type":"thinking","thinking":"ALIAS","signature":"ALIAS"}]}`, "content.0.name"},
		{"message_start", `{"type":"message_start","message":{"type":"message","content":[{"type":"tool_use","name":"ALIAS","input":{"name":"ALIAS"}}]}}`, "message.content.0.name"},
		{"content_start", `{"type":"content_block_start","content_block":{"type":"tool_use","name":"ALIAS","input":{"name":"ALIAS"}}}`, "content_block.name"},
		{"search_result", `{"type":"message","content":[{"type":"tool_search_tool_result","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"ALIAS"},{"type":"tool_reference","tool_name":"mcp__tools__read_file"}]}}]}`, "content.0.content.tool_references.0.tool_name"},
		{"search_start", `{"type":"content_block_start","content_block":{"type":"tool_search_tool_result","content":{"type":"tool_search_tool_search_result","tool_references":[{"type":"tool_reference","tool_name":"ALIAS"}]}}}`, "content_block.content.tool_references.0.tool_name"},
		{"tool_result", `{"type":"message","content":[{"type":"tool_result","content":[{"type":"tool_reference","tool_name":"ALIAS"},{"type":"text","text":"ALIAS"}]}]}`, "content.0.content.0.tool_name"},
		{"chat", `{"object":"chat.completion","choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"ALIAS","arguments":"ALIAS"}}],"content":"ALIAS"}}]}`, "choices.0.message.tool_calls.0.function.name"},
		{"chat_delta", `{"object":"chat.completion.chunk","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"ALIAS","arguments":"ALIAS"}}],"content":"ALIAS"}}]}`, "choices.0.delta.tool_calls.0.function.name"},
		{"response", `{"object":"response","output":[{"type":"function_call","name":"ALIAS","arguments":"ALIAS"},{"type":"message","content":[{"type":"output_text","text":"ALIAS"}]}]}`, "output.0.name"},
		{"response_item", `{"type":"response.output_item.added","item":{"type":"function_call","name":"ALIAS","arguments":"ALIAS"}}`, "item.name"},
		{"custom_item", `{"type":"response.output_item.done","item":{"type":"custom_tool_call","name":"ALIAS","input":"ALIAS"}}`, "item.name"},
		{"response_completed", `{"type":"response.completed","response":{"object":"response","output":[{"type":"function_call","name":"ALIAS","arguments":"ALIAS"}]}}`, "response.output.0.name"},
		{"arguments_done", `{"type":"response.function_call_arguments.done","name":"ALIAS","arguments":"ALIAS"}`, "name"},
		{"arguments_delta", `{"type":"response.function_call_arguments.delta","name":"ALIAS","delta":"ALIAS"}`, "name"},
		{"custom_input_done", `{"type":"response.custom_tool_call_input.done","name":"ALIAS","input":"ALIAS"}`, "name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(strings.ReplaceAll(tc.body, "ALIAS", alias))
			result := gjson.GetBytes(body, tc.path)
			require.Equal(t, alias, result.String())
			want := string(body[:result.Index]) + `"read"` + string(body[result.Index+len(result.Raw):])
			require.Equal(t, want, string(restoreToolNamesInBytes(body, rw)))
			for _, ending := range []string{"\n", "\r\n"} {
				prefix := "event: test" + ending + "data: "
				suffix := ending + ending + "data:[DONE]" + ending + ending
				require.Equal(t, prefix+want+suffix, string(restoreToolNamesInBytes([]byte(prefix+string(body)+suffix), rw)))
			}
		})
	}
}

func TestRestoreToolNamesInBytes_EscapedOriginalName(t *testing.T) {
	original := "quoted\"\\\n工具"
	body, err := json.Marshal(map[string]any{"tools": []any{map[string]string{"name": original}}})
	require.NoError(t, err)
	rw := buildTestToolNameRewrite(body)
	response := []byte(`{"type":"message","content":[{"type":"tool_use","name":"` + rw.Forward[original] + `","input":{}}]}`)
	out := restoreToolNamesInBytes(response, rw)
	require.True(t, json.Valid(out), string(out))
	require.Equal(t, original, gjson.GetBytes(out, "content.0.name").String())
}

func TestRestoreToolNamesInBytes_OpaqueAndInvalidPayloads(t *testing.T) {
	rw := buildTestToolNameRewrite([]byte(`{"tools":[{"name":"read"}]}`))
	alias := rw.Forward["read"]
	for _, body := range []string{
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ALIAS"}}`,
		`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"ALIAS"}}`,
		`{"type":"content_block_delta","delta":{"type":"signature_delta","signature":"ALIAS"}}`,
		`{"type":"error","error":{"message":"ALIAS"}}`,
		`{"type":"message","metadata":{"name":"ALIAS"},"content":[{"type":"server_tool_use","name":"ALIAS"}]}`,
		`{"type":"message","content":[{"type":"tool_use","name":"ALIAS_extra"}]}`,
		`{"type":"content_block_start","content_block":{"type":"tool_use","name":"ALIAS"}`,
		"data: [DONE]\n\n", "ALIAS", "", ": ALIAS\n\n",
	} {
		body = strings.ReplaceAll(body, "ALIAS", alias)
		require.Equal(t, body, string(restoreToolNamesInBytes([]byte(body), rw)))
	}
}
