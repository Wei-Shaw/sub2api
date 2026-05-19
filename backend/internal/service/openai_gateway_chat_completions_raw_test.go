//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOpenAIChatCompletionsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
REDACTED{
		// 已是 /chat/completions：原样返回
		{"already chat/completions", "https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1/chat/completions"REDACTED,
		// 以 /v1 结尾：追加 /chat/completions
		{"bare /v1", "https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"REDACTED,
		// 其他情况：追加 /v1/chat/completions
		{"bare domain", "https://api.openai.com", "https://api.openai.com/v1/chat/completions"REDACTED,
		{"domain with trailing slash", "https://api.openai.com/", "https://api.openai.com/v1/chat/completions"REDACTED,
		// 第三方上游常见形式
		{"third-party bare domain", "https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"REDACTED,
		{"third-party with path prefix", "https://api.gptgod.online/api", "https://api.gptgod.online/api/v1/chat/completions"REDACTED,
		{"third-party versioned path", "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4/chat/completions"REDACTED,
		// 带空白字符
		{"whitespace trimmed", "  https://api.openai.com/v1  ", "https://api.openai.com/v1/chat/completions"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildOpenAIChatCompletionsURL(tt.base)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

// TestBuildOpenAIResponsesURL_ProbeURL 锁定 probe/测试端点使用的 URL 构建逻辑，
// 确保 buildOpenAIResponsesURL 对标准 OpenAI base_url 格式均拼出 `/v1/responses`。
func TestBuildOpenAIResponsesURL_ProbeURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
REDACTED{
		{"bare domain", "https://api.openai.com", "https://api.openai.com/v1/responses"REDACTED,
		{"domain trailing slash", "https://api.openai.com/", "https://api.openai.com/v1/responses"REDACTED,
		{"bare /v1", "https://api.openai.com/v1", "https://api.openai.com/v1/responses"REDACTED,
		{"already /responses", "https://api.openai.com/v1/responses", "https://api.openai.com/v1/responses"REDACTED,
		{"third-party bare domain", "https://api.deepseek.com", "https://api.deepseek.com/v1/responses"REDACTED,
		{"third-party versioned path", "https://open.bigmodel.cn/api/paas/v4", "https://open.bigmodel.cn/api/paas/v4/responses"REDACTED,
		{"only domain, no scheme", "api.gptgod.online", "api.gptgod.online/v1/responses"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildOpenAIResponsesURL(tt.base)
			require.Equal(t, tt.want, got)
	REDACTED)
REDACTED
REDACTED

func TestForwardAsRawChatCompletions_ForcesStreamUsageUpstreamAndPassesUsageDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"ok"REDACTEDREDACTED]REDACTED`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13,"prompt_tokens_details":{"cached_tokens":3REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_raw_usage"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED
	account := rawChatCompletionsTestAccount()

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Contains(t, rec.Body.String(), `"usage"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
REDACTED

func TestForwardAsRawChatCompletions_ClientDisconnectDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"ok"REDACTEDREDACTED]REDACTED`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":17,"completion_tokens":8,"total_tokens":25,"prompt_tokens_details":{"cached_tokens":6REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_raw_disconnect"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED
	account := rawChatCompletionsTestAccount()

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 17, result.Usage.InputTokens)
	require.Equal(t, 8, result.Usage.OutputTokens)
	require.Equal(t, 6, result.Usage.CacheReadInputTokens)
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
REDACTED

func TestForwardAsRawChatCompletions_UpstreamRequestIgnoresClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reqCtx, cancel := context.WithCancel(context.Background())
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(reqCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	cancel()

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7REDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_raw_ctx"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED
	account := rawChatCompletionsTestAccount()

	result, err := svc.forwardAsRawChatCompletions(reqCtx, c, account, body, "")
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
REDACTED

func TestForwardAsChatCompletions_UnknownResponsesSupportFallbackUsesVersionedChatURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"glm-4.5-air","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"not found"REDACTEDREDACTED`)),
	REDACTED,
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid_raw_fallback"REDACTEDREDACTED,
			Body: io.NopCloser(strings.NewReader(
				`{"id":"chatcmpl_1","object":"chat.completion","model":"glm-4.5-air","choices":[{"index":0,"message":{"role":"assistant","content":"ok"REDACTED,"finish_reason":"stop"REDACTED],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3REDACTEDREDACTED`,
			)),
	REDACTED,
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
REDACTED
	account := rawChatCompletionsTestAccount()
	account.Credentials["base_url"] = "https://open.bigmodel.cn/api/paas/v4"

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 1, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/responses", upstream.requests[0].URL.String())
	require.Equal(t, "https://open.bigmodel.cn/api/paas/v4/chat/completions", upstream.requests[1].URL.String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"content":"ok"`)
REDACTED

func TestIsOpenAIChatUsageOnlyStreamChunk(t *testing.T) {
	t.Parallel()

	require.True(t, isOpenAIChatUsageOnlyStreamChunk(`{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2REDACTEDREDACTED`))
	require.False(t, isOpenAIChatUsageOnlyStreamChunk(`{"choices":[{"index":0REDACTED],"usage":{"prompt_tokens":1,"completion_tokens":2REDACTEDREDACTED`))
	require.False(t, isOpenAIChatUsageOnlyStreamChunk(`{"choices":[]REDACTED`))
	require.False(t, isOpenAIChatUsageOnlyStreamChunk(``))
REDACTED

func TestEnsureOpenAIChatStreamUsage(t *testing.T) {
	t.Parallel()

	body, err := ensureOpenAIChatStreamUsage([]byte(`{"model":"gpt-5.4"REDACTED`))
REDACTED
	require.True(t, gjson.GetBytes(body, "stream_options.include_usage").Bool())

	body, err = ensureOpenAIChatStreamUsage([]byte(`{"model":"gpt-5.4","stream_options":{"include_usage":falseREDACTEDREDACTED`))
REDACTED
	require.True(t, gjson.GetBytes(body, "stream_options.include_usage").Bool())
REDACTED

func TestBufferRawChatCompletions_RejectsOversizedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("toolong")),
REDACTED
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()REDACTED
	svc.cfg.Gateway.UpstreamResponseReadMaxBytes = 3

	result, err := svc.bufferRawChatCompletions(c, resp, "gpt-5.4", "gpt-5.4", "gpt-5.4", nil, nil, time.Now())
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, rec.Code)
REDACTED

func rawChatCompletionsTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func rawChatCompletionsTestAccount() *Account {
REDACTED
		ID:          101,
		Name:        "raw-openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
	REDACTED,
REDACTED
REDACTED
