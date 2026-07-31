package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type transparentTrackingReadCloser struct {
	reader      io.Reader
	bytesRead   int
	maxReadSize int
}

func (r *transparentTrackingReadCloser) Read(p []byte) (int, error) {
	if len(p) > r.maxReadSize {
		r.maxReadSize = len(p)
	}
	n, err := r.reader.Read(p)
	r.bytesRead += n
	return n, err
}

func (r *transparentTrackingReadCloser) Close() error { return nil }

func newOpenAITransparentPassthroughTestAccount() *Account {
	return &Account{
		ID:             125,
		Name:           "openai-transparent",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
		Extra:          map[string]any{"openai_passthrough": true},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}
}

func newOpenAITransparentPassthroughTestContext(path, userAgent string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", userAgent)
	return c, recorder
}

func TestShouldUseOfficialCodexResponsesTransparentPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := newOpenAITransparentPassthroughTestAccount()

	c, _ := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	require.True(t, shouldUseOfficialCodexResponsesTransparentPassthrough(c, account))

	c, _ = newOpenAITransparentPassthroughTestContext("/v1/responses/compact", "codex_cli_rs/0.145.0")
	require.False(t, shouldUseOfficialCodexResponsesTransparentPassthrough(c, account))

	c, _ = newOpenAITransparentPassthroughTestContext("/v1/responses", "curl/8.0")
	require.False(t, shouldUseOfficialCodexResponsesTransparentPassthrough(c, account))

	c, _ = newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	account.Extra = nil
	require.False(t, shouldUseOfficialCodexResponsesTransparentPassthrough(c, account))

	account = newOpenAITransparentPassthroughTestAccount()
	account.Credentials[openAIAuthModeCredentialKey] = OpenAIAuthModeAgentIdentity
	require.False(t, shouldUseOfficialCodexResponsesTransparentPassthrough(c, account))
}

func TestOpenAIGatewayService_OfficialCodexPassthroughPreservesRequestAndSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{
  "model":"gpt-5.6-sol",
  "stream":true,
  "store":false,
  "tools":[{"type":"function","namespace":"collaboration","name":"list_agents","parameters":{"type":"object"}}],
  "tool_choice":{"type":"function","namespace":"collaboration","name":"list_agents"},
  "input":[{"type":"function_call","call_id":"call_1","namespace":"collaboration","name":"list_agents","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"[]"}]
}`)
	upstreamBody := "event: response.output_item.added\r\n" +
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"namespace\":\"collaboration\",\"name\":\"list_agents\",\"call_id\":\"call_2\",\"arguments\":\"{}\"}}\r\n\r\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\r\n\r\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream; charset=utf-8"},
			"Connection":   []string{"close, X-Hop"},
			"Set-Cookie":   []string{"upstream-secret=1"},
			"X-Hop":        []string{"must-not-be-forwarded"},
			"X-Upstream":   []string{"preserved"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, requestBody, upstream.lastBody)
	require.Equal(t, upstreamBody, recorder.Body.String())
	require.Equal(t, "preserved", recorder.Header().Get("X-Upstream"))
	require.Empty(t, recorder.Header().Get("Connection"))
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
	require.Empty(t, recorder.Header().Get("X-Hop"))
}

func TestOpenAIGatewayService_OfficialCodexPassthroughDefaultConfigUsesFixedReaderBuffer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	upstreamBody := &transparentTrackingReadCloser{reader: strings.NewReader(
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_small\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
			"data: [DONE]\n\n",
	)}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       upstreamBody,
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, openAITransparentSSEReaderBufferSize, upstreamBody.maxReadSize)
	require.Less(t, upstreamBody.maxReadSize, defaultMaxLineSize)
}

func TestOpenAIGatewayService_OfficialCodexPassthroughAppliesFastPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"service_tier":"default","input":"test"}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop"}}`)),
	}}
	settings := &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
		ServiceTier: OpenAIFastTierAny,
		Action:      OpenAIFastPolicyActionForcePriority,
		Scope:       BetaPolicyScopeAll,
	}}}
	svc := newOpenAIGatewayServiceWithSettings(t, settings)
	svc.httpUpstream = upstream

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughRejectsOversizedTerminalBeforeDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`)
	outputEvent := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"
	oversizedTerminal := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_large\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1},\"padding\":\"" + strings.Repeat("x", 512) + "\"}}\n\n"
	upstreamBody := outputEvent + oversizedTerminal + "data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 256}},
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.ErrorIs(t, err, ErrOfficialCodexTransparentStreamIntegrity)
	require.NotNil(t, result)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
	require.Equal(t, outputEvent, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughRecoversAfterOversizedDelta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`)
	oversizedDelta := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + strings.Repeat("x", 512) + "\"}\n\n"
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_after_large_delta\",\"usage\":{\"input_tokens\":7,\"output_tokens\":3}}}\n\n"
	upstreamBody := oversizedDelta + completed + "data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{MaxLineSize: 128}},
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, "resp_after_large_delta", result.ResponseID)
	require.Equal(t, upstreamBody, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughDoneRequiresTrustedTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_done_order\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n"
	done := "data: [DONE]\n\n"

	for _, tt := range []struct {
		name       string
		upstream   string
		wantBody   string
		wantErr    error
		wantInput  int
		wantOutput int
	}{
		{name: "done only", upstream: done, wantErr: ErrOfficialCodexTransparentStreamIntegrity},
		{name: "done before terminal", upstream: done + completed, wantErr: ErrOfficialCodexTransparentStreamIntegrity},
		{name: "completed before done", upstream: completed + done, wantBody: completed + done, wantInput: 2, wantOutput: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(tt.upstream)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

			result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.NotNil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tt.wantInput, result.Usage.InputTokens)
				require.Equal(t, tt.wantOutput, result.Usage.OutputTokens)
			}
			require.Equal(t, tt.wantBody, recorder.Body.String())
		})
	}
}

