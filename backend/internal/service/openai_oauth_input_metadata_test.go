package service

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func oauthInputMetadataHistory(items, textBytes, metadataEvery int) []byte {
	var body strings.Builder
	body.WriteString(`{"model":"gpt-5.5","opaque":9007199254740993,"input":[`)
	text := strings.Repeat("x", textBytes)
	for i := 0; i < items; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`{"role":"user","content":"`)
		body.WriteString(text)
		body.WriteByte('"')
		if metadataEvery > 0 && i%metadataEvery == 0 {
			body.WriteString(`,"internal_chat_message_metadata_passthrough":{"content_item_kinds":["text"]}`)
		}
		body.WriteByte('}')
	}
	body.WriteString(`],"tail":{"keep":true}}`)
	return []byte(body.String())
}

func TestOAuthInputInternalMetadataLargeHistory(t *testing.T) {
	for _, fixture := range []struct {
		name          string
		metadataEvery int
	}{
		{name: "all", metadataEvery: 1},
		{name: "sparse", metadataEvery: 32},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := oauthInputMetadataHistory(128, 16<<10, fixture.metadataEvery)
			original := bytes.Clone(body)
			out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, original, body, "normalization must not mutate caller-owned history")
			require.True(t, gjson.ValidBytes(out))
			require.Equal(t, int64(128), gjson.GetBytes(out, "input.#").Int())
			require.NotContains(t, string(out), "internal_chat_message_metadata_passthrough")
			require.Equal(t, "9007199254740993", gjson.GetBytes(out, "opaque").Raw)
			require.True(t, gjson.GetBytes(out, "tail.keep").Bool())
			text := strings.Repeat("x", 16<<10)
			for _, item := range gjson.GetBytes(out, "input").Array() {
				require.Equal(t, "user", item.Get("role").String())
				require.Equal(t, text, item.Get("content").String())
			}
		})
	}
}

func TestOAuthInputInternalMetadataPreservesRawContent(t *testing.T) {
	const body = ` {"opaque":9007199254740993,"inpu\u0074":[null,42,"keep",{"type":"function_call_output","call_id":"call_keep","output":[{"type":"input_image","image_url":"https://example.com/image.png","internal_chat_message_metadata_passthrough":{"keep":true}}],"internal_chat_message_metadata_passthrou\u0067h":null},{"role":"user","content":"internal_chat_message_metadata_passthrough"}],"tail" : "unchanged" } `
	out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody([]byte(body))
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, bytes.HasPrefix(out, []byte(` {"opaque":9007199254740993,"inpu\u0074":`)))
	require.True(t, bytes.HasSuffix(out, []byte(`,"tail" : "unchanged" } `)))
	require.Equal(t, `[null,42,"keep",{"type":"function_call_output","call_id":"call_keep","output":[{"type":"input_image","image_url":"https://example.com/image.png","internal_chat_message_metadata_passthrough":{"keep":true}}]},{"role":"user","content":"internal_chat_message_metadata_passthrough"}]`, gjson.GetBytes(out, "input").Raw)
}

func TestOAuthInputInternalMetadataUnchangedHistory(t *testing.T) {
	body := oauthInputMetadataHistory(128, 16<<10, 0)
	original := bytes.Clone(body)
	out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, original, out)
	require.Same(t, &body[0], &out[0], "unchanged history should retain the caller's buffer")
}

func BenchmarkOAuthInputInternalMetadataHistory(b *testing.B) {
	for _, fixture := range []struct {
		name          string
		items         int
		metadataEvery int
	}{
		{name: "1_item", items: 1, metadataEvery: 1},
		{name: "32_items", items: 32, metadataEvery: 1},
		{name: "128_items", items: 128, metadataEvery: 1},
		{name: "128_items_sparse", items: 128, metadataEvery: 32},
		{name: "128_items_unchanged", items: 128},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			body := oauthInputMetadataHistory(fixture.items, 16<<10, fixture.metadataEvery)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
				if err != nil || changed != (fixture.metadataEvery > 0) {
					b.Fatalf("changed=%v err=%v", changed, err)
				}
				runtime.KeepAlive(out)
			}
		})
	}
}

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
