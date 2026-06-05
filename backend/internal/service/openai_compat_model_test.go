package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAICompatFailingWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
REDACTED

func (w *openAICompatFailingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed: client disconnected")
REDACTED
	w.writes++
	return w.ResponseWriter.Write(p)
REDACTED

type openAICompatBlockingReadCloser struct {
	data      []byte
	offset    int
	closed    chan struct{REDACTED
	closeOnce sync.Once
REDACTED

func newOpenAICompatBlockingReadCloser(data []byte) *openAICompatBlockingReadCloser {
	return &openAICompatBlockingReadCloser{
		data:   data,
		closed: make(chan struct{REDACTED),
REDACTED
REDACTED

func (r *openAICompatBlockingReadCloser) Read(p []byte) (int, error) {
	if r.offset < len(r.data) {
		n := copy(p, r.data[r.offset:])
		r.offset += n
		return n, nil
REDACTED
	<-r.closed
	return 0, io.EOF
REDACTED

func (r *openAICompatBlockingReadCloser) Close() error {
	r.closeOnce.Do(func() {
		close(r.closed)
REDACTED)
	return nil
REDACTED

func TestNormalizeOpenAICompatRequestedModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
REDACTED{
		{name: "gpt reasoning alias strips xhigh", input: "gpt-5.4-xhigh", want: "gpt-5.4"REDACTED,
		{name: "gpt reasoning alias strips none", input: "gpt-5.4-none", want: "gpt-5.4"REDACTED,
		{name: "codex max model stays intact", input: "gpt-5.1-codex-max", want: "gpt-5.1-codex-max"REDACTED,
		{name: "non openai model unchanged", input: "claude-opus-4-6", want: "claude-opus-4-6"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeOpenAICompatRequestedModel(tt.input))
	REDACTED)
REDACTED
REDACTED

func TestApplyOpenAICompatModelNormalization(t *testing.T) {
	t.Parallel()

	t.Run("derives xhigh from model suffix when output config missing", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{Model: "gpt-5.4-xhigh"REDACTED

		applyOpenAICompatModelNormalization(req)

		require.Equal(t, "gpt-5.4", req.Model)
		require.NotNil(t, req.OutputConfig)
		require.Equal(t, "max", req.OutputConfig.Effort)
REDACTED)

	t.Run("explicit output config wins over model suffix", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{
			Model:        "gpt-5.4-xhigh",
			OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "low"REDACTED,
	REDACTED

		applyOpenAICompatModelNormalization(req)

		require.Equal(t, "gpt-5.4", req.Model)
		require.NotNil(t, req.OutputConfig)
		require.Equal(t, "low", req.OutputConfig.Effort)
REDACTED)

	t.Run("non openai model is untouched", func(t *testing.T) {
		req := &apicompat.AnthropicRequest{Model: "claude-opus-4-6"REDACTED

		applyOpenAICompatModelNormalization(req)

		require.Equal(t, "claude-opus-4-6", req.Model)
		require.Nil(t, req.OutputConfig)
REDACTED)
REDACTED

func TestForwardAsAnthropic_NormalizesRoutingAndEffortForGpt54XHigh(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4-xhigh","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_compat"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
			"model_mapping": map[string]any{
				"gpt-5.4": "gpt-5.4",
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.4-xhigh", result.Model)
	require.Equal(t, "gpt-5.4", result.UpstreamModel)
	require.Equal(t, "gpt-5.4", result.BillingModel)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "xhigh", *result.ReasoningEffort)

	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "gpt-5.4-xhigh", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, "ok", gjson.GetBytes(rec.Body.Bytes(), "content.0.text").String())
	t.Logf("upstream body: %s", string(upstream.lastBody))
	t.Logf("response body: %s", rec.Body.String())
REDACTED

func TestForwardAsAnthropic_MappedClaudeModelAcceptsChatUsageShape(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-opus-4-7","max_tokens":16,"messages":[{"role":"user","content":"compact this"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_compact","model":"gpt-5.5","status":"in_progress","output":[]REDACTEDREDACTED`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"REDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_compact","object":"response","model":"gpt-5.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"prompt_tokens":31,"completion_tokens":9,"total_tokens":40,"prompt_tokens_details":{"cached_tokens":11REDACTEDREDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_compact_usage"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
			"model_mapping": map[string]any{
				"gpt-5.5": "gpt-5.5",
		REDACTED,
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "claude-opus-4-7", result.Model)
	require.Equal(t, "gpt-5.5", result.BillingModel)
	require.Equal(t, "gpt-5.5", result.UpstreamModel)
	require.Equal(t, 31, result.Usage.InputTokens)
	require.Equal(t, 9, result.Usage.OutputTokens)
	require.Equal(t, 11, result.Usage.CacheReadInputTokens)
	require.Equal(t, "gpt-5.5", gjson.GetBytes(upstream.lastBody, "model").String())
REDACTED

func TestForwardAsAnthropic_InjectsPromptCacheKeyForAPIKeyMessagesDispatch(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":{"user_id":"claude-session-1"REDACTED,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.3-codex","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":3REDACTEDREDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_cache_key"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "stable-cache-key", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, "gpt-5.3-codex", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
REDACTED

func TestForwardAsAnthropic_AutoDerivesPromptCacheKeyWhenMessagesDispatchHasNoSessionID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"system":"You are helpful.","messages":[{"role":"user","content":"open repo"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.3-codex","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7,"input_tokens_details":{"cached_tokens":3REDACTEDREDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_auto_cache_key"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, result)
	cacheKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.NotEmpty(t, cacheKey)
	require.True(t, strings.HasPrefix(cacheKey, "anthropic-digest-"))
	require.Equal(t, generateSessionUUID(isolateOpenAISessionID(0, cacheKey)), upstream.lastReq.Header.Get("session_id"))
REDACTED

func TestForwardAsAnthropic_DoesNotAutoDerivePromptCacheKeyForNonCodexModel(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-4o","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_no_cache_key"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-4o")
REDACTED
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
	require.Empty(t, upstream.lastReq.Header.Get("session_id"))
REDACTED

func TestForwardAsAnthropic_TrimsFullReplayOnlyForCodexCompatModels(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+3)
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"REDACTED`)
REDACTED
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[` + strings.Join(messages, ",") + `],"stream":falseREDACTED`)

	run := func(t *testing.T, mappedModel string) []byte {
	REDACTED

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		upstreamBody := strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"` + mappedModel + `","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n")
		upstream := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_trim"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	REDACTEDREDACTED

		svc := &OpenAIGatewayService{
			httpUpstream: upstream,
			cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
	REDACTED
		account := &Account{
			ID:          1,
			Name:        "openai-apikey",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
	REDACTED
				"api_key":  "sk-test",
				"base_url": "https://api.openai.com/v1",
		REDACTED,
	REDACTED

		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", mappedModel)
	REDACTED
		require.NotNil(t, result)
		return upstream.lastBody
REDACTED

	codexBody := run(t, "gpt-5.3-codex")
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+1), gjson.GetBytes(codexBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(codexBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(codexBody, "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "message-03", gjson.GetBytes(codexBody, "input.1.content.0.text").String())
	require.Equal(t, "message-14", gjson.GetBytes(codexBody, "input.12.content.0.text").String())

	nonCompatBody := run(t, "gpt-4o")
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+3), gjson.GetBytes(nonCompatBody, "input.#").Int())
	require.Equal(t, "message-00", gjson.GetBytes(nonCompatBody, "input.0.content.0.text").String())
REDACTED

func TestForwardAsAnthropic_OAuthCompatKeepsFullReplayForCacheGrowth(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+3)
	for i := 0; i < openAICompatAnthropicReplayMaxTailMessages+3; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"REDACTED`)
REDACTED
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[` + strings.Join(messages, ",") + `],"stream":falseREDACTED`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_trim", "gpt-5.4")REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+4), gjson.GetBytes(upstream.lastBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "message-00", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
	require.Equal(t, "message-14", gjson.GetBytes(upstream.lastBody, "input.15.content.0.text").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
REDACTED

func TestForwardAsAnthropic_AttachesPreviousResponseIDForCompatContinuation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED],"stream":falseREDACTED`)
	upstream.resp = openAICompatSSECompletedResponse("resp_first", "gpt-5.3-codex")
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, firstResult)
	require.Equal(t, "resp_first", firstResult.ResponseID)
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	upstream.resp = openAICompatSSECompletedResponse("resp_second", "gpt-5.3-codex")
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, "resp_second", secondResult.ResponseID)
	require.Equal(t, "resp_first", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())
	require.Equal(t, int64(2), gjson.GetBytes(upstream.lastBody, "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "second", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
REDACTED

func TestForwardAsAnthropic_PreviousResponseIDKeepsMultiToolCallContext(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"inspect files"REDACTED],"stream":falseREDACTED`)
	upstream.resp = openAICompatSSECompletedResponse("resp_first_tools", "gpt-5.3-codex")
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, firstResult)

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"inspect files"REDACTED,{"role":"assistant","content":[{"type":"tool_use","id":"call_one","name":"Read","input":{"file_path":"a.go"REDACTEDREDACTED,{"type":"tool_use","id":"call_two","name":"Read","input":{"file_path":"b.go"REDACTEDREDACTED]REDACTED,{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_one","content":"package a"REDACTED,{"type":"tool_result","tool_use_id":"call_two","content":"package b"REDACTED,{"type":"text","text":"continue"REDACTED]REDACTED],"tools":[{"name":"Read","description":"read a file","input_schema":{"type":"object","properties":{"file_path":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTED],"stream":falseREDACTED`)
	upstream.resp = openAICompatSSECompletedResponse("resp_second_tools", "gpt-5.3-codex")
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, "resp_first_tools", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())

	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.Equal(t, "call_one", gjson.GetBytes(upstream.lastBody, "input.1.call_id").String())
	require.Equal(t, "function_call", gjson.GetBytes(upstream.lastBody, "input.2.type").String())
	require.Equal(t, "call_two", gjson.GetBytes(upstream.lastBody, "input.2.call_id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.3.type").String())
	require.Equal(t, "call_one", gjson.GetBytes(upstream.lastBody, "input.3.call_id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.4.type").String())
	require.Equal(t, "call_two", gjson.GetBytes(upstream.lastBody, "input.4.call_id").String())
	require.Equal(t, "continue", gjson.GetBytes(upstream.lastBody, "input.5.content.0.text").String())
REDACTED

func TestForwardAsAnthropic_ReplaysWithoutContinuationWhenPreviousResponseMissing(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "stable-cache-key", "resp_missing")
	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	upstream.responses = []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid_prev_missing"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"previous_response_not_found","message":"previous response not found"REDACTEDREDACTED`)),
	REDACTED,
		openAICompatSSECompletedResponse("resp_replayed", "gpt-5.3-codex"),
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "resp_replayed", result.ResponseID)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "resp_missing", gjson.GetBytes(upstream.bodies[0], "previous_response_id").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
	require.Equal(t, int64(4), gjson.GetBytes(upstream.bodies[1], "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.bodies[1], "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "first", gjson.GetBytes(upstream.bodies[1], "input.1.content.0.text").String())
	require.Equal(t, "second", gjson.GetBytes(upstream.bodies[1], "input.3.content.0.text").String())
REDACTED

func TestForwardAsAnthropic_DisablesAPIKeyContinuationWhenUpstreamRequiresWebSocketV2(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "stable-cache-key", "resp_http_unsupported")
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	upstream.responses = []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid_prev_http_unsupported"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"previous_response_id is only supported on Responses WebSocket v2","type":"invalid_request_error"REDACTEDREDACTED`)),
	REDACTED,
		openAICompatSSECompletedResponse("resp_replayed", "gpt-5.5"),
		openAICompatSSECompletedResponse("resp_later", "gpt-5.5"),
REDACTED

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.5")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "resp_replayed", result.ResponseID)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "resp_http_unsupported", gjson.GetBytes(upstream.bodies[0], "previous_response_id").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())

	laterRec := httptest.NewRecorder()
	laterCtx, _ := gin.CreateTestContext(laterRec)
	laterCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	laterCtx.Request.Header.Set("Content-Type", "application/json")

	laterResult, err := svc.ForwardAsAnthropic(context.Background(), laterCtx, account, body, "stable-cache-key", "gpt-5.5")
REDACTED
	require.NotNil(t, laterResult)
	require.Equal(t, "resp_later", laterResult.ResponseID)
	require.Len(t, upstream.requests, 3)
	require.False(t, gjson.GetBytes(upstream.bodies[2], "previous_response_id").Exists())
REDACTED

func TestForwardAsAnthropic_APIKeyMetadataSessionSurvivesChangingCacheControlAnchorAfterContinuationDisabled(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	metadata := `{"user_id":"{\"device_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"account_uuid\":\"\",\"session_id\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\"REDACTED"REDACTED`
	firstBody := []byte(`{"model":"REDACTED","max_tokens":16,"metadata":` + metadata + `,"system":[{"type":"text","text":"project docs","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":"first"REDACTED],"stream":falseREDACTED`)
	messages := make([]string, 0, openAICompatAnthropicReplayMaxTailMessages+4)
	messages = append(messages, `{"role":"user","content":[{"type":"text","text":"rewritten context","cache_control":{"type":"ephemeral"REDACTEDREDACTED]REDACTED`)
	for i := 1; i < openAICompatAnthropicReplayMaxTailMessages+4; i++ {
		messages = append(messages, `{"role":"user","content":"message-`+fmt.Sprintf("%02d", i)+`"REDACTED`)
REDACTED
	secondBody := []byte(`{"model":"REDACTED","max_tokens":16,"metadata":` + metadata + `,"messages":[` + strings.Join(messages, ",") + `],"stream":falseREDACTED`)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAICompatSSECompletedResponse("resp_first", "gpt-5.4-mini"),
		openAICompatSSECompletedResponse("resp_second", "gpt-5.4-mini"),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.4-mini")
REDACTED
	require.NotNil(t, firstResult)
	firstKey := gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String()
	require.NotEmpty(t, firstKey)
	require.True(t, strings.HasPrefix(firstKey, "anthropic-metadata-"))

	svc.disableOpenAICompatSessionContinuation(context.Background(), nil, account, firstKey)

	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.4-mini")
REDACTED
	require.NotNil(t, secondResult)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, firstKey, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
	require.Equal(t, int64(openAICompatAnthropicReplayMaxTailMessages+5), gjson.GetBytes(upstream.bodies[1], "input.#").Int())
	require.Equal(t, "developer", gjson.GetBytes(upstream.bodies[1], "input.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "input.0.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "rewritten context", gjson.GetBytes(upstream.bodies[1], "input.1.content.0.text").String())
	require.Equal(t, "message-15", gjson.GetBytes(upstream.bodies[1], "input.16.content.0.text").String())
REDACTED

func TestForwardAsAnthropic_DoesNotAttachPreviousResponseIDForOAuthCompat(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_next", "gpt-5.4")REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED
	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, account, "stable-cache-key", "resp_oauth_prev")

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.4")
REDACTED
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
REDACTED

func TestForwardAsAnthropic_ReusesOAuthCodexTurnState(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_first", "gpt-5.4")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_second", "gpt-5.4"),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED],"stream":falseREDACTED`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.4")
REDACTED
	require.NotNil(t, firstResult)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[0].Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.requests[0].Header.Get("originator"))

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.4")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, "turn_state_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Equal(t, generateSessionUUID(isolateOpenAISessionID(0, "stable-cache-key")), upstream.requests[1].Header.Get("session_id"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.Empty(t, upstream.requests[1].Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.requests[1].Header.Get("originator"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
REDACTED

func TestForwardAsAnthropic_OAuthDigestFallbackReusesTurnStateWithoutExplicitKey(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_digest_first", "gpt-5.4")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_digest_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_digest_second", "gpt-5.4"),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED],"stream":falseREDACTED`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.4")
REDACTED
	require.NotNil(t, firstResult)
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.4")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, firstSessionID, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "turn_state_digest_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
REDACTED

func TestForwardAsAnthropic_OAuthMetadataSessionSurvivesDigestPrefixRewrite(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_metadata_first", "gpt-5.5")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_metadata_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_metadata_second", "gpt-5.5"),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED
	metadata := `{"user_id":"{\"device_id\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"account_uuid\":\"\",\"session_id\":\"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa\"REDACTED"REDACTED`

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"messages":[{"role":"user","content":"first plan"REDACTED],"stream":falseREDACTED`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.5")
REDACTED
	require.NotNil(t, firstResult)
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"messages":[{"role":"user","content":"rewritten plan"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.5")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, firstSessionID, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "turn_state_metadata_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
REDACTED

func TestForwardAsAnthropic_OAuthMetadataSessionSurvivesChangingCacheControlAnchor(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	firstResp := openAICompatSSECompletedResponse("resp_oauth_cache_anchor_first", "gpt-5.5")
	firstResp.Header.Set("x-codex-turn-state", "turn_state_cache_anchor_first")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		firstResp,
		openAICompatSSECompletedResponse("resp_oauth_cache_anchor_second", "gpt-5.5"),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED
	metadata := `{"user_id":"{\"device_id\":\"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\",\"account_uuid\":\"\",\"session_id\":\"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb\"REDACTED"REDACTED`

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"system":[{"type":"text","text":"anchor one","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":"first"REDACTED],"stream":falseREDACTED`)
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "", "gpt-5.5")
REDACTED
	require.NotNil(t, firstResult)
	firstSessionID := upstream.requests[0].Header.Get("session_id")
	require.NotEmpty(t, firstSessionID)
	require.Empty(t, upstream.requests[0].Header.Get("x-codex-turn-state"))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").Exists())

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"metadata":` + metadata + `,"system":[{"type":"text","text":"anchor two","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "", "gpt-5.5")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, firstSessionID, upstream.requests[1].Header.Get("session_id"))
	require.Equal(t, "turn_state_cache_anchor_first", upstream.requests[1].Header.Get("x-codex-turn-state"))
	require.Empty(t, upstream.requests[1].Header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(upstream.bodies[1], "prompt_cache_key").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "previous_response_id").Exists())
REDACTED

func TestForwardAsAnthropic_OAuthKeepsSystemAsDeveloperInput(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_system", "gpt-5.4")REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"system":[{"type":"text","text":"project instructions","cache_control":{"type":"ephemeral"REDACTEDREDACTED],"messages":[{"role":"user","content":"first"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Equal(t, "input_text", gjson.GetBytes(upstream.lastBody, "input.0.content.0.type").String())
	require.Equal(t, "project instructions", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
	instructions := gjson.GetBytes(upstream.lastBody, "instructions")
	require.True(t, instructions.Exists())
	require.Empty(t, instructions.String())
	require.Empty(t, upstream.requests[0].Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.requests[0].Header.Get("originator"))
REDACTED

func TestForwardAsAnthropic_OAuthAddsClaudeCodeTodoGuardForCompatModel(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_todo_guard", "gpt-5.5")REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"system":"project instructions","messages":[{"role":"user","content":"review files"REDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
	require.Equal(t, "project instructions", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
	require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.1.role").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String(), "<sub2api-claude-code-todo-guard>")
	require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.2.role").String())
REDACTED

func TestForwardAsAnthropic_OAuthPreservesClaudeCodeToolCallID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_oauth_tool", "gpt-5.4")REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"list files"REDACTED,{"role":"assistant","content":[{"type":"tool_use","id":"toolu_123","name":"Bash","input":{"command":"ls"REDACTEDREDACTED]REDACTED,{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"ok"REDACTED]REDACTED],"tools":[{"name":"Bash","description":"run shell","input_schema":{"type":"object","properties":{"command":{"type":"string"REDACTEDREDACTEDREDACTEDREDACTED],"stream":falseREDACTED`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "stable-cache-key", "gpt-5.4")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "toolu_123", gjson.GetBytes(upstream.lastBody, `input.#(type=="function_call").call_id`).String())
	require.Equal(t, "toolu_123", gjson.GetBytes(upstream.lastBody, `input.#(type=="function_call_output").call_id`).String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())
	require.Equal(t, "medium", gjson.GetBytes(upstream.lastBody, "text.verbosity").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools.0.strict").Bool())
REDACTED

func TestForwardAsAnthropic_StoresStreamingResponseIDWithoutUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: falseREDACTEDREDACTEDREDACTED,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1",
	REDACTED,
REDACTED

	firstBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED],"stream":trueREDACTED`)
	upstream.resp = openAICompatSSEResponseWithoutUsage("resp_stream_first", "gpt-5.3-codex")
	firstRec := httptest.NewRecorder()
	firstCtx, _ := gin.CreateTestContext(firstRec)
	firstCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(firstBody))
	firstCtx.Request.Header.Set("Content-Type", "application/json")

	firstResult, err := svc.ForwardAsAnthropic(context.Background(), firstCtx, account, firstBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, firstResult)
	require.Equal(t, "resp_stream_first", firstResult.ResponseID)

	secondBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED],"stream":falseREDACTED`)
	upstream.resp = openAICompatSSECompletedResponse("resp_stream_second", "gpt-5.3-codex")
	secondRec := httptest.NewRecorder()
	secondCtx, _ := gin.CreateTestContext(secondRec)
	secondCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(secondBody))
	secondCtx.Request.Header.Set("Content-Type", "application/json")

	secondResult, err := svc.ForwardAsAnthropic(context.Background(), secondCtx, account, secondBody, "stable-cache-key", "gpt-5.3-codex")
REDACTED
	require.NotNil(t, secondResult)
	require.Equal(t, "resp_stream_first", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())
REDACTED

func openAICompatSSECompletedResponse(responseID, model string) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"` + responseID + `","object":"response","model":"` + model + `","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_continuation"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED
REDACTED

func openAICompatSSEResponseWithoutUsage(responseID, model string) *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"` + responseID + `","object":"response","model":"` + model + `","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED]REDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_" + responseIDREDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(body)),
REDACTED
REDACTED

func TestForwardAsAnthropic_ForcedCodexInstructionsTemplatePrependsRenderedInstructions(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	templateDir := t.TempDir()
	templatePath := filepath.Join(templateDir, "codex-instructions.md.tmpl")
	require.NoError(t, os.WriteFile(templatePath, []byte("server-prefix\n\n{{ .ExistingInstructions REDACTEDREDACTED"), 0o644))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"system":"client-system","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_forced"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			ForcedCodexInstructionsTemplateFile: templatePath,
			ForcedCodexInstructionsTemplate:     "server-prefix\n\n{{ .ExistingInstructions REDACTEDREDACTED",
REDACTED
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "server-prefix\n\nclient-system", gjson.GetBytes(upstream.lastBody, "instructions").String())
REDACTED

func TestForwardAsAnthropic_ForcedCodexInstructionsTemplateUsesCachedTemplateContent(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"system":"client-system","messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_forced_cached"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			ForcedCodexInstructionsTemplateFile: "/path/that/should/not/be/read.tmpl",
			ForcedCodexInstructionsTemplate:     "cached-prefix\n\n{{ .ExistingInstructions REDACTEDREDACTED",
REDACTED
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "cached-prefix\n\nclient-system", gjson.GetBytes(upstream.lastBody, "instructions").String())
REDACTED

func TestForwardAsAnthropic_ClientDisconnectDrainsUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]REDACTEDREDACTED`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"REDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":3REDACTEDREDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_disconnect"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
REDACTED

func TestForwardAsAnthropic_TerminalUsageWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5REDACTEDREDACTEDREDACTEDREDACTED` + "\n\n")
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
REDACTED()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_terminal_no_close"REDACTEDREDACTED,
		Body:       upstreamStream,
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
REDACTED
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: errREDACTED
REDACTED()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic should return after terminal usage event even if upstream keeps the connection open")
REDACTED
REDACTED

func TestForwardAsAnthropic_EventNamedTerminalWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(strings.Join([]string{
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5REDACTEDREDACTEDREDACTEDREDACTED`,
		``,
		``,
REDACTED, "\n"))
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
REDACTED()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_messages_event_named_terminal"REDACTEDREDACTED,
		Body:       upstreamStream,
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
REDACTED
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: errREDACTED
REDACTED()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic should use SSE event names when data payloads omit type")
REDACTED
REDACTED

func TestForwardAsAnthropic_EventNamedTerminalWithKeepaliveReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(strings.Join([]string{
		`: upstream ping`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5REDACTEDREDACTEDREDACTEDREDACTED`,
		``,
		``,
REDACTED, "\n"))
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
REDACTED()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_messages_event_named_keepalive"REDACTEDREDACTED,
		Body:       upstreamStream,
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			StreamKeepaliveInterval: 5,
REDACTED
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
REDACTED
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: errREDACTED
REDACTED()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic keepalive path should use SSE event names when data payloads omit type")
REDACTED
REDACTED

func TestForwardAsAnthropic_BufferedTerminalWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5REDACTEDREDACTEDREDACTEDREDACTED` + "\n\n")
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
REDACTED()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_buffered_terminal_no_close"REDACTEDREDACTED,
		Body:       upstreamStream,
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
REDACTED
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: errREDACTED
REDACTED()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
		require.Contains(t, rec.Body.String(), `"stop_reason":"end_turn"`)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic buffered response should return after terminal usage event even if upstream keeps the connection open")
REDACTED
REDACTED

func TestForwardAsAnthropic_BufferedEventNamedTerminalWithoutUpstreamCloseReturns(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := []byte(strings.Join([]string{
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":15,"output_tokens":6,"total_tokens":21,"input_tokens_details":{"cached_tokens":5REDACTEDREDACTEDREDACTEDREDACTED`,
		``,
		``,
REDACTED, "\n"))
	upstreamStream := newOpenAICompatBlockingReadCloser(upstreamBody)
	defer func() {
		require.NoError(t, upstreamStream.Close())
REDACTED()
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_messages_buffered_event_named"REDACTEDREDACTED,
		Body:       upstreamStream,
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	type forwardResult struct {
		result *OpenAIForwardResult
		err    error
REDACTED
	resultCh := make(chan forwardResult, 1)
	go func() {
		result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
		resultCh <- forwardResult{result: result, err: errREDACTED
REDACTED()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 15, got.result.Usage.InputTokens)
		require.Equal(t, 6, got.result.Usage.OutputTokens)
		require.Equal(t, 5, got.result.Usage.CacheReadInputTokens)
		require.Contains(t, rec.Body.String(), `"stop_reason":"end_turn"`)
	case <-time.After(time.Second):
		require.Fail(t, "ForwardAsAnthropic buffered response should use SSE event names when data payloads omit type")
REDACTED
REDACTED

func TestForwardAsAnthropic_MissingTerminalBeforeOutputReturnsFailoverAndOps(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := "data: [DONE]\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid_missing_terminal"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "missing terminal before output must use failover path")
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "OpenAI messages stream ended before a terminal event")
	require.NotNil(t, result)
	require.Zero(t, result.Usage.InputTokens)
	require.Zero(t, result.Usage.OutputTokens)
	require.False(t, c.Writer.Written(), "no client body/header should be committed before safe failover")
	require.Empty(t, rec.Body.String())

	events := openAICompatOpsEvents(t, c)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, http.StatusBadGateway, events[0].UpstreamStatusCode)
	require.Equal(t, int64(1), events[0].AccountID)
	require.Equal(t, "rid_missing_terminal", events[0].UpstreamRequestID)
	require.Contains(t, events[0].Message, "terminal event")
REDACTED

func TestForwardAsAnthropic_MissingTerminalAfterOutputRecordsOpsWithoutFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]REDACTEDREDACTED`,
		"",
		`data: {"type":"response.output_text.delta","delta":"partial"REDACTED`,
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid_partial_missing_terminal"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.Contains(t, err.Error(), "missing terminal event")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "partial output must not be replayed through failover")
	require.NotNil(t, result)
	require.False(t, result.ClientDisconnect)
	require.True(t, c.Writer.Written())
	require.Contains(t, rec.Body.String(), "event: message_start")
	require.Contains(t, rec.Body.String(), "partial")

	events := openAICompatOpsEvents(t, c)
	require.Len(t, events, 1)
	require.Equal(t, "stream_missing_terminal", events[0].Kind)
	require.Equal(t, http.StatusBadGateway, events[0].UpstreamStatusCode)
	require.Equal(t, int64(1), events[0].AccountID)
	require.Equal(t, "rid_partial_missing_terminal", events[0].UpstreamRequestID)
	require.Contains(t, events[0].Message, "terminal event")
REDACTED

func TestForwardAsAnthropic_MissingTerminalAfterClientDisconnectSkipsOpsAndFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAICompatFailingWriter{ResponseWriter: c.Writer, failAfter: 0REDACTED
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]REDACTEDREDACTED`,
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid_client_disconnect_missing_terminal"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.Contains(t, err.Error(), "missing terminal event")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Empty(t, rec.Body.String())
	_, ok := c.Get(OpsUpstreamErrorsKey)
	require.False(t, ok, "client disconnect must not be attributed as an upstream error")
REDACTED

func TestForwardAsAnthropic_CompleteStreamDoesNotRecordMissingTerminalOps(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":trueREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4","status":"in_progress","output":[]REDACTEDREDACTED`,
		"",
		`data: {"type":"response.output_text.delta","delta":"ok"REDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid_complete_terminal"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.1")
REDACTED
	require.NotNil(t, result)
	require.Equal(t, 9, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), "event: message_stop")
	_, ok := c.Get(OpsUpstreamErrorsKey)
	require.False(t, ok)
REDACTED

func openAICompatOpsEvents(t *testing.T, c *gin.Context) []*OpsUpstreamErrorEvent {
REDACTED
	v, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := v.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	return events
REDACTED

func TestForwardAsAnthropic_UpstreamRequestIgnoresClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	reqCtx, cancel := context.WithCancel(context.Background())
	body := []byte(`{"model":"gpt-5.4","max_tokens":16,"messages":[{"role":"user","content":"hello"REDACTED],"stream":falseREDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body)).WithContext(reqCtx)
	c.Request.Header.Set("Content-Type", "application/json")
	cancel()

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"REDACTED]REDACTED],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_ctx"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{httpUpstream: upstreamREDACTED
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
REDACTED

	result, err := svc.ForwardAsAnthropic(reqCtx, c, account, body, "", "gpt-5.1")
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
REDACTED
