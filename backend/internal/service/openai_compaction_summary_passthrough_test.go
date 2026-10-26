package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func summaryPassthroughTestContext(path string, body []byte) (*gin.Context, *httptest.ResponseRecorder, *OpenAIGatewayService) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Set("api_key", &APIKey{UserID: 42})
	summaryService := &OpenAIGatewayService{compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...)}
	c.Request = c.Request.WithContext(WithOpenAICompactionSummaryService(c.Request.Context(), summaryService))
	return c, recorder, summaryService
}

func responsesSummaryJSON(text string) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"id": "resp_passthrough_summary", "object": "response", "status": "completed",
		"output": []any{map[string]any{
			"type": "message", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": text}},
		}},
		"usage": map[string]any{"input_tokens": 9, "output_tokens": 4, "total_tokens": 13},
	})
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func responsesSummarySSE(text string) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","output_index":0,"delta":"` + text + `"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_oauth_summary","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"req_oauth_summary"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAPIKeyPassthroughLegacySummaryKeepsOrdinaryTargetAndParameters(t *testing.T) {
	body := []byte(`{"model":"lab-summary-model","stream":false,"temperature":0.2,"input":[{"type":"message","role":"user","content":"summarize this history"}]}`)
	c, recorder, _ := summaryPassthroughTestContext("/v1/responses/compact", body)
	account := &Account{
		ID: 1301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test", "base_url": "https://passthrough.example.test",
			"compact_model_mapping": map[string]any{"lab-summary-model": "gpt-5.5"},
			"model_mapping":         map[string]any{"lab-summary-model": "mapped-should-not-be-used"},
		},
		Extra: map[string]any{
			"openai_passthrough":                true,
			OpenAICompactSummaryEnabledExtraKey: true,
		},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"lab-summary-model": {ID: "lab-summary-model", ContextWindow: 128_000},
	}})
	upstream := &httpUpstreamRecorder{resp: responsesSummaryJSON("legacy passthrough summary")}
	svc := &OpenAIGatewayService{
		cfg:                  &config.Config{Gateway: config.GatewayConfig{OpenAICompactModel: "gpt-5.5"}},
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, "https://passthrough.example.test/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "lab-summary-model", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, 0.2, gjson.GetBytes(upstream.lastBody, "temperature").Float())
	require.Equal(t, "compaction", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
	require.NotContains(t, recorder.Body.String(), "legacy passthrough summary")
}

func TestOAuthPassthroughV2SummaryBuffersUpstreamStreamAndFinalizes(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":true,"instructions":"keep going","prompt_cache_key":"cache-key","input":[{"type":"message","role":"user","content":"summarize this"},{"type":"compaction_trigger"}]}`)
	c, recorder, _ := summaryPassthroughTestContext("/v1/responses", body)
	MarkOpenAINativeCompactionV2(c)
	MarkOpenAICompactClientStream(c)
	account := &Account{
		ID: 1302, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra: map[string]any{
			"openai_passthrough":                        true,
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
			OpenAICompactSummaryEnabledExtraKey:         true,
		},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gpt-5.5": {ID: "gpt-5.5", ContextWindow: 128_000},
	}})
	upstream := &httpUpstreamRecorder{resp: responsesSummarySSE("oauth passthrough summary")}
	svc := &OpenAIGatewayService{
		cfg:                  &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool(), "OAuth upstream must still receive stream=true")
	require.Equal(t, gjson.False, gjson.GetBytes(upstream.lastBody, "store").Type)
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, int64(OpenAICompactionSummaryOutputTokenBudget), gjson.GetBytes(upstream.lastBody, "max_output_tokens").Int())
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.output_item.done"))
	require.Contains(t, recorder.Body.String(), "event: response.completed")
	require.NotContains(t, recorder.Body.String(), `"text":"oauth passthrough summary"`)
}

func TestOAuthPassthroughLegacySummaryKeepsStoreAndUpstreamStream(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":false,"instructions":"keep going","prompt_cache_key":"legacy-cache-key","input":[{"type":"message","role":"user","content":"summarize this history"}]}`)
	c, recorder, _ := summaryPassthroughTestContext("/v1/responses/compact", body)
	account := &Account{
		ID: 1304, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra: map[string]any{
			"openai_passthrough":                        true,
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
			OpenAICompactSummaryEnabledExtraKey:         true,
		},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"gpt-5.5": {ID: "gpt-5.5", ContextWindow: 128_000},
	}})
	upstream := &httpUpstreamRecorder{resp: responsesSummarySSE("oauth legacy summary")}
	svc := &OpenAIGatewayService{
		cfg:                  &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
		httpUpstream:         upstream,
		compactionSummaryKey: append([]byte(nil), testOpenAICompactionSummaryKey()...),
	}

	result, err := svc.Forward(c.Request.Context(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 1)
	require.NotContains(t, upstream.lastReq.URL.Path, "/compact")
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, gjson.False, gjson.GetBytes(upstream.lastBody, "store").Type)
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "compaction", gjson.GetBytes(recorder.Body.Bytes(), "output.0.type").String())
	require.NotContains(t, recorder.Body.String(), `"text":"oauth legacy summary"`)
}

func TestGatewayForwardAsResponsesRejectsSummaryUsingDifferentFinalVertexWindow(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","input":"history","stream":false,"max_output_tokens":8192}`)
	c, recorder, _ := summaryPassthroughTestContext("/v1/responses", body)
	markOpenAICompactionSummaryExecution(c, openAICompactionKindLegacy)
	account := &Account{
		ID: 1303, Platform: PlatformAnthropic, Type: AccountTypeServiceAccount,
		Credentials: map[string]any{"project_id": "project-test"},
	}
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"claude-sonnet-4-5":          {ID: "claude-sonnet-4-5", ContextWindow: 128_000},
		"claude-sonnet-4-5@20250929": {ID: "claude-sonnet-4-5@20250929", ContextWindow: 100},
	}})
	upstream := &anthropicHTTPUpstreamRecorder{}
	svc := &GatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	_, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
	require.Error(t, err)
	require.Nil(t, upstream.lastReq, "different final-target window must reject before any upstream request")
	require.Contains(t, recorder.Body.String(), "compaction_summary_unsupported")
}

func TestAntigravitySummaryRejectsUsingThinkingSuffixWindowBeforeUpstream(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","input":"history","stream":false,"reasoning":{"effort":"high"},"max_output_tokens":8192}`)
	c, recorder, _ := summaryPassthroughTestContext("/v1/responses", body)
	markOpenAICompactionSummaryExecution(c, openAICompactionKindLegacy)
	account := newAntigravityCompatAccount(AccountTypeOAuth)
	account.SetUpstreamModelMetadataSnapshot(UpstreamModelMetadataSnapshot{Models: map[string]UpstreamModelMetadata{
		"claude-sonnet-4-5":          {ID: "claude-sonnet-4-5", ContextWindow: 128_000},
		"claude-sonnet-4-5-thinking": {ID: "claude-sonnet-4-5-thinking", ContextWindow: 100},
	}})
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{antigravityCompatSuccessResponse()}}
	svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, upstream)

	_, err := svc.ForwardAsResponses(context.Background(), c, account, body, nil)
	require.Error(t, err)
	require.Zero(t, upstream.callCount, "thinking-suffixed final target must reject before upstream")
	require.Contains(t, recorder.Body.String(), "compaction_summary_unsupported")
}
