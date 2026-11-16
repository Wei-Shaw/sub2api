package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func oauthInputMetadataRequestBody(imageBytes, metadataItems int) []byte {
	var body bytes.Buffer
	body.Grow(imageBytes + metadataItems*200 + 512)
	body.WriteString(`{"model":"gpt-6.1-sol","stream":true,"store":false,"instructions":"Review this image.","input":[`)
	for index := 0; index < metadataItems; index++ {
		fmt.Fprintf(&body, `{"role":"user","content":"history %d","internal_chat_message_metadata_passthrough":{"content_item_kinds":["text"]}},`, index)
	}
	body.WriteString(`{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,`)
	body.Write(bytes.Repeat([]byte{'A'}, imageBytes))
	body.WriteString(`"}]}],"opaque":9007199254740993}`)
	return body.Bytes()
}

func BenchmarkOpenAIOAuthInputMetadata(b *testing.B) {
	for _, imageBytes := range []int{16 << 20, 112 << 20} {
		b.Run(fmt.Sprintf("%dMiB_50items", imageBytes>>20), func(b *testing.B) {
			body := oauthInputMetadataRequestBody(imageBytes, 50)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
				if err != nil || !changed || len(out) >= len(body) {
					b.Fatalf("请求元数据清理结果：changed=%v len=%d err=%v", changed, len(out), err)
				}
				runtime.KeepAlive(out)
			}
		})
	}
}

func TestOAuthInputMetadataAllocationsScaleWithRequest(t *testing.T) {
	body := oauthInputMetadataRequestBody(16<<20, 50)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gjson.ValidBytes(out))
	allocated := after.TotalAlloc - before.TotalAlloc
	require.Less(t, allocated, uint64(3*len(body)), "清理多条元数据时分配随请求体积增长：allocated=%d body=%d", allocated, len(body))
	runtime.KeepAlive(out)
}

func TestOAuthInputMetadataPreservesRawItemsAndDuplicateFields(t *testing.T) {
	const field = "internal_chat_message_metadata_passthrough"
	for _, fixture := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "duplicate input keeps first occurrence behavior",
			body: ` {"input":[{"content":"first","internal_chat_message_metadata_passthrough":true}],"input":[{"content":"second","internal_chat_message_metadata_passthrough":true}],"opaque":9007199254740993} `,
			want: ` {"input":[{"content":"first"}],"input":[{"content":"second","internal_chat_message_metadata_passthrough":true}],"opaque":9007199254740993} `,
		},
		{
			name: "duplicate metadata keeps single deletion behavior",
			body: `{"input":[{"internal_chat_message_metadata_passthrough":1,"internal_chat_message_metadata_passthrough":2,"opaque":9007199254740993}]}`,
			want: `{"input":[{"internal_chat_message_metadata_passthrough":2,"opaque":9007199254740993}]}`,
		},
		{
			name: "escaped keys and opaque fields keep original spelling",
			body: `{"in\u0070ut":[null,"keep",42,{"internal_chat_message_metadata_\u0070assthrough":null,"opaque":9007199254740993,"opaque":2,"content":{"internal_chat_message_metadata_passthrough":true}}],"tail":"keep"}`,
			want: `{"in\u0070ut":[null,"keep",42,{"opaque":9007199254740993,"opaque":2,"content":{"internal_chat_message_metadata_passthrough":true}}],"tail":"keep"}`,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := []byte(fixture.body)
			out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, fixture.want, string(out))
			require.Equal(t, fixture.body, string(body), "原始请求保留完整内容")
		})
	}

	body := oauthInputMetadataRequestBody(2<<20, 2)
	original := bytes.Clone(body)
	image := gjson.GetBytes(body, "input.2.content.0.image_url").String()
	out, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, original, body)
	require.Equal(t, image, gjson.GetBytes(out, "input.2.content.0.image_url").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(out, "opaque").Raw)
	require.False(t, gjson.GetBytes(out, "input.0."+field).Exists())
	require.False(t, gjson.GetBytes(out, "input.1."+field).Exists())

	again, changed, err := normalizeOpenAIOAuthResponsesCompatibilityBody(out)
	require.NoError(t, err)
	require.False(t, changed)
	require.True(t, bytes.Equal(out, again))
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
