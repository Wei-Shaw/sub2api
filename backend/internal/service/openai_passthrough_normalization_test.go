package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIPassthroughOAuthBody_RemovesUnsupportedUser(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello","user":"user_123","metadata":{"user_id":"user_123"},"prompt_cache_retention":"24h","safety_identifier":"sid","stream_options":{"include_usage":true}}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	for _, field := range openAIChatGPTInternalUnsupportedFields {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be stripped", field)
	}
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
}

func TestNormalizeOpenAIPassthroughOAuthBody_CompactRemovesUnsupportedUser(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello","user":"user_123","metadata":{"user_id":"user_123"},"stream":true,"store":true}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, true)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "user").Exists())
	require.False(t, gjson.GetBytes(normalized, "metadata").Exists())
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
}

func TestNormalizeOpenAIPassthroughOAuthBody_StripsInputNamespaceOnly(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","namespace":"codex","sequence_number":9007199254740993,"content":[{"type":"input_text","text":"namespace stays in text","namespace":"inner"}]},{"type":"function_call","namespace":"shell","arguments":"{}"}],"tools":[{"type":"namespace","namespace":"functions"}]}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "input.0.namespace").Exists())
	require.False(t, gjson.GetBytes(normalized, "input.0.content.0.namespace").Exists())
	require.False(t, gjson.GetBytes(normalized, "input.1.namespace").Exists())
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "input.0.sequence_number").Raw)
	require.Equal(t, "namespace stays in text", gjson.GetBytes(normalized, "input.0.content.0.text").String())
	require.Equal(t, "functions", gjson.GetBytes(normalized, "tools.0.namespace").String())
}
