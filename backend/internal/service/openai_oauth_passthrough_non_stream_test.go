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

// Regression tests for #7978: /v1/responses with stream=false must return a
// single JSON Responses object even though the ChatGPT OAuth upstream is
// always called with stream=true and answers with SSE.

const openAINonStreamRegressionSSE = "event: response.created\n" +
	`data: {"type":"response.created","response":{"id":"resp_7978","status":"in_progress","output":[]}}` + "\n\n" +
	"event: response.completed\n" +
	`data: {"type":"response.completed","response":{"id":"resp_7978","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}` + "\n\n"

func runOpenAIOAuthNonStreamRegression(t *testing.T, passthrough bool, requestBody string, upstreamSSE ...string) (*httptest.ResponseRecorder, *httpUpstreamRecorder, *OpenAIForwardResult) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("Accept", "application/json")

	sseBody := openAINonStreamRegressionSSE
	if len(upstreamSSE) > 0 {
		sseBody = upstreamSSE[0]
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_7978"}},
		Body:       io.NopCloser(strings.NewReader(sseBody)),
	}}
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
	account := &Account{
		ID:             7978,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra:          map[string]any{"openai_passthrough": passthrough},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.Forward(context.Background(), c, account, []byte(requestBody))
	require.NoError(t, err)
	require.NotNil(t, result)
	return rec, upstream, result
}

func requireSingleJSONResponsesObject(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"), "Content-Type=%q", rec.Header().Get("Content-Type"))
	var obj map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &obj), "body=%s", rec.Body.String())
	require.Equal(t, "resp_7978", obj["id"])
	require.Equal(t, "completed", obj["status"])
	require.Equal(t, "OK", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
}

func TestOpenAIGatewayService_OAuthPassthrough_StreamFalseReturnsJSON(t *testing.T) {
	rec, upstream, result := runOpenAIOAuthNonStreamRegression(t, true,
		`{"model":"gpt-5.4","instructions":"x","input":"Reply with exactly OK.","stream":false}`)

	// Upstream still requires stream=true.
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, result.Stream)
	requireSingleJSONResponsesObject(t, rec)
}

func TestOpenAIGatewayService_OAuthPassthrough_StreamTrueStillStreams(t *testing.T) {
	rec, upstream, result := runOpenAIOAuthNonStreamRegression(t, true,
		`{"model":"gpt-5.4","instructions":"x","input":"Reply with exactly OK.","stream":true}`)

	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.True(t, result.Stream)
	require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream"), "Content-Type=%q", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), "event: response.completed")
}

func TestOpenAIGatewayService_OAuthLegacy_StreamFalseReturnsJSONContentType(t *testing.T) {
	rec, upstream, result := runOpenAIOAuthNonStreamRegression(t, false,
		`{"model":"gpt-5.4","instructions":"x","input":"Reply with exactly OK.","stream":false}`)

	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, result.Stream)
	requireSingleJSONResponsesObject(t, rec)
}

const openAINonStreamIncompleteSSE = "event: response.created\n" +
	`data: {"type":"response.created","response":{"id":"resp_7978","status":"in_progress","output":[]}}` + "\n\n" +
	"event: response.output_text.delta\n" +
	`data: {"type":"response.output_text.delta","delta":"Hel"}` + "\n\n" +
	"event: response.incomplete\n" +
	`data: {"type":"response.incomplete","response":{"id":"resp_7978","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hel"}]}],"usage":{"input_tokens":5,"output_tokens":16,"total_tokens":21}}}` + "\n\n"

func TestOpenAIGatewayService_OAuthStreamFalseIncompleteReturnsJSON(t *testing.T) {
	for _, passthrough := range []bool{true, false} {
		rec, _, result := runOpenAIOAuthNonStreamRegression(t, passthrough,
			`{"model":"gpt-5.4","instructions":"x","input":"hi","max_output_tokens":16,"stream":false}`,
			openAINonStreamIncompleteSSE)

		require.False(t, result.Stream)
		require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"), "passthrough=%v Content-Type=%q", passthrough, rec.Header().Get("Content-Type"))
		var obj map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &obj), "passthrough=%v body=%s", passthrough, rec.Body.String())
		require.Equal(t, "resp_7978", obj["id"])
		require.Equal(t, "incomplete", obj["status"])
		require.Equal(t, "max_output_tokens", gjson.GetBytes(rec.Body.Bytes(), "incomplete_details.reason").String())
		require.Equal(t, "Hel", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
		require.Equal(t, 5, result.Usage.InputTokens)
		require.Equal(t, 16, result.Usage.OutputTokens)
	}
}

func TestOpenAIGatewayService_OAuthStreamFalseCancelledReturnsJSON(t *testing.T) {
	cancelledSSE := "event: response.cancelled\n" +
		`data: {"type":"response.cancelled","response":{"id":"resp_7978","object":"response","status":"cancelled","output":[],"usage":{"input_tokens":5,"output_tokens":0,"total_tokens":5}}}` + "\n\n"
	for _, passthrough := range []bool{true, false} {
		rec, _, _ := runOpenAIOAuthNonStreamRegression(t, passthrough,
			`{"model":"gpt-5.4","instructions":"x","input":"hi","stream":false}`, cancelledSSE)

		require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"), "passthrough=%v Content-Type=%q", passthrough, rec.Header().Get("Content-Type"))
		require.Equal(t, "cancelled", gjson.GetBytes(rec.Body.Bytes(), "status").String(), "passthrough=%v body=%s", passthrough, rec.Body.String())
	}
}
