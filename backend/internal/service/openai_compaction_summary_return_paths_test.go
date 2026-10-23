//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newCompactionSummaryReturnContext(t *testing.T, kind string) (*gin.Context, *httptest.ResponseRecorder, *OpenAIGatewayService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: 42})
	markOpenAICompactionSummaryExecution(c, kind)
	if kind == openAICompactionKindV2 {
		MarkOpenAICompactClientStream(c)
	}
	summaryService := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...)}
	c.Request = c.Request.WithContext(WithOpenAICompactionSummaryService(c.Request.Context(), summaryService))
	return c, recorder, summaryService
}

func anthropicCompactionSummarySSE(text string) string {
	return strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_summary","type":"message","role":"assistant","model":"claude-sonnet-4.5","content":[],"usage":{"input_tokens":11}}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")
}

func assertCompactionSummaryReturn(t *testing.T, kind string, recorder *httptest.ResponseRecorder, rawSummary string) {
	t.Helper()
	require.NotContains(t, recorder.Body.String(), `"text":"`+rawSummary+`"`)

	var sealed string
	if kind == openAICompactionKindV2 {
		require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
		require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.output_item.done"))
		require.Contains(t, recorder.Body.String(), "event: response.completed")
		sealed = compactionCiphertextFromSSE(t, recorder.Body.String())
	} else {
		require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
		require.Len(t, gjson.GetBytes(recorder.Body.Bytes(), "output").Array(), 1)
		require.Equal(t, "compaction", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
		sealed = gjson.GetBytes(recorder.Body.Bytes(), "output.0.encrypted_content").String()
	}
	require.NotEmpty(t, sealed)
	scope, err := NewOpenAICompactionOwnerScope(42, nil)
	require.NoError(t, err)
	summary, err := OpenOpenAICompactionSummary(testOpenAICompactionSummaryKey(), scope, sealed)
	require.NoError(t, err)
	require.Equal(t, rawSummary, summary)
}

func TestNativeAnthropicSummaryReturnFinalizesBeforeWriting(t *testing.T) {
	for _, kind := range []string{openAICompactionKindV2, openAICompactionKindLegacy} {
		t.Run(kind, func(t *testing.T) {
			c, recorder, svc := newCompactionSummaryReturnContext(t, kind)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"req_native_anthropic"}},
				Body:       io.NopCloser(strings.NewReader(anthropicCompactionSummarySSE("native anthropic summary"))),
			}

			result, err := svc.handleResponsesBufferedFromNativeAnthropic(
				resp, c, "public-model", "mapped-model", "mapped-model", nil, time.Now(), apicompat.ResponsesClientToolMapping{},
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			assertCompactionSummaryReturn(t, kind, recorder, "native anthropic summary")
		})
	}
}

func TestGatewayForwardAsResponsesSummaryReturnFinalizesBeforeWriting(t *testing.T) {
	for _, kind := range []string{openAICompactionKindV2, openAICompactionKindLegacy} {
		t.Run(kind, func(t *testing.T) {
			c, recorder, _ := newCompactionSummaryReturnContext(t, kind)
			upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"req_gateway_anthropic"}},
				Body:       io.NopCloser(strings.NewReader(anthropicCompactionSummarySSE("gateway anthropic summary"))),
			}}
			svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			body := []byte(`{"model":"public-summary","input":"history","stream":false,"max_output_tokens":8192}`)
			account := &Account{
				ID: 1201, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				Credentials: map[string]any{
					"api_key":       "anthropic-key",
					"base_url":      "https://anthropic.example.test",
					"model_mapping": map[string]any{"public-summary": "mapped-summary"},
				},
			}
			account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
				"mapped-summary": {ID: "mapped-summary", ContextWindow: 128_000},
			}})

			result, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.NotContains(t, upstream.lastReq.URL.Path, "/compact")
			require.Equal(t, "mapped-summary", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, int64(OpenAICompactionSummaryOutputTokenBudget), gjson.GetBytes(upstream.lastBody, "max_tokens").Int())
			require.Equal(t, 11, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			assertCompactionSummaryReturn(t, kind, recorder, "gateway anthropic summary")
		})
	}
}

func TestAntigravitySummaryReturnFinalizesBeforeWriting(t *testing.T) {
	for _, kind := range []string{openAICompactionKindV2, openAICompactionKindLegacy} {
		t.Run(kind, func(t *testing.T) {
			c, recorder, _ := newCompactionSummaryReturnContext(t, kind)
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
			svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)
			body := []byte(`{"model":"gemini-3.1-pro-high","input":"history","stream":false}`)

			account := newAntigravityCompatAccount(AccountTypeOAuth)
			account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
				"gemini-3.1-pro-high": {ID: "gemini-3.1-pro-high", ContextWindow: 128_000},
			}})
			result, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 8, result.Usage.InputTokens)
			require.Equal(t, 3, result.Usage.OutputTokens)
			assertCompactionSummaryReturn(t, kind, recorder, "ok")
		})
	}
}
