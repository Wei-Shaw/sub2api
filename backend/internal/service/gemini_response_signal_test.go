//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDetectGeminiResponseSignal(t *testing.T) {
	prohibited := `{"candidates":[{"content":{},"finishMessage":"The model output could not be generated. This output contains sensitive words that violate Google's [Generative AI Prohibited Use policy](https://policies.google.com/terms/generative-ai/use-policy).","finishReason":"PROHIBITED_CONTENT"}],"usageMetadata":{"candidatesTokenCount":1289,"promptTokenCount":21775}}`

	tests := []struct {
		name        string
		payload     string
		wantHit     bool
		wantKind    geminiResponseSignalKind
		wantReason  string
		wantStatus  int
		wantMessage string
	}{
		{name: "empty", payload: "", wantHit: false},
		{name: "invalid json", payload: "data: {", wantHit: false},
		{name: "streaming chunk without finishReason", payload: `{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"}}]}`, wantHit: false},
		{name: "text mentioning error is not an envelope", payload: `{"candidates":[{"content":{"parts":[{"text":"an \"error\" happened in the story"}]}}]}`, wantHit: false},
		{name: "STOP", payload: `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`, wantHit: false},
		{name: "MAX_TOKENS", payload: `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"MAX_TOKENS"}]}`, wantHit: false},
		{name: "FINISH_REASON_UNSPECIFIED", payload: `{"candidates":[{"finishReason":"FINISH_REASON_UNSPECIFIED"}]}`, wantHit: false},
		{name: "OTHER is a normal stop", payload: `{"candidates":[{"content":{"parts":[{"text":"full answer"}]},"finishReason":"OTHER"}]}`, wantHit: false},
		{name: "TOO_MANY_TOOL_CALLS is a normal stop", payload: `{"candidates":[{"finishReason":"TOO_MANY_TOOL_CALLS"}]}`, wantHit: false},
		{name: "NO_IMAGE is a normal stop", payload: `{"candidates":[{"finishReason":"NO_IMAGE"}]}`, wantHit: false},
		{name: "unknown future finishReason is a normal stop", payload: `{"candidates":[{"finishReason":"SOME_NEW_REASON"}]}`, wantHit: false},
		{name: "countTokens body", payload: `{"totalTokens":12}`, wantHit: false},
		{name: "BLOCKED_REASON_UNSPECIFIED is not a block", payload: `{"promptFeedback":{"blockReason":"BLOCKED_REASON_UNSPECIFIED"}}`, wantHit: false},
		{name: "secondary candidate is ignored", payload: `{"candidates":[{"index":1,"finishReason":"SAFETY"}]}`, wantHit: false},
		{name: "error as string is not an envelope", payload: `{"error":"plain"}`, wantHit: false},
		{name: "error as number is not an envelope", payload: `{"error":123}`, wantHit: false},
		{
			name: "code assist wrapped event is unwrapped", payload: `{"response":{"candidates":[{"content":{},"finishReason":"SAFETY"}]},"traceId":"t"}`, wantHit: true,
			wantKind: geminiSignalContentFilter, wantReason: "SAFETY", wantStatus: http.StatusBadRequest,
		},
		{
			name: "PROHIBITED_CONTENT with finishMessage", payload: prohibited, wantHit: true,
			wantKind: geminiSignalContentFilter, wantReason: "PROHIBITED_CONTENT", wantStatus: http.StatusBadRequest,
			wantMessage: "Gemini content policy stop (finishReason=PROHIBITED_CONTENT): The model output could not be generated.",
		},
		{
			name: "SAFETY without finishMessage uses fallback text", payload: `{"candidates":[{"content":{},"finishReason":"SAFETY"}]}`, wantHit: true,
			wantKind: geminiSignalContentFilter, wantReason: "SAFETY", wantStatus: http.StatusBadRequest,
			wantMessage: "Gemini content policy stop (finishReason=SAFETY): Response stopped due to safety reasons.",
		},
		{
			name: "lowercase finishReason is normalized", payload: `{"candidates":[{"finishReason":"blocklist"}]}`, wantHit: true,
			wantKind: geminiSignalContentFilter, wantReason: "BLOCKLIST", wantStatus: http.StatusBadRequest,
		},
		{
			name: "explicit index 0 candidate is primary", payload: `{"candidates":[{"index":1,"finishReason":"STOP"},{"index":0,"finishReason":"SPII"}]}`, wantHit: true,
			wantKind: geminiSignalContentFilter, wantReason: "SPII", wantStatus: http.StatusBadRequest,
		},
		{
			name: "MALFORMED_FUNCTION_CALL is not a signal", payload: `{"candidates":[{"content":{},"finishReason":"MALFORMED_FUNCTION_CALL"}]}`, wantHit: false,
		},
		{
			name: "UNEXPECTED_TOOL_CALL is not a signal", payload: `{"candidates":[{"finishReason":"UNEXPECTED_TOOL_CALL"}]}`, wantHit: false,
		},
		{
			name: "promptFeedback blockReason", payload: `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT","safetyRatings":[]},"usageMetadata":{"promptTokenCount":10,"totalTokenCount":10}}`, wantHit: true,
			wantKind: geminiSignalPromptBlocked, wantReason: "PROHIBITED_CONTENT", wantStatus: http.StatusBadRequest,
			wantMessage: "Gemini content policy block (blockReason=PROHIBITED_CONTENT): prompt was blocked before generation",
		},
		{
			name: "google error envelope with numeric code", payload: `{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`, wantHit: true,
			wantKind: geminiSignalError, wantReason: "RESOURCE_EXHAUSTED", wantStatus: http.StatusTooManyRequests,
			wantMessage: "Resource has been exhausted (e.g. check quota).",
		},
		{
			name: "google error envelope without code maps status name", payload: `{"error":{"message":"Internal error encountered.","status":"INTERNAL"}}`, wantHit: true,
			wantKind: geminiSignalError, wantReason: "INTERNAL", wantStatus: http.StatusInternalServerError,
			wantMessage: "Internal error encountered.",
		},
		{
			name: "error envelope with out-of-range code maps status name", payload: `{"error":{"code":200,"message":"weird","status":"resource_exhausted"}}`, wantHit: true,
			wantKind: geminiSignalError, wantReason: "RESOURCE_EXHAUSTED", wantStatus: http.StatusTooManyRequests,
		},
		{
			name: "error envelope without status or code falls back to 502", payload: `{"error":{"message":"boom"}}`, wantHit: true,
			wantKind: geminiSignalError, wantReason: "UPSTREAM_ERROR", wantStatus: http.StatusBadGateway, wantMessage: "boom",
		},
		{
			name: "error takes precedence over finishReason", payload: `{"error":{"code":503,"status":"UNAVAILABLE","message":"busy"},"candidates":[{"finishReason":"SAFETY"}]}`, wantHit: true,
			wantKind: geminiSignalError, wantReason: "UNAVAILABLE", wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, hit := detectGeminiResponseSignal([]byte(tt.payload))
			require.Equal(t, tt.wantHit, hit)
			if !tt.wantHit {
				require.Equal(t, geminiSignalNone, sig.Kind)
				return
			}
			require.Equal(t, tt.wantKind, sig.Kind)
			require.Equal(t, tt.wantReason, sig.Reason)
			require.Equal(t, tt.wantStatus, sig.Status)
			if tt.wantMessage != "" {
				require.Contains(t, sig.Message, tt.wantMessage)
			}
			if tt.wantKind == geminiSignalError {
				require.NotEmpty(t, sig.Detail)
			} else {
				require.Empty(t, sig.Detail)
			}
		})
	}
}