func TestOpenAIGatewayService_OfficialCodexPassthroughSanitizesAndBoundsHTTPErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"test"}`)

	for _, tt := range []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "within fixed limit", body: `{"error":{"message":"sensitive upstream detail"}}`, wantStatus: http.StatusBadRequest, wantBody: `{"error":{"message":"Upstream request failed","type":"upstream_error"}}`},
		{name: "oversized body", body: strings.Repeat("x", int(openAIUpstreamErrorBodyReadLimit*2)), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"message":"Upstream request failed","type":"upstream_error"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
			upstreamBody := &transparentTrackingReadCloser{reader: strings.NewReader(tt.body)}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header: http.Header{
					"Content-Type":      []string{"application/json"},
					"X-Upstream-Secret": []string{"must-not-be-forwarded"},
				},
				Body: upstreamBody,
			}}
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{},
				httpUpstream: upstream,
			}

			result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, tt.wantStatus, recorder.Code)
			require.Equal(t, tt.wantBody, recorder.Body.String())
			require.Empty(t, recorder.Header().Get("X-Upstream-Secret"))
			require.LessOrEqual(t, upstreamBody.bytesRead, int(openAIUpstreamErrorBodyReadLimit))
		})
	}
}

func TestOpenAIGatewayService_OfficialCodexPassthroughPreservesNonStreamingBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"test"}`)
	upstreamBody := "event: response.created\r\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_raw\"}}\r\n\r\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_raw\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\r\n\r\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, upstreamBody, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughFailsSettlementClosedWhenNonStreamingUsageIsUnobservable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"test"}`)
	upstreamBody := `{"id":"resp_large","output":[{"type":"message","content":"` + strings.Repeat("x", 128) + `"}],"usage":{"input_tokens":4,"output_tokens":2}}`
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{UpstreamResponseReadMaxBytes: 64}},
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.EqualError(t, err, "official Codex nonstream response rejected before commit: response exceeded bounded observation limit")
	require.Nil(t, result)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughRejectsUsageLessNonstreamBeforeCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"test"}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_missing_usage","output":[]}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.EqualError(t, err, "official Codex nonstream response rejected before commit: response did not contain trustworthy usage")
	require.Nil(t, result)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughRejectsEmptyNonstreamUsageBeforeCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"test"}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_empty_usage","output":[],"usage":{}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.EqualError(t, err, "official Codex nonstream response rejected before commit: response did not contain trustworthy usage")
	require.Nil(t, result)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughRejectsCompletedWithEmptyUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	upstreamBody := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_empty_usage\",\"usage\":{}}}\n\n" +
		"data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`))

	require.ErrorIs(t, err, ErrOfficialCodexTransparentStreamIntegrity)
	require.NotNil(t, result)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIGatewayService_OfficialCodexPassthroughAcceptsExplicitZeroUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	upstreamBody := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_zero_usage\",\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n" +
		"data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
	require.Equal(t, upstreamBody, recorder.Body.String())
}

func TestOpenAITransparentUsageHasRecognizedNonNegativeIntegerTokenCount(t *testing.T) {
	for _, tt := range []struct {
		name  string
		usage string
		want  bool
	}{
		{name: "empty usage", usage: `{}`, want: false},
		{name: "explicit zero", usage: `{"input_tokens":0}`, want: true},
		{name: "positive integer", usage: `{"output_tokens":12}`, want: true},
		{name: "negative integer", usage: `{"input_tokens":-1}`, want: false},
		{name: "negative fraction", usage: `{"input_tokens":-0.5}`, want: false},
		{name: "positive fraction", usage: `{"input_tokens":0.5}`, want: false},
		{name: "integer exponent", usage: `{"input_tokens":1e3}`, want: true},
		{name: "fraction exponent", usage: `{"input_tokens":1e-1}`, want: false},
		{name: "overflowing exponent", usage: `{"input_tokens":1e999}`, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAITransparentUsageHasRecognizedTokenCount(gjson.Parse(tt.usage)))
		})
	}
}

