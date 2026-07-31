package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIResponsesRequestBoundaryParallelToolCalls(t *testing.T) {
	tests := []struct {
		name string
		body string
		keep bool
	}{
		{name: "tools missing", body: `{"model":"gpt-5","parallel_tool_calls":true}`},
		{name: "tools null", body: `{"tools":null,"parallel_tool_calls":true}`},
		{name: "tools object", body: `{"tools":{"type":"function"},"parallel_tool_calls":true}`},
		{name: "tools empty", body: `{"tools":[],"parallel_tool_calls":true}`},
		{name: "tools non empty", body: `{"tools":[{"type":"function","name":"lookup"}],"parallel_tool_calls":true}`, keep: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, _, err := normalizeOpenAIResponsesRequestBoundary([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.keep, gjson.GetBytes(normalized, "parallel_tool_calls").Exists())
		})
	}
}

func TestNormalizeOpenAIResponsesRequestBoundaryStatelessReplay(t *testing.T) {
	body := []byte(`{
		"store":false,
		"tools":[{"type":"function","name":"lookup"}],
		"parallel_tool_calls":true,
		"input":[
			{"type":"reasoning","id":"rs_keep","encrypted_content":"cipher","summary":null,"opaque":{"kept":true}},
			{"type":"reasoning","id":"rs_drop_missing","summary":[]},
			{"type":"reasoning","id":"rs_drop_empty","encrypted_content":""},
			{"type":"item_reference","id":"rs_reference"},
			{"type":"message","id":"msg_1","role":"user","content":"hello"},
			{"type":"function_call","id":"rs_function","call_id":"call_1","name":"lookup","arguments":"{}"},
			{"type":"function_call_output","id":"out_1","call_id":"call_1","output":"ok"},
			{"type":"reasoning","id":"reasoning_ordinary","summary":null},
			{"type":"item_reference","id":"call_1"}
		]
	}`)

	normalized, changed, err := normalizeOpenAIResponsesRequestBoundary(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Len(t, gjson.GetBytes(normalized, "input").Array(), 6)
	require.False(t, gjson.GetBytes(normalized, "input.0.id").Exists())
	require.Equal(t, "cipher", gjson.GetBytes(normalized, "input.0.encrypted_content").String())
	require.True(t, gjson.GetBytes(normalized, "input.0.summary").IsArray())
	require.True(t, gjson.GetBytes(normalized, "input.0.opaque.kept").Bool())
	require.Equal(t, "msg_1", gjson.GetBytes(normalized, "input.1.id").String())
	require.Equal(t, "rs_function", gjson.GetBytes(normalized, "input.2.id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(normalized, "input.3.type").String())
	require.Equal(t, "reasoning_ordinary", gjson.GetBytes(normalized, "input.4.id").String())
	require.True(t, gjson.GetBytes(normalized, "input.4.summary").Exists())
	require.Equal(t, "call_1", gjson.GetBytes(normalized, "input.5.id").String())
}

func TestNormalizeOpenAIResponsesRequestBoundaryDoesNotSanitizeStoredRequests(t *testing.T) {
	for _, store := range []string{"", `"store":true,`, `"store":null,`} {
		body := []byte(`{` + store + `"tools":[{"type":"function"}],"input":[{"type":"reasoning","id":"rs_1","encrypted_content":"cipher","summary":null},{"type":"item_reference","id":"rs_1"}]}`)
		normalized, changed, err := normalizeOpenAIResponsesRequestBoundary(body)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, string(body), string(normalized))
	}
}

func TestNormalizeOpenAIResponsesRequestBoundaryRemovesParallelToolCallsFromCompactBody(t *testing.T) {
	compact, changed, err := normalizeOpenAICompactRequestBody([]byte(`{"model":"gpt-5","input":[],"parallel_tool_calls":true,"stream":false}`))
	require.NoError(t, err)
	require.True(t, changed)

	normalized, changed, err := normalizeOpenAIResponsesRequestBoundary(compact)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "parallel_tool_calls").Exists())
}
