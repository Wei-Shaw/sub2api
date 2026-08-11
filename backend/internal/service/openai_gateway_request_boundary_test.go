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

func TestNormalizeOpenAIResponsesRequestBoundaryPreservesUnrelatedBytes(t *testing.T) {
	body := []byte(`{ "model" : "gpt-5", "parallel_tool_calls" : true, "input" : [{"type":"reasoning","id":"rs_1"}] }`)
	want := []byte(`{ "model" : "gpt-5", "input" : [{"type":"reasoning","id":"rs_1"}] }`)

	normalized, changed, err := normalizeOpenAIResponsesRequestBoundary(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, want, normalized)
}

func TestNormalizeOpenAIResponsesRequestBoundaryDoesNotSanitizeStatelessReplayForAnyAccount(t *testing.T) {
	body := []byte(`{
		"store":false,
		"tools":[{"type":"function","name":"lookup"}],
		"parallel_tool_calls":true,
		"input":[
			{"type":"reasoning","id":"rs_encrypted","encrypted_content":"cipher","summary":null},
			{"type":"reasoning","id":"rs_plain","summary":[]},
			{"type":"item_reference","id":"rs_reference"}
		]
	}`)

	for _, accountKind := range []string{"OpenAI OAuth", "OpenAI API key", "Grok", "non-target account"} {
		t.Run(accountKind, func(t *testing.T) {
			normalized, changed, err := normalizeOpenAIResponsesRequestBoundary(body)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, normalized)
		})
	}
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