func TestGeminiResponseSignalKindPrecedence(t *testing.T) {
	require.Greater(t, geminiSignalError, geminiSignalAbnormalStop)
	require.Greater(t, geminiSignalAbnormalStop, geminiSignalContentFilter)
	require.Greater(t, geminiSignalContentFilter, geminiSignalPromptBlocked)
	require.Greater(t, geminiSignalPromptBlocked, geminiSignalNone)
}

func TestDetectGeminiResponseSignalInBody(t *testing.T) {
	t.Run("json array picks the most severe element", func(t *testing.T) {
		body := `[{"candidates":[{"content":{"parts":[{"text":"a"}]}}]}
,
{"candidates":[{"content":{},"finishReason":"SAFETY"}]}
,
{"error":{"code":503,"status":"UNAVAILABLE","message":"busy"}}
]`
		sig, ok := detectGeminiResponseSignalInBody([]byte(body))
		require.True(t, ok)
		require.Equal(t, geminiSignalError, sig.Kind)
		require.Equal(t, "UNAVAILABLE", sig.Reason)
	})
	t.Run("json array without signal", func(t *testing.T) {
		_, ok := detectGeminiResponseSignalInBody([]byte(`[{"candidates":[{"content":{"parts":[{"text":"a"}]},"finishReason":"STOP"}]}]`))
		require.False(t, ok)
	})
	t.Run("code assist wrapped object is unwrapped", func(t *testing.T) {
		sig, ok := detectGeminiResponseSignalInBody([]byte(`{"response":{"candidates":[{"content":{},"finishReason":"PROHIBITED_CONTENT"}]},"traceId":"t"}`))
		require.True(t, ok)
		require.Equal(t, geminiSignalContentFilter, sig.Kind)
		require.Equal(t, "PROHIBITED_CONTENT", sig.Reason)
	})
	t.Run("plain object", func(t *testing.T) {
		sig, ok := detectGeminiResponseSignalInBody([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`))
		require.True(t, ok)
		require.Equal(t, geminiSignalPromptBlocked, sig.Kind)
	})
	t.Run("non json", func(t *testing.T) {
		_, ok := detectGeminiResponseSignalInBody([]byte(`<html>error</html>`))
		require.False(t, ok)
	})
}

func TestIsGeminiEmptyResponseBody(t *testing.T) {
	require.True(t, isGeminiEmptyResponseBody(nil))
	require.True(t, isGeminiEmptyResponseBody([]byte("  \n")))
	require.True(t, isGeminiEmptyResponseBody([]byte(`{}`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`{"candidates":[]}`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`{"candidates":[],"usageMetadata":{"promptTokenCount":3}}`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`{"candidates":[],"promptFeedback":{"safetyRatings":[]}}`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`{"totalTokens":12}`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`[]`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`[{}]`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`[{"candidates":[]},{}]`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`[{"candidates":[]},{"candidates":[{"content":{}}]}]`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`[1]`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`{"response":{}}`)))
	require.True(t, isGeminiEmptyResponseBody([]byte(`{"response":{"candidates":[]},"traceId":"t"}`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`{"response":{"totalTokens":3}}`)))
	require.False(t, isGeminiEmptyResponseBody([]byte(`not json`)))
}

func TestMarkOpsStreamErrorValue_RequestScopedSkipsUpstreamSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.7-flash:generateContent", nil)
		setOpsUpstreamError(c, http.StatusTooManyRequests, "earlier attempt was rate limited", "detail")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{UpstreamStatusCode: http.StatusTooManyRequests, Message: "earlier attempt was rate limited", SkipMonitoring: true})
		return c
	}

	scoped := newCtx()
	MarkOpsStreamErrorValue(scoped, OpsStreamError{ErrType: "invalid_request_error", Code: "SAFETY", Message: "m", IntendedStatus: http.StatusBadRequest, RequestScoped: true})
	got, ok := GetOpsStreamError(scoped)
	require.True(t, ok)
	require.Zero(t, got.UpstreamStatus)
	require.Empty(t, got.UpstreamMessage)
	require.Empty(t, got.UpstreamDetail)
	require.Nil(t, got.UpstreamErrors)
	require.False(t, got.SkipMonitoring)

	plain := newCtx()
	MarkOpsStreamErrorValue(plain, OpsStreamError{ErrType: "rate_limit_error", Code: "RESOURCE_EXHAUSTED", Message: "m", IntendedStatus: http.StatusTooManyRequests, CountTowardsSLA: true})
	got, ok = GetOpsStreamError(plain)
	require.True(t, ok)
	require.Equal(t, http.StatusTooManyRequests, got.UpstreamStatus)
	require.Equal(t, "earlier attempt was rate limited", got.UpstreamMessage)
	require.Len(t, got.UpstreamErrors, 1)
	require.True(t, got.SkipMonitoring)
}

func TestGeminiSSEFallbackBody(t *testing.T) {
	var fb geminiSSEFallbackBody
	fb.AddLine("")
	fb.AddLine("   ")
	fb.AddLine(": keepalive")
	require.Empty(t, fb.Bytes())
	fb.AddLine(`{"a":1}`)
	fb.AddLine("event: ping")
	require.Equal(t, "{\"a\":1}\nevent: ping\n", string(fb.Bytes()))
	require.False(t, fb.Truncated())

	var big geminiSSEFallbackBody
	big.AddLine(string(make([]byte, geminiSSEFallbackBodyLimit)))
	require.True(t, big.Truncated())
	require.Empty(t, big.Bytes())
	var nilBody *geminiSSEFallbackBody
	nilBody.AddLine("x")
	require.Nil(t, nilBody.Bytes())
	require.False(t, nilBody.Truncated())
}
