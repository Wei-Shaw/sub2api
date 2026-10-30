package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func newCompactBodySignalTestContext(t *testing.T, path string, body []byte) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func TestNormalizeOpenAIResponsesCompactRequest_RemoteV2StaysOnResponses(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	tests := []struct {
		name       string
		betaHeader string
		userAgent  string
	}{
		{name: "headerless"},
		{name: "unrelated_header", betaHeader: "responses_websockets_v2"},
		{name: "wrong_case_header", betaHeader: "REMOTE_COMPACTION_V2"},
		{name: "declared_header", betaHeader: "remote_compaction_v2"},
		{name: "codex_cli_user_agent", userAgent: "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color"},
		{name: "codex_desktop_user_agent", userAgent: "Codex Desktop/0.139.0 (Mac OS X 14; arm64) unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{
				"model":"gpt-5.6-sol",
				"stream":true,
				"store":true,
				"prompt_cache_key":"pck-signal-1",
				"reasoning":{"effort":"max","context":"all_turns"},
				"input":[
					{"type":"message","role":"user","content":"hello"},
					{"type":"compaction_trigger"}
				]
			}`)
			c := newCompactBodySignalTestContext(t, "/v1/responses", body)
			if tt.betaHeader != "" {
				c.Request.Header.Set("x-codex-beta-features", tt.betaHeader)
			}
			if tt.userAgent != "" {
				c.Request.Header.Set("User-Agent", tt.userAgent)
			}

			normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
			require.True(t, ok)

			require.Equal(t, "/v1/responses", c.Request.URL.Path)
			require.False(t, isOpenAILegacyCompactPath(c))
			require.Equal(t, body, normalized)
			require.True(t, gjson.GetBytes(normalized, "stream").Bool())
			require.True(t, gjson.GetBytes(normalized, "store").Bool())
			require.Equal(t, "pck-signal-1", gjson.GetBytes(normalized, "prompt_cache_key").String())
			require.Equal(t, "max", gjson.GetBytes(normalized, "reasoning.effort").String())
			require.Equal(t, "all_turns", gjson.GetBytes(normalized, "reasoning.context").String())
			legacyCompact := service.IsOpenAIResponsesCompactPath(c)
			nativeV2 := isBareOpenAIResponsesPath(c) && isOpenAIRemoteCompactionV2Request(normalized)
			require.False(t, legacyCompact)
			require.True(t, nativeV2)
			require.Equal(t, service.OpenAIEndpointCapabilityResponses,
				openAIResponsesRequiredCapabilityForRequest(false, nativeV2 || legacyCompact, service.PlatformOpenAI))

			reqStream, streamOK := parseOpenAICompatibleStream(normalized)
			require.True(t, streamOK)
			require.True(t, reqStream)

			_, seedExists := c.Get(service.OpenAICompactSessionSeedKeyForTest())
			require.False(t, seedExists)
			_, streamMarkerExists := c.Get(service.OpenAICompactClientStreamKeyForTest())
			require.False(t, streamMarkerExists)
		})
	}
}

func TestNormalizeOpenAIResponsesCompactRequest_RemoteV2PathAliasesStayOnResponses(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	for _, path := range []string{
		"/v1/responses/",
		"/openai/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
	} {
		t.Run(path, func(t *testing.T) {
			c := newCompactBodySignalTestContext(t, path, body)

			normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
			require.True(t, ok)
			require.Equal(t, path, c.Request.URL.Path)
			require.Equal(t, body, normalized)
			legacyCompact := service.IsOpenAIResponsesCompactPath(c)
			nativeV2 := isBareOpenAIResponsesPath(c) && isOpenAIRemoteCompactionV2Request(normalized)
			require.False(t, legacyCompact)
			require.True(t, nativeV2)
			require.Equal(t, service.OpenAIEndpointCapabilityResponses,
				openAIResponsesRequiredCapabilityForRequest(false, nativeV2 || legacyCompact, service.PlatformOpenAI))
		})
	}
}

func TestOpenAIResponsesCompactionRoutingFlags(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	tests := []struct {
		name                string
		body                []byte
		path                string
		wantLegacyBefore    bool
		wantNativeBefore    bool
		wantLegacyAfter     bool
		wantNativeAfter     bool
		wantCapabilityAfter service.OpenAIEndpointCapability
		wantPathAfter       string
		wantBodyUnchanged   bool
	}{
		{
			name:                "native_v2_stream_trigger",
			body:                []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`),
			path:                "/v1/responses",
			wantLegacyBefore:    false,
			wantNativeBefore:    true,
			wantLegacyAfter:     false,
			wantNativeAfter:     true,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityResponses,
			wantPathAfter:       "/v1/responses",
			wantBodyUnchanged:   true,
		},
		{
			name:                "native_v2_without_trigger",
			body:                []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`),
			path:                "/v1/responses",
			wantLegacyBefore:    false,
			wantNativeBefore:    false,
			wantLegacyAfter:     false,
			wantNativeAfter:     false,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityChatCompletions,
			wantPathAfter:       "/v1/responses",
			wantBodyUnchanged:   true,
		},
		{
			name:                "explicit_compact",
			body:                []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`),
			path:                "/v1/responses/compact",
			wantLegacyBefore:    true,
			wantNativeBefore:    false,
			wantLegacyAfter:     true,
			wantNativeAfter:     false,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityResponses,
			wantPathAfter:       "/v1/responses/compact",
		},
		{
			name:                "nested_compact",
			body:                []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`),
			path:                "/v1/responses/compact/detail",
			wantLegacyBefore:    true,
			wantNativeBefore:    false,
			wantLegacyAfter:     true,
			wantNativeAfter:     false,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityResponses,
			wantPathAfter:       "/v1/responses/compact/detail",
		},
		{
			name:                "responses_subpath_with_native_signal",
			body:                []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`),
			path:                "/v1/responses/resp_123/responses",
			wantLegacyBefore:    false,
			wantNativeBefore:    false,
			wantLegacyAfter:     false,
			wantNativeAfter:     false,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityChatCompletions,
			wantPathAfter:       "/v1/responses/resp_123/responses",
			wantBodyUnchanged:   true,
		},
		{
			name:                "stream_false_promotes",
			body:                []byte(`{"model":"gpt-5.6-sol","stream":false,"input":[{"type":"compaction_trigger"}]}`),
			path:                "/v1/responses",
			wantLegacyBefore:    false,
			wantNativeBefore:    false,
			wantLegacyAfter:     true,
			wantNativeAfter:     false,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityResponses,
			wantPathAfter:       "/v1/responses/compact",
		},
		{
			name:                "stream_absent_promotes",
			body:                []byte(`{"model":"gpt-5.6-sol","input":[{"type":"compaction_trigger"}]}`),
			path:                "/v1/responses",
			wantLegacyBefore:    false,
			wantNativeBefore:    false,
			wantLegacyAfter:     true,
			wantNativeAfter:     false,
			wantCapabilityAfter: service.OpenAIEndpointCapabilityResponses,
			wantPathAfter:       "/v1/responses/compact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCompactBodySignalTestContext(t, tt.path, tt.body)
			legacyBefore := service.IsOpenAIResponsesCompactPath(c)
			nativeBefore := isBareOpenAIResponsesPath(c) && isOpenAIRemoteCompactionV2Request(tt.body)
			require.Equal(t, tt.wantLegacyBefore, legacyBefore)
			require.Equal(t, tt.wantNativeBefore, nativeBefore)
			normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), tt.body)
			require.True(t, ok)
			require.Equal(t, tt.wantPathAfter, c.Request.URL.Path)
			legacyAfter := service.IsOpenAIResponsesCompactPath(c)
			nativeAfter := isBareOpenAIResponsesPath(c) && isOpenAIRemoteCompactionV2Request(normalized)
			require.Equal(t, tt.wantLegacyAfter, legacyAfter)
			require.Equal(t, tt.wantNativeAfter, nativeAfter)
			require.Equal(t, tt.wantCapabilityAfter,
				openAIResponsesRequiredCapabilityForRequest(false, nativeAfter || legacyAfter, service.PlatformOpenAI))
			if tt.wantBodyUnchanged {
				require.Equal(t, tt.body, normalized)
			}
		})
	}
}

