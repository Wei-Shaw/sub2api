package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type anthropicHTTPUpstreamRecorder struct {
	lastReq  *http.Request
	lastBody []byte
	resp     *http.Response
	err      error
REDACTED

func newAnthropicAPIKeyAccountForTest() *Account {
REDACTED
		ID:          201,
		Name:        "anthropic-apikey-pass-test",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
	REDACTED,
		Extra: map[string]any{
			"anthropic_passthrough": true,
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED
REDACTED

func (u *anthropicHTTPUpstreamRecorder) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.lastReq = req
	if req != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		u.lastBody = b
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
REDACTED
	if u.err != nil {
		return nil, u.err
REDACTED
	return u.resp, nil
REDACTED

func (u *anthropicHTTPUpstreamRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, enableTLSFingerprint bool) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

type streamReadCloser struct {
	payload []byte
	sent    bool
	err     error
REDACTED

func (r *streamReadCloser) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, r.payload)
		return n, nil
REDACTED
	if r.err != nil {
		return 0, r.err
REDACTED
	return 0, io.EOF
REDACTED

func (r *streamReadCloser) Close() error { return nil REDACTED

type failWriteResponseWriter struct {
	gin.ResponseWriter
REDACTED

func (w *failWriteResponseWriter) Write(data []byte) (int, error) {
	return 0, errors.New("client disconnected")
REDACTED

func (w *failWriteResponseWriter) WriteString(_ string) (int, error) {
	return 0, errors.New("client disconnected")
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardStreamPreservesBodyAndAuthReplacement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/1.0.0")
	c.Request.Header.Set("Authorization", "Bearer inbound-token")
	c.Request.Header.Set("X-Api-Key", "inbound-api-key")
	c.Request.Header.Set("X-Goog-Api-Key", "inbound-goog-key")
	c.Request.Header.Set("Cookie", "secret=1")
	c.Request.Header.Set("Anthropic-Beta", "REDACTED")

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","stream":true,"system":[{"type":"text","text":"x-anthropic-billing-header keep"REDACTED],"messages":[{"role":"user","content":[{"type":"text","text":"hello"REDACTED]REDACTED]REDACTED`)
	parsed := &ParsedRequest{
		Body:   body,
		Model:  "claude-3-7-sonnet-20250219",
		Stream: true,
REDACTED

	upstreamSSE := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":9,"cached_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		`data: {"type":"message_delta","usage":{"output_tokens":3REDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"REDACTED,
				"x-request-id": []string{"rid-anthropic-pass"REDACTED,
				"Set-Cookie":   []string{"secret=upstream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(upstreamSSE)),
	REDACTED,
REDACTED

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
		REDACTED,
	REDACTED,
		httpUpstream:        upstream,
		rateLimitService:    &RateLimitService{REDACTED,
		deferredService:     &DeferredService{REDACTED,
		billingCacheService: nil,
REDACTED

	account := &Account{
		ID:          101,
		Name:        "anthropic-apikey-pass",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":       "upstream-anthropic-key",
			"base_url":      "https://api.anthropic.com",
			"model_mapping": map[string]any{"claude-3-7-sonnet-20250219": "claude-3-haiku-20240307"REDACTED,
	REDACTED,
		Extra: map[string]any{
			"anthropic_passthrough": true,
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, parsed)
REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)

	require.Equal(t, body, upstream.lastBody, "透传模式不应改写上游请求体")
	require.Equal(t, "claude-3-7-sonnet-20250219", gjson.GetBytes(upstream.lastBody, "model").String())

	require.Equal(t, "upstream-anthropic-key", upstream.lastReq.Header.Get("x-api-key"))
	require.Empty(t, upstream.lastReq.Header.Get("authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("x-goog-api-key"))
	require.Empty(t, upstream.lastReq.Header.Get("cookie"))
	require.Equal(t, "2023-06-01", upstream.lastReq.Header.Get("anthropic-version"))
	require.Equal(t, "REDACTED", upstream.lastReq.Header.Get("anthropic-beta"))
	require.Empty(t, upstream.lastReq.Header.Get("x-stainless-lang"), "API Key 透传不应注入 OAuth 指纹头")

	require.Contains(t, rec.Body.String(), `"cached_tokens":7`)
	require.NotContains(t, rec.Body.String(), `"cache_read_input_tokens":7`, "透传输出不应被网关改写")
	require.Equal(t, 7, result.Usage.CacheReadInputTokens, "计费 usage 解析应保留 cached_tokens 兼容")
	require.Empty(t, rec.Header().Get("Set-Cookie"), "响应头应经过安全过滤")
	rawBody, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	bodyBytes, ok := rawBody.([]byte)
	require.True(t, ok, "应以 []byte 形式缓存上游请求体，避免重复 string 拷贝")
	require.Equal(t, body, bodyBytes)
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardCountTokensPreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", nil)
	c.Request.Header.Set("Authorization", "Bearer inbound-token")
	c.Request.Header.Set("X-Api-Key", "inbound-api-key")
	c.Request.Header.Set("Cookie", "secret=1")

	body := []byte(`{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":[{"type":"text","text":"hello"REDACTED]REDACTED],"thinking":{"type":"enabled"REDACTEDREDACTED`)
	parsed := &ParsedRequest{
		Body:  body,
		Model: "claude-3-5-sonnet-latest",
REDACTED

	upstreamRespBody := `{"input_tokens":42REDACTED`
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"REDACTED,
				"x-request-id": []string{"rid-count"REDACTED,
				"Set-Cookie":   []string{"secret=upstream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(upstreamRespBody)),
	REDACTED,
REDACTED

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
		REDACTED,
	REDACTED,
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{REDACTED,
REDACTED

	account := &Account{
		ID:          102,
		Name:        "anthropic-apikey-pass-count",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":       "upstream-anthropic-key",
			"base_url":      "https://api.anthropic.com",
			"model_mapping": map[string]any{"claude-3-5-sonnet-latest": "claude-3-opus-20240229"REDACTED,
	REDACTED,
		Extra: map[string]any{
			"anthropic_passthrough": true,
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	err := svc.ForwardCountTokens(context.Background(), c, account, parsed)
REDACTED

	require.Equal(t, body, upstream.lastBody, "count_tokens 透传模式不应改写请求体")
	require.Equal(t, "claude-3-5-sonnet-latest", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "upstream-anthropic-key", upstream.lastReq.Header.Get("x-api-key"))
	require.Empty(t, upstream.lastReq.Header.Get("authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("cookie"))
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, upstreamRespBody, rec.Body.String())
	require.Empty(t, rec.Header().Get("Set-Cookie"))
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_BuildRequestRejectsInvalidBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					Enabled: false,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
REDACTED
			"api_key":  "k",
			"base_url": "://invalid-url",
	REDACTED,
REDACTED

	_, err := svc.buildUpstreamRequestAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(`{REDACTED`), "k")
REDACTED
REDACTED

func TestGatewayService_AnthropicOAuth_NotAffectedByAPIKeyPassthroughToggle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSizeREDACTED,
	REDACTED,
REDACTED
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"anthropic_passthrough": true,
	REDACTED,
REDACTED

	require.False(t, account.IsAnthropicAPIKeyPassthroughEnabled())

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"claude-3-7-sonnet-20250219"REDACTED`), "oauth-token", "oauth", "claude-3-7-sonnet-20250219", true, false)
REDACTED
	require.Equal(t, "Bearer oauth-token", req.Header.Get("authorization"))
	require.Contains(t, req.Header.Get("anthropic-beta"), claude.BetaOAuth, "OAuth 链路仍应按原逻辑补齐 oauth beta")
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingStillCollectsUsageAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Use a canceled context recorder to simulate client disconnect behavior.
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
		REDACTED,
	REDACTED,
		rateLimitService: &RateLimitService{REDACTED,
REDACTED

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":11REDACTEDREDACTEDREDACTED`,
			"",
			`data: {"type":"message_delta","usage":{"output_tokens":5REDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n"))),
REDACTED

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1REDACTED, time.Now())
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_NonStreamingSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"model":"claude-3-5-sonnet-latest","messages":[{"role":"user","content":[{"type":"text","text":"hello"REDACTED]REDACTED]REDACTED`)
	upstreamJSON := `{"id":"msg_1","type":"message","usage":{"input_tokens":12,"output_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3REDACTED,"cached_tokens":4REDACTEDREDACTED`
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"REDACTED,
				"x-request-id": []string{"rid-nonstream"REDACTED,
		REDACTED,
			Body: io.NopCloser(strings.NewReader(upstreamJSON)),
	REDACTED,
REDACTED
	svc := &GatewayService{
		cfg:              &config.Config{REDACTED,
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{REDACTED,
REDACTED

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), body, "claude-3-5-sonnet-latest", false, time.Now())
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 12, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheCreationInputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
	require.Equal(t, upstreamJSON, rec.Body.String())
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_InvalidTokenType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	account := &Account{
		ID:       202,
		Name:     "anthropic-oauth",
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "oauth-token",
	REDACTED,
REDACTED
	svc := &GatewayService{REDACTED

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(`{REDACTED`), "claude-3-5-sonnet-latest", false, time.Now())
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "requires apikey token")
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_UpstreamRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstream := &anthropicHTTPUpstreamRecorder{
		err: errors.New("dial tcp timeout"),
REDACTED
	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
		REDACTED,
	REDACTED,
		httpUpstream: upstream,
REDACTED
	account := newAnthropicAPIKeyAccountForTest()

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, account, []byte(`{"model":"x"REDACTED`), "x", false, time.Now())
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "upstream request failed")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	rawBody, ok := c.Get(OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	_, ok = rawBody.([]byte)
	require.True(t, ok)
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_ForwardDirect_EmptyResponseBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"x-request-id": []string{"rid-empty-body"REDACTEDREDACTED,
			Body:       nil,
	REDACTED,
REDACTED
	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTED,
		REDACTED,
	REDACTED,
		httpUpstream: upstream,
REDACTED

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{"model":"x"REDACTED`), "x", false, time.Now())
	require.Nil(t, result)
REDACTED
	require.Contains(t, err.Error(), "empty response")
REDACTED

func TestExtractAnthropicSSEDataLine(t *testing.T) {
	t.Run("valid data line with spaces", func(t *testing.T) {
		data, ok := extractAnthropicSSEDataLine("data:   {\"type\":\"message_start\"REDACTED")
		require.True(t, ok)
		require.Equal(t, `{"type":"message_start"REDACTED`, data)
REDACTED)

	t.Run("non data line", func(t *testing.T) {
		data, ok := extractAnthropicSSEDataLine("event: message_start")
		require.False(t, ok)
		require.Empty(t, data)
REDACTED)
REDACTED

func TestGatewayService_ParseSSEUsagePassthrough_MessageStartFallbacks(t *testing.T) {
	svc := &GatewayService{REDACTED
	usage := &ClaudeUsage{REDACTED
	data := `{"type":"message_start","message":{"usage":{"input_tokens":12,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cached_tokens":9,"cache_creation":{"ephemeral_5m_input_tokens":3,"ephemeral_1h_input_tokens":4REDACTEDREDACTEDREDACTEDREDACTED`

	svc.parseSSEUsagePassthrough(data, usage)

	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 9, usage.CacheReadInputTokens, "应兼容 cached_tokens 字段")
	require.Equal(t, 7, usage.CacheCreationInputTokens, "聚合字段为空时应从 5m/1h 明细回填")
	require.Equal(t, 3, usage.CacheCreation5mTokens)
	require.Equal(t, 4, usage.CacheCreation1hTokens)
REDACTED

func TestGatewayService_ParseSSEUsagePassthrough_MessageDeltaSelectiveOverwrite(t *testing.T) {
	svc := &GatewayService{REDACTED
	usage := &ClaudeUsage{
		InputTokens:           10,
		CacheCreation5mTokens: 2,
		CacheCreation1hTokens: 6,
REDACTED
	data := `{"type":"message_delta","usage":{"input_tokens":0,"output_tokens":5,"cache_creation_input_tokens":8,"cache_read_input_tokens":0,"cached_tokens":11,"cache_creation":{"ephemeral_5m_input_tokens":1,"ephemeral_1h_input_tokens":0REDACTEDREDACTEDREDACTED`

	svc.parseSSEUsagePassthrough(data, usage)

	require.Equal(t, 10, usage.InputTokens, "message_delta 中 0 值不应覆盖已有 input_tokens")
	require.Equal(t, 5, usage.OutputTokens)
	require.Equal(t, 8, usage.CacheCreationInputTokens)
	require.Equal(t, 11, usage.CacheReadInputTokens, "cache_read_input_tokens 为空时应回退到 cached_tokens")
	require.Equal(t, 1, usage.CacheCreation5mTokens)
	require.Equal(t, 6, usage.CacheCreation1hTokens, "message_delta 中 0 值不应覆盖已有 1h 明细")
REDACTED

func TestGatewayService_ParseSSEUsagePassthrough_NoopCases(t *testing.T) {
	svc := &GatewayService{REDACTED

	usage := &ClaudeUsage{InputTokens: 3REDACTED
	svc.parseSSEUsagePassthrough("", usage)
	require.Equal(t, 3, usage.InputTokens)

	svc.parseSSEUsagePassthrough("[DONE]", usage)
	require.Equal(t, 3, usage.InputTokens)

	svc.parseSSEUsagePassthrough("not-json", usage)
	require.Equal(t, 3, usage.InputTokens)

	// nil usage 不应 panic
	svc.parseSSEUsagePassthrough(`{"type":"message_start"REDACTED`, nil)
REDACTED

func TestGatewayService_ParseSSEUsagePassthrough_FallbackFromUsageNode(t *testing.T) {
	svc := &GatewayService{REDACTED
	usage := &ClaudeUsage{REDACTED
	data := `{"type":"content_block_delta","usage":{"cached_tokens":6,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":1REDACTEDREDACTEDREDACTED`

	svc.parseSSEUsagePassthrough(data, usage)

	require.Equal(t, 6, usage.CacheReadInputTokens)
	require.Equal(t, 3, usage.CacheCreationInputTokens)
REDACTED

func TestParseClaudeUsageFromResponseBody(t *testing.T) {
	t.Run("empty or missing usage", func(t *testing.T) {
		got := parseClaudeUsageFromResponseBody(nil)
		require.NotNil(t, got)
		require.Equal(t, 0, got.InputTokens)

		got = parseClaudeUsageFromResponseBody([]byte(`{"id":"x"REDACTED`))
		require.NotNil(t, got)
		require.Equal(t, 0, got.OutputTokens)
REDACTED)

	t.Run("parse all usage fields and fallback", func(t *testing.T) {
		body := []byte(`{"usage":{"input_tokens":21,"output_tokens":34,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cached_tokens":13,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":8REDACTEDREDACTEDREDACTED`)
		got := parseClaudeUsageFromResponseBody(body)
		require.Equal(t, 21, got.InputTokens)
		require.Equal(t, 34, got.OutputTokens)
		require.Equal(t, 13, got.CacheReadInputTokens, "cache_read_input_tokens 为空时应回退 cached_tokens")
		require.Equal(t, 13, got.CacheCreationInputTokens, "聚合字段为空时应由 5m/1h 回填")
		require.Equal(t, 5, got.CacheCreation5mTokens)
		require.Equal(t, 8, got.CacheCreation1hTokens)
REDACTED)

	t.Run("keep explicit aggregate values", func(t *testing.T) {
		body := []byte(`{"usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":9,"cache_read_input_tokens":7,"cached_tokens":99,"cache_creation":{"ephemeral_5m_input_tokens":4,"ephemeral_1h_input_tokens":5REDACTEDREDACTEDREDACTED`)
		got := parseClaudeUsageFromResponseBody(body)
		require.Equal(t, 9, got.CacheCreationInputTokens, "已显式提供聚合字段时不应被明细覆盖")
		require.Equal(t, 7, got.CacheReadInputTokens, "已显式提供 cache_read_input_tokens 时不应回退 cached_tokens")
REDACTED)
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingErrTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: 32,
		REDACTED,
	REDACTED,
REDACTED

	// Scanner 初始缓冲为 64KB，构造更长单行触发 bufio.ErrTooLong。
	longLine := "data: " + strings.Repeat("x", 80*1024)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(longLine)),
REDACTED

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2REDACTED, time.Now())
REDACTED
	require.ErrorIs(t, err, bufio.ErrTooLong)
	require.NotNil(t, result)
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
		REDACTED,
	REDACTED,
REDACTED

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: &streamReadCloser{
			err: context.Canceled,
	REDACTED,
REDACTED

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 3REDACTED, time.Now())
REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
REDACTED

func TestGatewayService_AnthropicAPIKeyPassthrough_StreamingUpstreamReadErrorAfterClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Writer = &failWriteResponseWriter{ResponseWriter: c.WriterREDACTED

	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
		REDACTED,
	REDACTED,
REDACTED

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
		Body: &streamReadCloser{
			payload: []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":8REDACTEDREDACTEDREDACTED` + "\n\n"),
			err:     io.ErrUnexpectedEOF,
	REDACTED,
REDACTED

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 4REDACTED, time.Now())
REDACTED
	require.NotNil(t, result)
	require.True(t, result.clientDisconnect)
	require.Equal(t, 8, result.usage.InputTokens)
REDACTED