func TestOpenAIGatewayService_OfficialCodexPassthroughSanitizesFailedSSEAndSuppressesDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`)
	upstreamBody := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"error\":{\"message\":\"upstream failed\"},\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\r\n\r\n" +
		"data: [DONE]\r\n\r\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.ErrorIs(t, err, ErrOfficialCodexTransparentStreamFailed)
	require.NotNil(t, result)
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "response.failed"))
	require.Contains(t, recorder.Body.String(), "Upstream request failed")
	require.NotContains(t, recorder.Body.String(), "upstream failed")
	require.NotContains(t, recorder.Body.String(), "[DONE]")
	require.Contains(t, recorder.Body.String(), `"input_tokens":2`)
}

func TestOpenAIGatewayService_OfficialCodexPassthroughFailedWithoutUsageDoesNotReturnZeroUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	upstreamBody := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed_no_usage\",\"error\":{\"message\":\"upstream failed\"}}}\n\n" +
		"data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`))

	require.ErrorIs(t, err, ErrOfficialCodexTransparentStreamFailed)
	require.Nil(t, result)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "response.failed"))
	require.Contains(t, recorder.Body.String(), "Upstream request failed")
	require.NotContains(t, recorder.Body.String(), "upstream failed")
	require.NotContains(t, recorder.Body.String(), "[DONE]")
}

func TestOpenAIGatewayService_OfficialCodexPassthroughFailedWithEmptyUsageDoesNotReturnZeroUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
	upstreamBody := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed_empty_usage\",\"error\":{\"message\":\"upstream failed\"},\"usage\":{}}}\n\n" +
		"data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`))

	require.ErrorIs(t, err, ErrOfficialCodexTransparentStreamFailed)
	require.Nil(t, result)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "response.failed"))
	require.Contains(t, recorder.Body.String(), "Upstream request failed")
	require.NotContains(t, recorder.Body.String(), "upstream failed")
	require.NotContains(t, recorder.Body.String(), "[DONE]")
}

func TestOpenAIGatewayService_OfficialCodexPassthroughClientDisconnectDoesNotWaiveMissingTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.145.0")
	writer := &passthroughFlushTestWriter{ResponseWriter: c.Writer, recorder: recorder, failAfterWrites: 0}
	c.Writer = writer
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"test"}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

	require.ErrorIs(t, err, ErrOfficialCodexTransparentStreamIntegrity)
	require.NotNil(t, result)
	require.Equal(t, 1, writer.failedWrites)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIGatewayService_NonTransparentPassthroughKeepsLegacyNormalizationAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("API key strips input namespaces", func(t *testing.T) {
		c, _ := newOpenAITransparentPassthroughTestContext("/v1/responses", "codex_cli_rs/0.145.0")
		requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":[{"type":"function_call","call_id":"call_1","namespace":"collaboration","name":"list_agents","arguments":"{}"}]}`)
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_api_key","usage":{"input_tokens":1,"output_tokens":1}}`)),
		}}
		account := newOpenAITransparentPassthroughTestAccount()
		account.Type = AccountTypeAPIKey
		account.Credentials = map[string]any{"api_key": "test-key", "base_url": "https://api.example.test"}
		account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeAuto)
		account.Extra[openai_compat.ExtraKeyResponsesSupported] = true
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

		result, err := svc.Forward(context.Background(), c, account, requestBody)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "list_agents", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
		require.False(t, gjson.GetBytes(upstream.lastBody, "input.0.namespace").Exists())
	})

	t.Run("compact rejects namespace collisions", func(t *testing.T) {
		c, recorder := newOpenAITransparentPassthroughTestContext("/v1/responses/compact", "codex_cli_rs/0.145.0")
		requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"tools":[{"type":"function","name":"collaboration__list_agents","parameters":{"type":"object"}},{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"list_agents","parameters":{"type":"object"}}]}],"input":"test"}`)
		upstream := &httpUpstreamRecorder{}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

		result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

		require.Error(t, err)
		require.Nil(t, result)
		require.Nil(t, upstream.lastReq)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Equal(t, "tools", gjson.Get(recorder.Body.String(), "error.param").String())
		require.Contains(t, gjson.Get(recorder.Body.String(), "error.message").String(), "conflicts with a top-level tool")
	})

	t.Run("nonofficial OAuth normalizes body", func(t *testing.T) {
		c, _ := newOpenAITransparentPassthroughTestContext("/v1/responses", "curl/8.0")
		requestBody := []byte(`{"model":"gpt-5.6-sol","stream":false,"store":true,"instructions":"test","input":"test"}`)
		upstreamSSE := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_nonofficial\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n"
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
		}}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

		result, err := svc.Forward(context.Background(), c, newOpenAITransparentPassthroughTestAccount(), requestBody)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
		require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
	})
}