func TestNormalizeOpenAIResponsesCompactRequest_BodySignalTrailingSlashPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
}

func TestNormalizeOpenAIResponsesCompactRequest_CodexDirectAliasPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/backend-api/codex/responses", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/backend-api/codex/responses/compact", c.Request.URL.Path)
}

func TestNormalizeOpenAIResponsesCompactRequest_NonRemoteV2BodySignalPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	tests := []struct {
		name       string
		body       []byte
		betaHeader string
		wantMarked bool
	}{
		{
			name: "stream_false_headerless",
			body: []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"compaction_trigger"}]}`),
		},
		{
			name: "stream_absent_headerless",
			body: []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`),
		},
		{
			name:       "stream_false_declared_header",
			body:       []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"compaction_trigger"}]}`),
			betaHeader: "remote_compaction_v2",
		},
		{
			name:       "stream_absent_wrong_case_header",
			body:       []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`),
			betaHeader: "REMOTE_COMPACTION_V2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newCompactBodySignalTestContext(t, "/v1/responses", tt.body)
			if tt.betaHeader != "" {
				c.Request.Header.Set("x-codex-beta-features", tt.betaHeader)
			}

			normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), tt.body)
			require.True(t, ok)
			require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
			require.False(t, gjson.GetBytes(normalized, "stream").Exists())

			marked, exists := c.Get(service.OpenAICompactClientStreamKeyForTest())
			require.Equal(t, tt.wantMarked, exists)
			if tt.wantMarked {
				require.Equal(t, true, marked)
			}
		})
	}
}

func TestNormalizeOpenAIResponsesCompactRequest_NoTriggerUntouched(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.False(t, isOpenAILegacyCompactPath(c))
	require.Equal(t, body, normalized)
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
}

func TestNormalizeOpenAIResponsesCompactRequest_PathBasedNoDoubleSuffix(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"store":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/compact", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
}

func TestNormalizeOpenAIResponsesCompactRequest_SubpathNotPromoted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/resp_123/cancel", body)

	normalized, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	require.Equal(t, "/v1/responses/resp_123/cancel", c.Request.URL.Path)
	require.Equal(t, body, normalized)
}

// path-based compact（Codex v1 unary 协议）即使 body 带 stream:true 也不标记，
// 保持 JSON 写回行为不变。
func TestNormalizeOpenAIResponsesCompactRequest_PathBasedStreamTrueNotMarked(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
	c := newCompactBodySignalTestContext(t, "/v1/responses/compact", body)

	_, ok := h.normalizeOpenAIResponsesCompactRequest(c, zap.NewNop(), body)
	require.True(t, ok)
	_, exists := c.Get(service.OpenAICompactClientStreamKeyForTest())
	require.False(t, exists)
}
