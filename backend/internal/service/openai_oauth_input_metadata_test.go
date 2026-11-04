package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOAuthInputInternalMetadata(t *testing.T) {
	const field = "internal_chat_message_metadata_passthrough"
	// The identically named field in user content must not be recursively stripped.
	const body = `{"model":"gpt-5.5","input":[{"role":"user","content":[{"type":"input_text","text":"hello","internal_chat_message_metadata_passthrough":{"keep":true}}],"internal_chat_message_metadata_passthrough":{"content_item_kinds":["text"]}},{"type":"function_call","call_id":"fc_123","name":"echo","arguments":"{\"internal_chat_message_metadata_passthrough\":true}","internal_chat_message_metadata_passthrough":null},{"type":"function_call_output","call_id":"fc_123","output":"result"}],"internal_chat_message_metadata_passthrough":{"keep":true}}`
	tests := []struct {
		name      string
		normalize func([]byte) ([]byte, bool, error)
		strip     bool
	}{
		{"transformed OAuth", func(b []byte) ([]byte, bool, error) {
			var req map[string]any
			if err := json.Unmarshal(b, &req); err != nil {
				return nil, false, err
			}
			result := applyCodexOAuthTransform(req, false, false)
			out, err := json.Marshal(req)
			return out, result.Modified, err
		}, true},
		{"OAuth passthrough", func(b []byte) ([]byte, bool, error) { return normalizeOpenAIPassthroughOAuthBody(b, false) }, true},
		{"OAuth compact", func(b []byte) ([]byte, bool, error) { return normalizeOpenAIPassthroughOAuthBody(b, true) }, true},
		{"OAuth websocket", func(b []byte) ([]byte, bool, error) {
			return normalizeOpenAIResponsesWebSocketCompatibilityBody(b, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, false)
		}, true},
		{"API key websocket", func(b []byte) ([]byte, bool, error) {
			return normalizeOpenAIResponsesWebSocketCompatibilityBody(b, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, false)
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := tt.normalize([]byte(body))
			require.NoError(t, err)
			require.Equal(t, !tt.strip, gjson.GetBytes(out, "input.0."+field).Exists())
			require.Equal(t, !tt.strip, gjson.GetBytes(out, "input.1."+field).Exists())
			require.Equal(t, "hello", gjson.GetBytes(out, "input.0.content.0.text").String())
			require.True(t, gjson.GetBytes(out, "input.0.content.0."+field+".keep").Bool())
			require.True(t, gjson.GetBytes(out, field+".keep").Bool())
			require.Equal(t, `{"internal_chat_message_metadata_passthrough":true}`, gjson.GetBytes(out, "input.1.arguments").String())
			require.Equal(t, "fc_123", gjson.GetBytes(out, "input.1.call_id").String())
			require.Equal(t, "result", gjson.GetBytes(out, "input.2.output").String())
		})
	}
}

func TestOAuthInputInternalMetadataCompatibility(t *testing.T) {
	for _, body := range []string{
		`{"input":"hello"}`,
		`{"input":{"internal_chat_message_metadata_passthrough":true}}`,
		`{"input":[null,"hello",{"role":"user","content":"hello"}]}`,
		`{"input":[{"content":{"internal_chat_message_metadata_passthrough":true}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody([]byte(body))
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, string(out))
			var req map[string]any
			require.NoError(t, json.Unmarshal([]byte(body), &req))
			require.False(t, normalizeOpenAIOAuthResponsesCompatibilityFields(req))
		})
	}
	// Legacy prompt aliases and later-turn payloads use the same boundary.
	body := []byte(`{"prompt":[{"role":"user","content":"hello","internal_chat_message_metadata_passthrough":{}}],"previous_response_id":"resp_123"}`)
	out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "input.0.internal_chat_message_metadata_passthrough").Exists())
	require.Equal(t, "resp_123", gjson.GetBytes(out, "previous_response_id").String())
	var req map[string]any
	require.NoError(t, json.Unmarshal(body, &req))
	require.True(t, normalizeOpenAIOAuthResponsesCompatibilityFields(req))
	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.NotContains(t, input[0], "internal_chat_message_metadata_passthrough")
}
