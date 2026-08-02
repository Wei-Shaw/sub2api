package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func passthroughCompressionTestConfig() *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{
		OpenAIRequestCompression: config.GatewayOpenAIRequestCompressionConfig{
			Enabled:              true,
			FallbackUncompressed: true,
		},
	}}
}

func passthroughCompressionTestAccount(id int64) *Account {
	return &Account{
		ID:          id,
		Name:        "oauth-compression",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Extra:       map[string]any{"openai_passthrough": true},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func decodePassthroughCompressionBody(t *testing.T, body []byte) []byte {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer decoder.Close()
	decoded, err := decoder.DecodeAll(body, nil)
	require.NoError(t, err)
	return decoded
}

func compressionErrorResponse(status int, code, message string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":"` + code + `","message":"` + message + `"}}`,
		)),
	}
}

func compressionBridgeSuccessResponse(responseID string) *http.Response {
	body := `data: {"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-5","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestBuildUpstreamRequestOpenAIPassthroughLegacyWrapperStaysUncompressed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5","stream":true,"input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	svc := &OpenAIGatewayService{cfg: passthroughCompressionTestConfig()}
	req, err := svc.buildUpstreamRequestOpenAIPassthrough(
		context.Background(), c, passthroughCompressionTestAccount(1), body, "oauth-token",
	)

	require.NoError(t, err)
	require.Empty(t, req.Header.Get("Content-Encoding"))
	wireBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, body, wireBody)
}

func TestForwardOpenAIPassthroughCompressionFallbackIsGlobalAcrossAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5","stream":true,"instructions":"test","input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		compressionErrorResponse(http.StatusUnsupportedMediaType, "unsupported_content_encoding", "unsupported content-encoding: zstd"),
		compressionErrorResponse(http.StatusTooManyRequests, "rate_limit_exceeded", "limited"),
		compressionErrorResponse(http.StatusBadRequest, "invalid_request_error", "business validation failed"),
	}}
	svc := &OpenAIGatewayService{cfg: passthroughCompressionTestConfig(), httpUpstream: upstream}

	_, firstErr := svc.Forward(context.Background(), c, passthroughCompressionTestAccount(11), body)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, firstErr, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)

	_, secondErr := svc.Forward(context.Background(), c, passthroughCompressionTestAccount(12), body)
	require.Error(t, secondErr)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[1].Header.Get("Content-Encoding"))
	require.Empty(t, upstream.requests[2].Header.Get("Content-Encoding"))
	require.JSONEq(t, string(decodePassthroughCompressionBody(t, upstream.bodies[0])), string(upstream.bodies[1]))
	require.JSONEq(t, string(upstream.bodies[1]), string(upstream.bodies[2]))
}

type cancelAfterResponseUpstream struct {
	*httpUpstreamRecorder
	cancel context.CancelFunc
}

func (u *cancelAfterResponseUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	resp, err := u.httpUpstreamRecorder.Do(req, proxyURL, accountID, accountConcurrency)
	u.cancel()
	return resp, err
}

func TestForwardOpenAIPassthroughCompressionDoesNotFallbackAfterClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5","stream":true,"instructions":"test","input":"hello"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
	baseUpstream := &httpUpstreamRecorder{responses: []*http.Response{
		compressionErrorResponse(http.StatusUnsupportedMediaType, "unsupported_content_encoding", "unsupported content-encoding: zstd"),
	}}
	upstream := &cancelAfterResponseUpstream{httpUpstreamRecorder: baseUpstream, cancel: cancel}
	svc := &OpenAIGatewayService{cfg: passthroughCompressionTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(ctx, c, passthroughCompressionTestAccount(21), body)

	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, baseUpstream.requests, 1)
	require.Equal(t, "zstd", baseUpstream.requests[0].Header.Get("Content-Encoding"))
}

type bridgeCompressionCloseCheckingUpstream struct {
	*httpUpstreamRecorder
	firstBody         *passthroughCloseTrackingReadCloser
	closedBeforeRetry bool
}

func (u *bridgeCompressionCloseCheckingUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	if len(u.requests) == 1 {
		u.closedBeforeRetry = u.firstBody.closed
	}
	return u.httpUpstreamRecorder.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestOpenAIWSHTTPBridgeCompressionFallbackRebuildsAndDisablesConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"type":"response.create","model":"gpt-5","stream":true,"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"},"input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	firstBody := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(`{"error":{"code":"unsupported_content_encoding","message":"unsupported content-encoding: zstd"}}`)}
	baseUpstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusUnsupportedMediaType, Header: make(http.Header), Body: firstBody},
		compressionBridgeSuccessResponse("resp_bridge_fallback"),
		compressionBridgeSuccessResponse("resp_bridge_next_turn"),
	}}
	upstream := &bridgeCompressionCloseCheckingUpstream{
		httpUpstreamRecorder: baseUpstream,
		firstBody:            firstBody,
	}
	svc := &OpenAIGatewayService{cfg: passthroughCompressionTestConfig(), httpUpstream: upstream}
	account := passthroughCompressionTestAccount(31)
	var writes [][]byte
	writeClient := func(message []byte) error {
		writes = append(writes, append([]byte(nil), message...))
		return nil
	}

	firstResult, firstErr := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "oauth-token", payload, len(payload),
		"gpt-5", "", "", "", "", 1, writeClient,
	)
	require.NoError(t, firstErr)
	require.NotNil(t, firstResult)

	secondResult, secondErr := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "oauth-token", payload, len(payload),
		"gpt-5", "", "", "", "", 2, writeClient,
	)
	require.NoError(t, secondErr)
	require.NotNil(t, secondResult)

	require.Len(t, baseUpstream.requests, 3)
	require.Equal(t, "zstd", baseUpstream.requests[0].Header.Get("Content-Encoding"))
	require.Empty(t, baseUpstream.requests[1].Header.Get("Content-Encoding"))
	require.Empty(t, baseUpstream.requests[2].Header.Get("Content-Encoding"))
	require.Equal(t, "true", baseUpstream.requests[0].Header.Get(responsesLiteHeader))
	require.Equal(t, "true", baseUpstream.requests[1].Header.Get(responsesLiteHeader))
	require.True(t, upstream.closedBeforeRetry)
	require.True(t, firstBody.closed)
	storedScopeValue, ginScopeStored := c.Get(openAIRequestCompressionScopeContextKey)
	require.True(t, ginScopeStored)
	storedScope, ok := storedScopeValue.(*openAIRequestCompressionScope)
	require.True(t, ok)
	storedScope.mu.Lock()
	storedEntry := storedScope.entry
	storedScope.mu.Unlock()
	require.Nil(t, storedEntry)
	require.Len(t, writes, 2)
	require.JSONEq(t, string(decodePassthroughCompressionBody(t, baseUpstream.bodies[0])), string(baseUpstream.bodies[1]))
}

func TestOpenAIWSHTTPBridgeCompressionReusesBodyAcrossPreOutputFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"type":"response.create","model":"gpt-5","stream":true,"input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		compressionErrorResponse(http.StatusServiceUnavailable, "server_error", "temporarily unavailable"),
		compressionBridgeSuccessResponse("resp_bridge_replayed"),
	}}
	svc := &OpenAIGatewayService{cfg: passthroughCompressionTestConfig(), httpUpstream: upstream}
	metricsBefore := SnapshotOpenAIRequestCompressionMetrics()

	firstResult, firstErr := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, passthroughCompressionTestAccount(51), "oauth-token", payload, len(payload),
		"gpt-5", "", "", "", "", 1, func([]byte) error { return nil },
	)
	var failoverErr *UpstreamFailoverError
	require.Nil(t, firstResult)
	require.ErrorAs(t, firstErr, &failoverErr)

	storedScopeValue, ok := c.Get(openAIRequestCompressionScopeContextKey)
	require.True(t, ok)
	storedScope, ok := storedScopeValue.(*openAIRequestCompressionScope)
	require.True(t, ok)
	storedScope.mu.Lock()
	storedEntry := storedScope.entry
	storedScope.mu.Unlock()
	require.NotNil(t, storedEntry)

	secondResult, secondErr := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, passthroughCompressionTestAccount(52), "oauth-token", payload, len(payload),
		"gpt-5", "", "", "", "", 1, func([]byte) error { return nil },
	)
	require.NoError(t, secondErr)
	require.NotNil(t, secondResult)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
	require.Equal(t, "zstd", upstream.requests[1].Header.Get("Content-Encoding"))
	require.Equal(t, upstream.bodies[0], upstream.bodies[1])

	metricsAfter := SnapshotOpenAIRequestCompressionMetrics()
	require.Equal(t, metricsBefore.CompressionOperationsTotal+1, metricsAfter.CompressionOperationsTotal)
	require.Equal(t, metricsBefore.CompressionCacheHitsTotal+1, metricsAfter.CompressionCacheHitsTotal)
	storedScope.mu.Lock()
	storedEntry = storedScope.entry
	storedScope.mu.Unlock()
	require.Nil(t, storedEntry)
}

func TestForwardOpenAIPassthroughBusiness400DoesNotFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5","stream":true,"instructions":"test","input":"hello"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		compressionErrorResponse(http.StatusBadRequest, "unsupported_parameter", "unsupported content-encoding: zstd"),
	}}
	svc := &OpenAIGatewayService{cfg: passthroughCompressionTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, passthroughCompressionTestAccount(41), body)

	require.Nil(t, result)
	require.Error(t, err)
	require.False(t, errors.Is(err, context.Canceled))
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "zstd", upstream.requests[0].Header.Get("Content-Encoding"))
}
