package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func strictResponsesTestConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled: false, AllowInsecureHTTP: true,
	}}}
}

func strictResponsesTestAccount(baseURL string, noAuth bool) *Account {
	credentials := map[string]any{
		"base_url": baseURL,
		"api_key":  "UPSTREAM_KEY",
	}
	if noAuth {
		delete(credentials, "api_key")
		credentials[OpenAIUpstreamAuthModeCredentialKey] = OpenAIUpstreamAuthModeNone
	}
	return &Account{
		ID:          71001,
		Name:        "strict-responses",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: credentials,
		Extra: map[string]any{
			OpenAIResponsesForwardModeExtraKey: string(OpenAIResponsesForwardModeStrictRaw),
		},
		Concurrency: 1,
		Status:      StatusActive,
		Schedulable: true,
	}
}

func strictResponsesTestContext(path string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c, recorder
}

func TestOpenAIGatewayStrictResponsesPreservesBodyAndSafeHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte("{\n  \"model\": \"chatgpt-web/high\",\n  \"reasoning\": {\"effort\": \"minimal\"},\n  \"client_metadata\": {\"cwd\": \"/workspace\"},\n  \"tools\": [{\"type\": \"custom\", \"opaque\": {\"z\": 1}}],\n  \"stream\": false\n}\n")
	responseBody := []byte(`{"id":"resp_strict_1","object":"response","status":"completed","model":"chatgpt-web/high","output":[],"usage":{"input_tokens":7,"output_tokens":3}}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":         []string{"application/json"},
			"OpenAI-Processing-Ms": []string{"17"},
		},
		Body: io.NopCloser(bytes.NewReader(responseBody)),
	}}
	c, recorder := strictResponsesTestContext("/v1/responses")
	c.Request.Header.Set("Authorization", "Bearer DOWNSTREAM_SUB2API_KEY")
	c.Request.Header.Set("Proxy-Authorization", "Basic forbidden")
	c.Request.Header.Set("X-API-Key", "DOWNSTREAM_SUB2API_KEY")
	c.Request.Header.Set("Cookie", "session=browser-secret")
	c.Request.Header.Set("Connection", "upgrade")
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/1.2.3")
	c.Request.Header.Set("originator", "codex_cli_rs")
	c.Request.Header.Set("X-Session-Id", "session-123")
	c.Request.Header.Set("X-Codex-Turn-Metadata", "opaque-turn-metadata")
	c.Request.Header.Set("X-Codex-Future-Signal", "future-value")
	c.Request.Header.Set("X-Codex-Token", "must-not-forward")
	c.Request.Header.Set("X-Codex-Session-Token", "must-not-forward-either")

	svc := &OpenAIGatewayService{cfg: strictResponsesTestConfig(), httpUpstream: upstream}
	account := strictResponsesTestAccount("http://strict.example/v1", false)
	account.Credentials["model_mapping"] = map[string]any{"chatgpt-web/high": "must-not-map"}
	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, body, upstream.lastBody)
	require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
	require.Equal(t, "Bearer UPSTREAM_KEY", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "identity", upstream.lastReq.Header.Get("Accept-Encoding"))
	require.Empty(t, upstream.lastReq.Header.Get("Proxy-Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("X-API-Key"))
	require.Empty(t, upstream.lastReq.Header.Get("Cookie"))
	require.Empty(t, upstream.lastReq.Header.Get("Connection"))
	require.Empty(t, upstream.lastReq.Header.Get("Upgrade"))
	require.Equal(t, "codex_cli_rs/1.2.3", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", upstream.lastReq.Header.Get("originator"))
	require.Equal(t, "session-123", upstream.lastReq.Header.Get("X-Session-Id"))
	require.Equal(t, "opaque-turn-metadata", upstream.lastReq.Header.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, "future-value", upstream.lastReq.Header.Get("X-Codex-Future-Signal"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Codex-Token"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Codex-Session-Token"))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, responseBody, recorder.Body.Bytes())
	require.Equal(t, "17", recorder.Header().Get("OpenAI-Processing-Ms"))
	require.Equal(t, "resp_strict_1", result.ResponseID)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}

func TestOpenAIGatewayStrictResponsesCompactNoAuthPreservesUpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"custom/model","input":[{"type":"compaction","encrypted_content":"opaque"}],"stream":false}`)
	responseBody := []byte(`{"error":{"type":"invalid_request_error","message":"continuation state missing"}}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusConflict,
		Header: http.Header{
			"Content-Type":      []string{"application/json"},
			"Set-Cookie":        []string{"bridge_session=secret"},
			"OpenAI-Request-ID": []string{"req_strict_conflict"},
		},
		Body: io.NopCloser(bytes.NewReader(responseBody)),
	}}
	c, recorder := strictResponsesTestContext("/v1/responses/compact")
	c.Request.Header.Set("Accept", "text/event-stream")
	c.Request.Header.Set("X-Codex-Turn-Metadata", "compact-metadata")

	svc := &OpenAIGatewayService{cfg: strictResponsesTestConfig(), httpUpstream: upstream}
	result, err := svc.Forward(context.Background(), c, strictResponsesTestAccount("http://strict.example/v1?tenant=acme", true), body)

	var upstreamErr *openAIStrictResponsesHTTPError
	require.ErrorAs(t, err, &upstreamErr)
	require.NotNil(t, result)
	require.Equal(t, body, upstream.lastBody)
	require.Equal(t, "/v1/responses/compact", upstream.lastReq.URL.Path)
	require.Equal(t, "tenant=acme", upstream.lastReq.URL.RawQuery)
	require.Empty(t, upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, "compact-metadata", upstream.lastReq.Header.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Equal(t, responseBody, recorder.Body.Bytes())
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
	require.Equal(t, "req_strict_conflict", recorder.Header().Get("OpenAI-Request-ID"))
}

type fragmentedStrictBody struct {
	fragments [][]byte
	finalErr  error
}

func (b *fragmentedStrictBody) Read(p []byte) (int, error) {
	if len(b.fragments) == 0 {
		if b.finalErr != nil {
			err := b.finalErr
			b.finalErr = nil
			return 0, err
		}
		return 0, io.EOF
	}
	fragment := b.fragments[0]
	b.fragments = b.fragments[1:]
	return copy(p, fragment), nil
}

func (b *fragmentedStrictBody) Close() error { return nil }

func TestOpenAIGatewayStrictResponsesStreamsFragmentedSSEByteForByte(t *testing.T) {
	gin.SetMode(gin.TestMode)
	want := []byte("event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_stream_strict\"}}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream_strict\",\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n" +
		"data: [DONE]\n\n")
	fragments := [][]byte{
		want[:7], want[7:31], want[31:78], want[78:141], want[141:219], want[219:],
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       &fragmentedStrictBody{fragments: fragments},
	}}
	c, recorder := strictResponsesTestContext("/v1/responses")
	body := []byte(`{"model":"chatgpt-web/high","stream":true}`)

	svc := &OpenAIGatewayService{cfg: strictResponsesTestConfig(), httpUpstream: upstream}
	result, err := svc.Forward(context.Background(), c, strictResponsesTestAccount("http://strict.example/v1", true), body)

	require.NoError(t, err)
	require.Equal(t, want, recorder.Body.Bytes())
	require.Equal(t, "resp_stream_strict", result.ResponseID)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}

func TestOpenAIGatewayStrictResponsesReadErrorKeepsRawPartialStreamAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	partial := []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_partial\",\"usage\":{\"input_tokens\":6,\"output_tokens\":1}}}\n\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &fragmentedStrictBody{
			fragments: [][]byte{partial[:17], partial[17:]},
			finalErr:  io.ErrUnexpectedEOF,
		},
	}}
	c, recorder := strictResponsesTestContext("/v1/responses")
	svc := &OpenAIGatewayService{cfg: strictResponsesTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(
		context.Background(),
		c,
		strictResponsesTestAccount("http://strict.example/v1", true),
		[]byte(`{"model":"custom/model","stream":true}`),
	)

	require.ErrorContains(t, err, "upstream response failed: read strict Responses upstream")
	require.Equal(t, partial, recorder.Body.Bytes())
	require.NotNil(t, result)
	require.Equal(t, "resp_partial", result.ResponseID)
	require.Equal(t, 6, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
}

type cancelAwareStrictUpstream struct {
	started  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func (u *cancelAwareStrictUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	close(u.started)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       strictContextBody{ctx: req.Context(), canceled: u.canceled, once: &u.once},
	}, nil
}

func (u *cancelAwareStrictUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type strictContextBody struct {
	ctx      context.Context
	canceled chan struct{}
	once     *sync.Once
}

func (b strictContextBody) Read(_ []byte) (int, error) {
	<-b.ctx.Done()
	b.once.Do(func() { close(b.canceled) })
	return 0, b.ctx.Err()
}

func (b strictContextBody) Close() error { return nil }

func TestOpenAIGatewayStrictResponsesCancellationCancelsUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &cancelAwareStrictUpstream{started: make(chan struct{}), canceled: make(chan struct{})}
	c, _ := strictResponsesTestContext("/v1/responses")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	svc := &OpenAIGatewayService{cfg: strictResponsesTestConfig(), httpUpstream: upstream}
	go func() {
		_, err := svc.Forward(ctx, c, strictResponsesTestAccount("http://strict.example/v1", true), []byte(`{"model":"custom/model","stream":true}`))
		done <- err
	}()

	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("strict upstream request did not start")
	}
	cancel()
	select {
	case <-upstream.canceled:
	case <-time.After(time.Second):
		t.Fatal("strict upstream request context was not canceled")
	}
	require.Error(t, <-done)
}
