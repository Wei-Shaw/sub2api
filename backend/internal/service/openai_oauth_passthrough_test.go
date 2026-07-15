package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func f64p(v float64) *float64 { return &v REDACTED

type httpUpstreamRecorder struct {
	lastReq      *http.Request
	lastBody     []byte
	lastProxyURL string
	requests     []*http.Request
	bodies       [][]byte

	resp      *http.Response
	responses []*http.Response
	err       error
REDACTED

type passthroughErrReadCloser struct {
	err error
REDACTED

type passthroughCloseTrackingReadCloser struct {
	io.Reader
	closed bool
REDACTED

func (r *passthroughCloseTrackingReadCloser) Close() error {
	r.closed = true
	return nil
REDACTED

func (r passthroughErrReadCloser) Read(_ []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
REDACTED
	return 0, io.ErrUnexpectedEOF
REDACTED

func (r passthroughErrReadCloser) Close() error {
	return nil
REDACTED

func (u *httpUpstreamRecorder) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.lastReq = req
	u.lastProxyURL = proxyURL
	if req != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		u.lastBody = b
		u.bodies = append(u.bodies, append([]byte(nil), b...))
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
REDACTED
	u.requests = append(u.requests, req)
	if u.err != nil {
		return nil, u.err
REDACTED
	if len(u.responses) > 0 {
		resp := u.responses[0]
		u.responses = u.responses[1:]
		return resp, nil
REDACTED
	return u.resp, nil
REDACTED

func (u *httpUpstreamRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

func TestOpenAIGatewayService_ResponsesUnknownModelDoesNotFallbackToGPT54(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	originalBody := []byte(`{"model":"gpt6","stream":false,"instructions":"local-test-instructions","input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(originalBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid_unknown_model"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"model not found"REDACTEDREDACTED`)),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{REDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          123,
		Name:        "acc",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.Nil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", upstream.lastReq.URL.String())
	require.Equal(t, "gpt6", gjson.GetBytes(upstream.lastBody, "model").String())
	require.NotEqual(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, rec.Code >= http.StatusBadRequest)
REDACTED

func TestOpenAIGatewayService_NativeResponsesBodyModificationPreservesHTMLChars(t *testing.T) {
	gin.SetMode(gin.TestMode)

	payloadText := strings.Repeat(`<tag>&value</tag>`, 128)
	originalBody := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":false,"max_output_tokens":100,"previous_response_id":"resp_prev","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":%qREDACTED]REDACTED]REDACTED`, payloadText))
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(originalBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid_native_reencode"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop after capture"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled:           false,
			AllowInsecureHTTP: true,
	REDACTEDREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          456,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
REDACTED
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
	REDACTED,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
			openai_compat.ExtraKeyResponsesSupported: true,
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.Nil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.Contains(t, string(upstream.lastBody), payloadText)
	require.NotContains(t, string(upstream.lastBody), `\\u003c`)
	require.NotContains(t, string(upstream.lastBody), `\\u003e`)
	require.NotContains(t, string(upstream.lastBody), `\\u0026`)
REDACTED

func TestOpenAIGatewayService_OAuthMessagesBridgeDoesNotInjectDefaultInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	originalBody := []byte(`{"model":"gpt-5.5","stream":true,"prompt_cache_key":"anthropic-metadata-session-1","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"<sub2api-claude-code-todo-guard>"REDACTED]REDACTED,{"type":"message","role":"user","content":"hello"REDACTED]REDACTED`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(originalBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid_bridge"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"bridge stop"REDACTEDREDACTED`)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{REDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:          123,
		Name:        "acc",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
REDACTED
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
	REDACTED,
		Status:      StatusActive,
		Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.Nil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "", gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").Exists())
	require.NotEmpty(t, upstream.lastReq.Header.Get("Session_Id"))
	require.Empty(t, upstream.lastReq.Header.Get("Conversation_Id"))
	require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.Empty(t, upstream.lastReq.Header.Get("originator"))
REDACTED

type openAIPassthroughFailoverRepo struct {
	stubOpenAIAccountRepo
	rateLimitCalls []time.Time
	overloadCalls  []time.Time
REDACTED

func (r *openAIPassthroughFailoverRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls = append(r.rateLimitCalls, resetAt)
	return nil
REDACTED

func (r *openAIPassthroughFailoverRepo) SetOverloaded(_ context.Context, _ int64, until time.Time) error {
	r.overloadCalls = append(r.overloadCalls, until)
	return nil
REDACTED

var structuredLogCaptureMu sync.Mutex

type inMemoryLogSink struct {
	mu     sync.Mutex
	events []*logger.LogEvent
REDACTED

func (s *inMemoryLogSink) WriteLogEvent(event *logger.LogEvent) {
	if event == nil {
		return
REDACTED
	cloned := *event
	if event.Fields != nil {
		cloned.Fields = make(map[string]any, len(event.Fields))
		for k, v := range event.Fields {
			cloned.Fields[k] = v
	REDACTED
REDACTED
	s.mu.Lock()
	s.events = append(s.events, &cloned)
	s.mu.Unlock()
REDACTED

func (s *inMemoryLogSink) ContainsMessage(substr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev != nil && strings.Contains(ev.Message, substr) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *inMemoryLogSink) ContainsMessageAtLevel(substr, level string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	wantLevel := strings.ToLower(strings.TrimSpace(level))
	for _, ev := range s.events {
		if ev == nil {
			continue
	REDACTED
		if strings.Contains(ev.Message, substr) && strings.ToLower(strings.TrimSpace(ev.Level)) == wantLevel {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *inMemoryLogSink) ContainsFieldValue(field, substr string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev == nil || ev.Fields == nil {
			continue
	REDACTED
		if v, ok := ev.Fields[field]; ok && strings.Contains(fmt.Sprint(v), substr) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *inMemoryLogSink) ContainsField(field string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev == nil || ev.Fields == nil {
			continue
	REDACTED
		if _, ok := ev.Fields[field]; ok {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func captureStructuredLog(t *testing.T) (*inMemoryLogSink, func()) {
REDACTED
	structuredLogCaptureMu.Lock()

	err := logger.Init(logger.InitOptions{
		Level:       "debug",
		Format:      "json",
		ServiceName: "sub2api",
		Environment: "test",
		Output: logger.OutputOptions{
			ToStdout: true,
			ToFile:   false,
	REDACTED,
		Sampling: logger.SamplingOptions{Enabled: falseREDACTED,
REDACTED)
REDACTED

	sink := &inMemoryLogSink{REDACTED
	logger.SetSink(sink)
	return sink, func() {
		logger.SetSink(nil)
		structuredLogCaptureMu.Unlock()
REDACTED
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_StreamKeepsToolNameAndBodyNormalized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Authorization", "Bearer inbound-should-not-forward")
	c.Request.Header.Set("Cookie", "secret=1")
	c.Request.Header.Set("X-Api-Key", "sk-inbound")
	c.Request.Header.Set("X-Goog-Api-Key", "goog-inbound")
	c.Request.Header.Set("Accept-Encoding", "gzip")
	c.Request.Header.Set("Proxy-Authorization", "Basic abc")
	c.Request.Header.Set("X-Test", "keep")
	c.Request.Header.Set("x-codex-beta-features", "remote_compaction_v2")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"store":true,"instructions":"local-test-instructions","input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	upstreamSSE := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"tool_call","tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
		openAITokenProvider: &OpenAITokenProvider{ // minimal: will be bypassed by nil cache/service, but GetAccessToken uses provider only if non-nil
			accountRepo: nil,
	REDACTED,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	// Use the gateway method that reads token from credentials when provider is nil.
	svc.openAITokenProvider = nil

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)

	// 1) 透传 OAuth 请求体与旧链路关键行为保持一致：store=false + stream=true。
	require.Equal(t, false, gjson.GetBytes(upstream.lastBody, "store").Bool())
	require.Equal(t, true, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "local-test-instructions", strings.TrimSpace(gjson.GetBytes(upstream.lastBody, "instructions").String()))
	// 其余关键字段保持原值。
	require.Equal(t, "gpt-5.2", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "hi", gjson.GetBytes(upstream.lastBody, "input.0.text").String())

	// 2) only auth is replaced; inbound auth/cookie are not forwarded
	require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "codex_cli_rs/0.1.0", upstream.lastReq.Header.Get("User-Agent"))
	require.Empty(t, upstream.lastReq.Header.Get("Cookie"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Api-Key"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Goog-Api-Key"))
	require.Empty(t, upstream.lastReq.Header.Get("Accept-Encoding"))
	require.Empty(t, upstream.lastReq.Header.Get("Proxy-Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Test"))
	require.Equal(t, "remote_compaction_v2", upstream.lastReq.Header.Get("x-codex-beta-features"))

	// 3) required OAuth headers are present
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))

	// 4) downstream SSE keeps tool name (no toolCorrector)
	body := rec.Body.String()
	require.Contains(t, body, "apply_patch")
	require.NotContains(t, body, "\"name\":\"edit\"")
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_NamespaceRequestAndStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")

	originalBody := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"instructions":"local-test-instructions",
		"tools":[
			{"type":"function","name":"plain","description":"keep","parameters":{"type":"object"REDACTEDREDACTED,
			{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","description":"spawn","parameters":{"type":"object"REDACTEDREDACTED]REDACTED
		],
		"tool_choice":{"type":"function","name":"spawn_agent","namespace":"collaboration"REDACTED,
		"input":[{"type":"function_call","call_id":"call_old","name":"spawn_agent","namespace":"collaboration","arguments":"{REDACTED"REDACTED]
REDACTED`)

	upstreamSSE := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"collaboration__spawn_agent","arguments":""REDACTEDREDACTED`,
		"",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"collaboration__spawn_agent","arguments":"{REDACTED"REDACTEDREDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"collaboration__spawn_agent","arguments":"{REDACTED"REDACTED],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_namespace"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	account := &Account{
		ID: 123, Name: "acc", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:       map[string]any{"openai_passthrough": trueREDACTED, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, result)

	require.Len(t, gjson.GetBytes(upstream.lastBody, "tools").Array(), 2)
	require.Equal(t, "plain", gjson.GetBytes(upstream.lastBody, "tools.0.name").String())
	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.1.type").String())
	require.Equal(t, "collaboration__spawn_agent", gjson.GetBytes(upstream.lastBody, "tools.1.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools.1.tools").Exists())
	require.Equal(t, "collaboration__spawn_agent", gjson.GetBytes(upstream.lastBody, "tool_choice.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice.namespace").Exists())
	require.Equal(t, "collaboration__spawn_agent", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.0.namespace").Exists())

	downstream := rec.Body.String()
	require.NotContains(t, downstream, "collaboration__spawn_agent")
	require.Contains(t, downstream, `"name":"spawn_agent"`)
	require.Contains(t, downstream, `"namespace":"collaboration"`)
REDACTED

func TestOpenAIGatewayService_NativeOAuth_NamespaceRequestAndStreamResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	body := []byte(`{
		"model":"gpt-5.5","stream":true,"instructions":"test",
		"tools":[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"REDACTEDREDACTED]REDACTED],
		"input":[{"type":"function_call","call_id":"call_old","name":"spawn_agent","namespace":"collaboration","arguments":"{REDACTED"REDACTED]
REDACTED`)
	upstreamSSE := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"collaboration__spawn_agent","arguments":""REDACTEDREDACTED`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"collaboration__spawn_agent","arguments":"{REDACTED"REDACTED],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3REDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_native_namespace"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
REDACTEDREDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	account := &Account{
		ID: 124, Name: "native", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Status:      StatusActive, Schedulable: true, RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.NotNil(t, result)
	require.Equal(t, "function", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "collaboration__spawn_agent", gjson.GetBytes(upstream.lastBody, "tools.0.name").String())
	require.Equal(t, "collaboration__spawn_agent", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.0.namespace").Exists())
	require.NotContains(t, rec.Body.String(), "collaboration__spawn_agent")
	require.Contains(t, rec.Body.String(), `"name":"spawn_agent"`)
	require.Contains(t, rec.Body.String(), `"namespace":"collaboration"`)
REDACTED

func TestOpenAIGatewayService_NativeOAuth_NamespaceNonStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	setOpenAIResponsesNamespaceNames(c, map[string]apicompat.ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"REDACTED,
REDACTED)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_1","output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"call_1","arguments":"{REDACTED"REDACTED],
			"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2REDACTED
	REDACTED`)),
REDACTED

	result, err := (&OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED).handleNonStreamingResponse(
		context.Background(), resp, c, &Account{Type: AccountTypeOAuthREDACTED, "gpt-5.5", "gpt-5.5",
	)
REDACTED
	require.NotNil(t, result)
	require.NotContains(t, rec.Body.String(), "collaboration__spawn_agent")
	require.Contains(t, rec.Body.String(), `"name":"spawn_agent"`)
	require.Contains(t, rec.Body.String(), `"namespace":"collaboration"`)
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_NamespaceNonStreamingResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"call_1","arguments":"{REDACTED"REDACTED],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
REDACTED
	names := map[string]apicompat.ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"REDACTED,
REDACTED
	setOpenAIResponsesNamespaceNames(c, names)

	result, err := (&OpenAIGatewayService{cfg: &config.Config{REDACTEDREDACTED).handleNonStreamingResponsePassthrough(
		context.Background(), resp, c, "gpt-5.5", "",
	)
REDACTED
	require.NotNil(t, result)
	require.NotContains(t, rec.Body.String(), "collaboration__spawn_agent")
	require.Contains(t, rec.Body.String(), `"name":"spawn_agent"`)
	require.Contains(t, rec.Body.String(), `"namespace":"collaboration"`)
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_NamespaceCollisionReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	body := []byte(`{
		"model":"gpt-5.5","stream":true,"instructions":"test",
		"tools":[
			{"type":"function","name":"collaboration__spawn_agent","parameters":{"type":"object"REDACTEDREDACTED,
			{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"REDACTEDREDACTED]REDACTED
		],"input":"hi"
REDACTED`)
	upstream := &httpUpstreamRecorder{REDACTED
	svc := &OpenAIGatewayService{cfg: &config.Config{REDACTED, httpUpstream: upstreamREDACTED
	account := &Account{
		ID: 123, Name: "acc", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
REDACTED"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:       map[string]any{"openai_passthrough": trueREDACTED, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, body)
REDACTED
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Equal(t, "tools", gjson.Get(rec.Body.String(), "error.param").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), "conflicts with a top-level tool")
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_CompactUsesJSONAndKeepsNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Content-Type", "application/json")

	originalBody := []byte(`{"model":"gpt-5.1-codex","stream":true,"store":true,"instructions":"local-test-instructions","input":[{"type":"text","text":"compact me"REDACTED]REDACTED`)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-compact"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_123","usage":{"input_tokens":11,"output_tokens":22REDACTEDREDACTED`)),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, result)
	require.False(t, result.Stream)

	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Exists())
	require.Equal(t, "gpt-5.1-codex", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "compact me", gjson.GetBytes(upstream.lastBody, "input.0.text").String())
	require.Equal(t, "local-test-instructions", strings.TrimSpace(gjson.GetBytes(upstream.lastBody, "instructions").String()))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, upstream.lastReq.Header.Get("Version"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("Session_Id"))
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Contains(t, rec.Body.String(), `"id":"cmp_123"`)
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_UpstreamRequestIgnoresClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	reqCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil)).WithContext(reqCtx)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	cancel()

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"store":true,"instructions":"local-test-instructions","input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_passthrough_ctx"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":1REDACTEDREDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n"))),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": true, "openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOffREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(reqCtx, c, account, originalBody)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_CodexMissingInstructionsRejectedBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?trace=1", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0 (Windows 10.0.19045; x86_64) unknown")
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("OpenAI-Beta", "responses=experimental")

	// Codex 模型且缺少 instructions，应在本地直接 403 拒绝，不触达上游。
	originalBody := []byte(`{"model":"gpt-5.1-codex-max","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"output":[],"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
	REDACTED,
REDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.Nil(t, result)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "requires a non-empty instructions field")
	require.Nil(t, upstream.lastReq)

	require.True(t, logSink.ContainsMessage("OpenAI passthrough 本地拦截：Codex 请求缺少有效 instructions"))
	require.True(t, logSink.ContainsFieldValue("request_user_agent", "codex_cli_rs/0.98.0 (Windows 10.0.19045; x86_64) unknown"))
	require.True(t, logSink.ContainsFieldValue("reject_reason", "instructions_missing"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_DisabledUsesLegacyTransform(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

	// store=true + stream=false should be forced to store=false + stream=true by applyCodexOAuthTransform (OAuth legacy path)
	inputBody := []byte(`{"model":"gpt-5.2","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": falseREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, inputBody)
REDACTED

	// legacy path rewrites request body (not byte-equal)
	require.NotEqual(t, inputBody, upstream.lastBody)
	require.Contains(t, string(upstream.lastBody), `"store":false`)
	require.Contains(t, string(upstream.lastBody), `"stream":true`)
REDACTED

func TestOpenAIGatewayService_OAuthLegacy_UpstreamRequestIgnoresClientCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	reqCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil)).WithContext(reqCtx)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	cancel()

	originalBody := []byte(`{"model":"gpt-5.2","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid_legacy_ctx"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n"))),
REDACTEDREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": false, "openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOffREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(reqCtx, c, account, originalBody)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.NoError(t, upstream.lastReq.Context().Err())
REDACTED

func TestOpenAIGatewayService_OAuthLegacy_CompositeCodexUAUsesCodexOriginator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	// 复合 UA（前缀不是 codex_cli_rs），历史实现会误判为非 Codex 并走 opencode。
	c.Request.Header.Set("User-Agent", "Mozilla/5.0 codex_cli_rs/0.1.0")

	inputBody := []byte(`{"model":"gpt-5.2","stream":true,"store":false,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": falseREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, inputBody)
REDACTED
	require.NotNil(t, upstream.lastReq)
	// 浏览器型复合 UA 被替换为默认 Codex UA（codex-tui 形态），originator 随最终 UA 配套（issue #3901）。
	require.Equal(t, DefaultOpenAICodexUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "codex-tui", upstream.lastReq.Header.Get("originator"))
	require.NotEqual(t, "opencode", upstream.lastReq.Header.Get("originator"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_ResponseHeadersAllowXCodex(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("x-request-id", "rid")
	headers.Set("x-codex-primary-used-percent", "12")
	headers.Set("x-codex-secondary-used-percent", "34")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-window-minutes", "10080")
	headers.Set("x-codex-primary-reset-after-seconds", "1")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"h"REDACTED`,
			"",
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n"))),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED

	require.Equal(t, "12", rec.Header().Get("x-codex-primary-used-percent"))
	require.Equal(t, "34", rec.Header().Get("x-codex-secondary-used-percent"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_UpstreamErrorIncludesPassthroughFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

	originalBody := []byte(`{"model":"gpt-5.2","stream":false,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"bad"REDACTEDREDACTED`)),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.True(t, c.Writer.Written(), "非 429/529 的 passthrough 错误应直接写回客户端")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// should append an upstream error event with passthrough=true
	v, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	arr, ok := v.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, arr)
	require.True(t, arr[len(arr)-1].Passthrough)
	require.Equal(t, "http_error", arr[len(arr)-1].Kind)
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_RebuildsUpstreamErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		statusCode     int
		contentType    string
		responseBody   string
		retryAfter     string
		wantStatus     int
		wantMessage    string
		wantRetryAfter string
REDACTED{
		{
			name:           "upstream forbidden is reported as gateway failure",
			statusCode:     http.StatusForbidden,
			contentType:    "text/html; charset=UTF-8",
			responseBody:   `<!DOCTYPE html><title>secret-upstream.example denied the request</title>`,
			retryAfter:     "17",
			wantStatus:     http.StatusBadGateway,
			wantMessage:    "Upstream access denied",
			wantRetryAfter: "17",
	REDACTED,
		{
			name:         "upstream unauthorized is reported as gateway failure",
			statusCode:   http.StatusUnauthorized,
			contentType:  "application/json",
			responseBody: `{"error":{"message":"invalid secret-upstream.example token","type":"authentication_error","code":"invalid_api_key","param":"api_key"REDACTED,"rate_limit":{"remaining":0REDACTEDREDACTED`,
			wantStatus:   http.StatusBadGateway,
			wantMessage:  "Upstream authentication failed",
	REDACTED,
		{
			name:         "html 5xx",
			statusCode:   http.StatusBadGateway,
			contentType:  "text/html; charset=UTF-8",
			responseBody: `<!DOCTYPE html><title>secret-upstream.example | 502: Bad gateway</title>`,
			wantStatus:   http.StatusBadGateway,
			wantMessage:  "Upstream service temporarily unavailable",
	REDACTED,
		{
			name:         "structured 5xx",
			statusCode:   http.StatusInternalServerError,
			contentType:  "application/json",
			responseBody: `{"error":{"message":"secret-upstream.example internal failure"REDACTEDREDACTED`,
			wantStatus:   http.StatusInternalServerError,
			wantMessage:  "Upstream service temporarily unavailable",
	REDACTED,
		{
			name:         "unstructured 4xx",
			statusCode:   http.StatusBadRequest,
			contentType:  "text/plain",
			responseBody: `proxy secret-upstream.example rejected the request`,
			wantStatus:   http.StatusBadRequest,
			wantMessage:  "Upstream request failed",
	REDACTED,
		{
			name:         "malicious valid json 4xx",
			statusCode:   http.StatusBadRequest,
			contentType:  "application/json",
			responseBody: `{"error":{"message":"secret-upstream.example invalid parameter","type":"invalid_request_error","code":"upstream_secret_code","param":"private_field","internal_token":"sk-upstream-secret"REDACTED,"rate_limit":{"remaining":0,"reset":"internal-window"REDACTED,"debug":{"admin":"root"REDACTED,"redirect":"https://secret-upstream.example/admin"REDACTED`,
			retryAfter:   "not-a-valid-delay",
			wantStatus:   http.StatusBadRequest,
			wantMessage:  "Upstream request failed",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: tt.statusCode,
				Header: http.Header{
					"Content-Type":                 []string{tt.contentTypeREDACTED,
					"Location":                     []string{"https://secret-upstream.example/admin"REDACTED,
					"Retry-After":                  []string{tt.retryAfterREDACTED,
					"Server":                       []string{"secret-upstream-proxy"REDACTED,
					"Set-Cookie":                   []string{"admin_token=secret"REDACTED,
					"WWW-Authenticate":             []string{`Bearer realm="secret-upstream.example"`REDACTED,
					"X-Admin-Debug":                []string{"internal-route=secret-upstream.example"REDACTED,
					"X-Codex-Primary-Used-Percent": []string{"99"REDACTED,
					"x-request-id":                 []string{"rid-sensitive-upstream"REDACTED,
			REDACTED,
				Body: io.NopCloser(strings.NewReader(tt.responseBody)),
		REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
				httpUpstream: upstream,
		REDACTED
			account := &Account{
				ID:          124,
				Name:        "sensitive-upstream",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
		REDACTED
					"api_key":  "sk-test",
					"base_url": "https://secret-upstream.example",
			REDACTED,
				Extra:       map[string]any{"openai_passthrough": trueREDACTED,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED
			requestBody := []byte(`{"model":"gpt-5.2","stream":false,"input":"hello"REDACTED`)

			_, err := svc.Forward(context.Background(), c, account, requestBody)

		REDACTED
			require.Equal(t, tt.wantStatus, rec.Code)
			opsValue, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			opsEvents, ok := opsValue.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.NotEmpty(t, opsEvents)
			require.Equal(t, tt.statusCode, opsEvents[len(opsEvents)-1].UpstreamStatusCode)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			require.Equal(t, tt.wantRetryAfter, rec.Header().Get("Retry-After"))
			for _, key := range []string{
				"Location",
				"Server",
				"Set-Cookie",
				"WWW-Authenticate",
				"X-Admin-Debug",
				"X-Codex-Primary-Used-Percent",
				"X-Request-Id",
		REDACTED {
				require.Empty(t, rec.Header().Values(key), "sensitive upstream header %s must be dropped", key)
		REDACTED
			require.Equal(t, "upstream_error", gjson.Get(rec.Body.String(), "error.type").String())
			require.Equal(t, tt.wantMessage, gjson.Get(rec.Body.String(), "error.message").String())
			require.False(t, gjson.Get(rec.Body.String(), "error.code").Exists())
			require.False(t, gjson.Get(rec.Body.String(), "error.param").Exists())
			require.False(t, gjson.Get(rec.Body.String(), "rate_limit").Exists())
			require.NotContains(t, rec.Body.String(), "secret-upstream.example")
			require.NotContains(t, rec.Body.String(), "sk-upstream-secret")
			require.NotContains(t, err.Error(), "secret-upstream.example")
	REDACTED)
REDACTED
REDACTED

func TestWriteOpenAIPassthroughErrorHeaders_StrictRetryAfter(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name string
		raw  string
		want bool
REDACTED{
		{name: "positive delay seconds", raw: "17", want: trueREDACTED,
		{name: "fractional delay", raw: "1.5"REDACTED,
		{name: "scientific notation", raw: "1e3"REDACTED,
		{name: "explicit plus sign", raw: "+17"REDACTED,
		{name: "zero", raw: "0"REDACTED,
		{name: "negative delay", raw: "-1"REDACTED,
		{name: "uint64 overflow", raw: "18446744073709551616"REDACTED,
		{name: "future http date", raw: now.Add(time.Hour).Format(http.TimeFormat), want: trueREDACTED,
		{name: "past http date", raw: now.Add(-time.Hour).Format(http.TimeFormat)REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := http.Header{"Retry-After": []string{"stale"REDACTEDREDACTED
			writeOpenAIPassthroughErrorHeaders(dst, http.Header{"Retry-After": []string{tt.rawREDACTEDREDACTED)
			if tt.want {
				require.Equal(t, tt.raw, dst.Get("Retry-After"))
		REDACTED else {
				require.Empty(t, dst.Get("Retry-After"))
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_CompactErrorBeforeKeepaliveIsSingleJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
	MarkOpenAICompactClientStream(c)
	stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
	defer stop()

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"secret-upstream.example invalid request"REDACTEDREDACTED`)),
REDACTED
REDACTED
	account := &Account{
		ID: 125, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://secret-upstream.example"REDACTED,
		Extra:       map[string]any{"openai_passthrough": trueREDACTED, Status: StatusActive, Schedulable: true,
REDACTED

	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.2","input":"hello"REDACTED`))

REDACTED
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, gjson.Valid(rec.Body.String()))
	require.Equal(t, "upstream_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.NotContains(t, rec.Body.String(), "event:")
	require.NotContains(t, rec.Body.String(), ": keepalive")
	require.NotContains(t, rec.Body.String(), "secret-upstream.example")
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_CompactErrorAfterKeepaliveIsFailedSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
	MarkOpenAICompactClientStream(c)
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"secret-upstream.example invalid request"REDACTEDREDACTED`)),
REDACTED
REDACTED
	account := &Account{
		ID: 126, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://secret-upstream.example"REDACTED,
		Extra:       map[string]any{"openai_passthrough": trueREDACTED, Status: StatusActive, Schedulable: true,
REDACTED

	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.2","input":"hello"REDACTED`))

REDACTED
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Result().Header.Get("Content-Type"), "text/event-stream")
	events := parseCompactBridgeSSE(t, stripKeepaliveComments(rec.Body.String()))
	require.Len(t, events, 1)
	require.Equal(t, "response.failed", events[0][0])
	require.Equal(t, "failed", gjson.Get(events[0][1], "response.status").String())
	require.Equal(t, "upstream_error", gjson.Get(events[0][1], "response.error.code").String())
	require.Equal(t, "Upstream request failed", gjson.Get(events[0][1], "response.error.message").String())
	require.NotContains(t, rec.Body.String(), "secret-upstream.example")
REDACTED

func TestOpenAIGatewayService_OpenAIPassthrough_RetryableStatusesTriggerFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalBody := []byte(`{"model":"gpt-5.2","stream":false,"instructions":"local-test-instructions","input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	newAccount := func(accountType string) *Account {
		account := &Account{
			ID:             123,
			Name:           "acc",
			Platform:       PlatformOpenAI,
			Type:           accountType,
			Concurrency:    1,
			Extra:          map[string]any{"openai_passthrough": trueREDACTED,
			Status:         StatusActive,
			Schedulable:    true,
			RateMultiplier: f64p(1),
	REDACTED
		switch accountType {
		case AccountTypeOAuth:
			account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED
		case AccountTypeAPIKey:
			account.Credentials = map[string]any{"api_key": "sk-test"REDACTED
	REDACTED
		return account
REDACTED

	testCases := []struct {
		name           string
		accountType    string
		statusCode     int
		body           string
		expectFailover bool
		assertRepo     func(t *testing.T, repo *openAIPassthroughFailoverRepo, start time.Time)
REDACTED{
		{
			name:        "oauth_429_rate_limit",
			accountType: AccountTypeOAuth,
			statusCode:  http.StatusTooManyRequests,
			body: func() string {
				resetAt := time.Now().Add(7 * 24 * time.Hour).Unix()
				return fmt.Sprintf(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","resets_at":%dREDACTEDREDACTED`, resetAt)
		REDACTED(),
			expectFailover: true,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, _ time.Time) {
				require.Len(t, repo.rateLimitCalls, 1)
				require.Empty(t, repo.overloadCalls)
				require.True(t, time.Until(repo.rateLimitCalls[0]) > 24*time.Hour)
		REDACTED,
	REDACTED,
		{
			name:           "oauth_529_overload",
			accountType:    AccountTypeOAuth,
			statusCode:     529,
			body:           `{"error":{"message":"server overloaded","type":"server_error"REDACTEDREDACTED`,
			expectFailover: true,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, start time.Time) {
				require.Empty(t, repo.rateLimitCalls)
				require.Len(t, repo.overloadCalls, 1)
				require.WithinDuration(t, start.Add(10*time.Minute), repo.overloadCalls[0], 5*time.Second)
		REDACTED,
	REDACTED,
		{
			name:           "oauth_502_bad_gateway",
			accountType:    AccountTypeOAuth,
			statusCode:     http.StatusBadGateway,
			body:           `{"error":{"message":"bad gateway","type":"server_error"REDACTEDREDACTED`,
			expectFailover: false,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, _ time.Time) {
				require.Empty(t, repo.rateLimitCalls)
				require.Empty(t, repo.overloadCalls)
		REDACTED,
	REDACTED,
		{
			name:           "oauth_503_unavailable",
			accountType:    AccountTypeOAuth,
			statusCode:     http.StatusServiceUnavailable,
			body:           `{"error":{"message":"service unavailable","type":"server_error"REDACTEDREDACTED`,
			expectFailover: false,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, _ time.Time) {
				require.Empty(t, repo.rateLimitCalls)
				require.Empty(t, repo.overloadCalls)
		REDACTED,
	REDACTED,
		{
			name:           "oauth_504_gateway_timeout",
			accountType:    AccountTypeOAuth,
			statusCode:     http.StatusGatewayTimeout,
			body:           `{"error":{"message":"gateway timeout","type":"server_error"REDACTEDREDACTED`,
			expectFailover: false,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, _ time.Time) {
				require.Empty(t, repo.rateLimitCalls)
				require.Empty(t, repo.overloadCalls)
		REDACTED,
	REDACTED,
		{
			name:        "apikey_429_rate_limit",
			accountType: AccountTypeAPIKey,
			statusCode:  http.StatusTooManyRequests,
			body: func() string {
				resetAt := time.Now().Add(7 * 24 * time.Hour).Unix()
				return fmt.Sprintf(`{"error":{"message":"The usage limit has been reached","type":"usage_limit_reached","resets_at":%dREDACTEDREDACTED`, resetAt)
		REDACTED(),
			expectFailover: true,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, _ time.Time) {
				require.Len(t, repo.rateLimitCalls, 1)
				require.Empty(t, repo.overloadCalls)
				require.True(t, time.Until(repo.rateLimitCalls[0]) > 24*time.Hour)
		REDACTED,
	REDACTED,
		{
			name:           "apikey_529_overload",
			accountType:    AccountTypeAPIKey,
			statusCode:     529,
			body:           `{"error":{"message":"server overloaded","type":"server_error"REDACTEDREDACTED`,
			expectFailover: true,
			assertRepo: func(t *testing.T, repo *openAIPassthroughFailoverRepo, start time.Time) {
				require.Empty(t, repo.rateLimitCalls)
				require.Len(t, repo.overloadCalls, 1)
				require.WithinDuration(t, start.Add(10*time.Minute), repo.overloadCalls[0], 5*time.Second)
		REDACTED,
	REDACTED,
REDACTED

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

			resp := &http.Response{
				StatusCode: tc.statusCode,
				Header: http.Header{
					"Content-Type": []string{"application/json"REDACTED,
					"x-request-id": []string{"rid-failover"REDACTED,
			REDACTED,
				Body: io.NopCloser(strings.NewReader(tc.body)),
		REDACTED
			upstream := &httpUpstreamRecorder{resp: respREDACTED
			repo := &openAIPassthroughFailoverRepo{REDACTED
			rateSvc := &RateLimitService{
				accountRepo: repo,
				cfg: &config.Config{
					RateLimit: config.RateLimitConfig{OverloadCooldownMinutes: 10REDACTED,
			REDACTED,
		REDACTED

			svc := &OpenAIGatewayService{
				cfg:              &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
				httpUpstream:     upstream,
				rateLimitService: rateSvc,
		REDACTED

			account := newAccount(tc.accountType)
			start := time.Now()
			_, err := svc.Forward(context.Background(), c, account, originalBody)
		REDACTED

			var failoverErr *UpstreamFailoverError
			if tc.expectFailover {
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, tc.statusCode, failoverErr.StatusCode)
				require.False(t, c.Writer.Written(), "retryable passthrough 错误应返回 failover 错误给上层换号，而不是直接向客户端写响应")
		REDACTED else {
				require.False(t, errors.As(err, &failoverErr))
				require.True(t, c.Writer.Written(), "非 failover 的 passthrough http 错误应直接写回客户端")
				require.Equal(t, tc.statusCode, rec.Code)
		REDACTED

			v, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			arr, ok := v.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.NotEmpty(t, arr)
			require.True(t, arr[len(arr)-1].Passthrough)
			if tc.expectFailover {
				require.Equal(t, "failover", arr[len(arr)-1].Kind)
		REDACTED else {
				require.Equal(t, "http_error", arr[len(arr)-1].Kind)
		REDACTED
			require.Equal(t, tc.statusCode, arr[len(arr)-1].UpstreamStatusCode)

			tc.assertRepo(t, repo, start)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_Transient5xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.2","stream":false,"input":"hello"REDACTED`)

	for _, statusCode := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		520, 521, 522, 523, 524,
REDACTED {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

			upstreamBody := fmt.Sprintf(`{"error":{"message":"temporary upstream failure","status":%dREDACTEDREDACTED`, statusCode)
			body := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(upstreamBody)REDACTED
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: statusCode,
				Header: http.Header{
					"Content-Type": []string{"application/json"REDACTED,
					"X-Request-Id": []string{"rid-api-key-5xx"REDACTED,
			REDACTED,
				Body: body,
		REDACTEDREDACTED
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
				httpUpstream: upstream,
		REDACTED
			account := &Account{
				ID:          124,
				Name:        "api-key-transient-5xx",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
		REDACTED
					"api_key":  "sk-test",
					"base_url": "https://api.example.test",
			REDACTED,
				Extra:       map[string]any{"openai_passthrough": trueREDACTED,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED

			result, err := svc.Forward(context.Background(), c, account, requestBody)

			require.Nil(t, result, "failed attempts must not report usage or success metadata")
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, statusCode, failoverErr.StatusCode)
			require.JSONEq(t, upstreamBody, string(failoverErr.ResponseBody))
			require.Equal(t, "rid-api-key-5xx", failoverErr.ResponseHeaders.Get("x-request-id"))
			require.False(t, c.Writer.Written(), "failover must happen before downstream output is committed")
			require.True(t, body.closed, "the failed upstream response body must be closed")
			require.Equal(t, requestBody, upstream.lastBody, "the request body remains available for the outer account retry")

			value, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events, ok := value.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.NotEmpty(t, events)
			require.Equal(t, "failover", events[len(events)-1].Kind)
			require.Equal(t, account.ID, events[len(events)-1].AccountID)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_ContextWindow502DoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	const upstreamBody = `{"error":{"message":"Your input exceeds the context window of this model. Please adjust your input and try again.","type":"upstream_error"REDACTEDREDACTED`
	body := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(upstreamBody)REDACTED
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       body,
REDACTED
REDACTED
	account := &Account{
		ID: 127, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://api.example.test"REDACTED,
		Extra:       map[string]any{"openai_passthrough": trueREDACTED, Status: StatusActive, Schedulable: true,
REDACTED

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.2","input":"hello"REDACTED`))

	require.Nil(t, result)
REDACTED
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "context-window errors are deterministic request failures")
	require.True(t, c.Writer.Written())
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "exceeds the context window")
	require.True(t, body.closed)
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_PoolModeConfigured5xxRetriesSameAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"temporary upstream failure"REDACTEDREDACTED`)),
REDACTED
REDACTED
	account := &Account{
		ID: 128, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
REDACTED
			"api_key":                      "sk-test",
			"base_url":                     "https://api.example.test",
			"pool_mode":                    true,
			"pool_mode_retry_status_codes": []any{float64(http.StatusBadGateway)REDACTED,
	REDACTED,
		Extra: map[string]any{"openai_passthrough": trueREDACTED, Status: StatusActive, Schedulable: true,
REDACTED

	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.2","input":"hello"REDACTED`))

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.False(t, c.Writer.Written())
REDACTED

func TestOpenAIGatewayService_OpenAIPassthrough_CompactNetworkErrorsTriggerFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		resp           *http.Response
		err            error
		expectFailover bool
REDACTED{
		{
			name:           "request_error",
			err:            errors.New("stream disconnected before completion"),
			expectFailover: true,
	REDACTED,
		{
			name: "read_error",
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid-compact"REDACTEDREDACTED,
				Body:       passthroughErrReadCloser{err: io.ErrUnexpectedEOFREDACTED,
		REDACTED,
			expectFailover: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

			upstream := &httpUpstreamRecorder{resp: tt.resp, err: tt.errREDACTED
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
				httpUpstream: upstream,
		REDACTED
			account := &Account{
				ID:             123,
				Name:           "acc",
				Platform:       PlatformOpenAI,
				Type:           AccountTypeOAuth,
				Concurrency:    1,
				Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
				Extra:          map[string]any{"openai_passthrough": trueREDACTED,
				Status:         StatusActive,
				Schedulable:    true,
				RateMultiplier: f64p(1),
		REDACTED
			body := []byte(`{"model":"gpt-5.5","instructions":"local-test-instructions","input":[{"type":"text","text":"compact me"REDACTED]REDACTED`)

			_, err := svc.Forward(context.Background(), c, account, body)
		REDACTED
			var failoverErr *UpstreamFailoverError
			if tt.expectFailover {
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
				require.False(t, c.Writer.Written(), "compact 网络错误应交给外层 failover，而不是直接写回客户端")
		REDACTED else {
				require.False(t, errors.As(err, &failoverErr))
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
				require.False(t, c.Writer.Written())
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_NonCodexUAFallbackToCodexUA(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	// Non-Codex UA
	c.Request.Header.Set("User-Agent", "curl/8.0")

	inputBody := []byte(`{"model":"gpt-5.2","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, inputBody)
REDACTED
	require.Equal(t, false, gjson.GetBytes(upstream.lastBody, "store").Bool())
	require.Equal(t, true, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, codexCLIUserAgent, upstream.lastReq.Header.Get("User-Agent"))
REDACTED

// 回归（issue #3901）：codex-tui 等官方 UA 在透传模式下必须逐字保留，且 originator
// 由最终 UA 推导配套——历史实现会把 codex-tui UA 强改为 codex_cli_rs，而 originator
// 保留客户端原值，造成 originator/UA 首段错配被上游 404。
func TestOpenAIGatewayService_OAuthPassthrough_CodexTuiIdentityPreservedAndPaired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const tuiUA = "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)"

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", tuiUA)
	// 客户端携带错配的 originator，也必须按最终 UA 重配。
	c.Request.Header.Set("originator", "codex_cli_rs")

	inputBody := []byte(`{"model":"gpt-5.2","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, inputBody)
REDACTED
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, tuiUA, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "codex-tui", upstream.lastReq.Header.Get("originator"))
REDACTED

func TestOpenAIGatewayService_CodexCLIOnly_RejectsNonCodexClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "curl/8.0")

	inputBody := []byte(`{"model":"gpt-5.2","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": true, "codex_cli_only": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, inputBody)
REDACTED
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "Codex official clients")
REDACTED

func TestOpenAIGatewayService_CodexCLIOnly_AllowOfficialClientFamilies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		ua         string
		originator string
REDACTED{
		{name: "codex_cli_rs", ua: "codex_cli_rs/0.99.0", originator: ""REDACTED,
		{name: "codex_vscode", ua: "codex_vscode/1.0.0", originator: ""REDACTED,
		{name: "codex_app", ua: "codex_app/2.1.0", originator: ""REDACTED,
		// req②：codex_cli_only 下 UA 须能解析出引擎版本；originator 命中路径用可解析的非官方前缀 UA。
		{name: "originator_codex_chatgpt_desktop", ua: "myterm/0.141.0", originator: "codex_chatgpt_desktop"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", tt.ua)
			if tt.originator != "" {
				c.Request.Header.Set("originator", tt.originator)
		REDACTED
			// 引擎指纹头：真实官方客户端必带。本测试用 nil settingService 构造 gateway，
			// detectCodexClientRestriction 会兜底默认种子指纹信号（只勾 x-codex-），与生产默认策略一致，
			// 故官方家族也须携带 x-codex-* 才能过门（对齐 TestDetect_EngineFingerprintSignals）。
			c.Request.Header.Set("x-codex-window-id", "1")

			inputBody := []byte(`{"model":"gpt-5.2","stream":false,"store":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
		REDACTED
			upstream := &httpUpstreamRecorder{resp: respREDACTED

			svc := &OpenAIGatewayService{
				cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
				httpUpstream: upstream,
		REDACTED

			account := &Account{
				ID:             123,
				Name:           "acc",
				Platform:       PlatformOpenAI,
				Type:           AccountTypeOAuth,
				Concurrency:    1,
				Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
				Extra:          map[string]any{"openai_passthrough": true, "codex_cli_only": trueREDACTED,
				Status:         StatusActive,
				Schedulable:    true,
				RateMultiplier: f64p(1),
		REDACTED

			_, err := svc.Forward(context.Background(), c, account, inputBody)
		REDACTED
			require.NotNil(t, upstream.lastReq)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_StreamingSetsFirstTokenMs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"service_tier":"fast","input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	upstreamSSE := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"h"REDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	start := time.Now()
	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	// sanity: duration after start
	require.GreaterOrEqual(t, time.Since(start), time.Duration(0))
	require.NotNil(t, result.FirstTokenMs)
	require.GreaterOrEqual(t, *result.FirstTokenMs, 0)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "priority", *result.ServiceTier)
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_StreamClientDisconnectStillCollectsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	// 首次写入成功，后续写入失败，模拟客户端中途断开。
	c.Writer = &failingGinWriter{ResponseWriter: c.Writer, failAfter: 1REDACTED

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)

	upstreamSSE := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"h"REDACTED`,
		"",
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":11,"output_tokens":7,"input_tokens_details":{"cached_tokens":3REDACTEDREDACTEDREDACTEDREDACTED`,
		"",
		"data: [DONE]",
		"",
REDACTED, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
REDACTED

func TestOpenAIGatewayService_APIKeyPassthrough_PreservesBodyAndUsesResponsesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "curl/8.0")
	c.Request.Header.Set("X-Test", "keep")
	c.Request.Header.Set("x-codex-beta-features", "remote_compaction_v2")

	originalBody := []byte(`{"model":"gpt-5.2","stream":false,"service_tier":"flex","max_output_tokens":128,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTED, "x-request-id": []string{"rid"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"output":[],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTED`)),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED

	account := &Account{
		ID:             456,
		Name:           "apikey-acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		Concurrency:    1,
		Credentials:    map[string]any{"api_key": "sk-api-key", "base_url": "https://api.openai.com"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	result, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, result)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "flex", *result.ServiceTier)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, originalBody, upstream.lastBody)
	require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-api-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "curl/8.0", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "remote_compaction_v2", upstream.lastReq.Header.Get("x-codex-beta-features"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Test"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_WarnOnTimeoutHeadersForStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("x-stainless-timeout", "10000")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid-timeout"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:             321,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.True(t, logSink.ContainsMessage("检测到超时相关请求头，将按配置过滤以降低断流风险"))
	require.True(t, logSink.ContainsFieldValue("timeout_headers", "x-stainless-timeout=10000"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_InfoWhenStreamEndsWithoutDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, restore := captureStructuredLog(t)
	defer restore()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	// 注意：刻意不发送 [DONE]，模拟上游中途断流。
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid-truncate"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"h\"REDACTED\n\n")),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:             654,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, originalBody)
	require.EqualError(t, err, "stream usage incomplete: missing terminal event")
	require.True(t, logSink.ContainsMessage("上游流在未收到 [DONE] 时结束，疑似断流"))
	require.True(t, logSink.ContainsMessageAtLevel("上游流在未收到 [DONE] 时结束，疑似断流", "info"))
	require.True(t, logSink.ContainsFieldValue("upstream_request_id", "rid-truncate"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_DefaultFiltersTimeoutHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("x-stainless-timeout", "120000")
	c.Request.Header.Set("X-Test", "keep")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid-filter-default"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n"))),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: falseREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:             111,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, upstream.lastReq)
	require.Empty(t, upstream.lastReq.Header.Get("x-stainless-timeout"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Test"))
REDACTED

func TestOpenAIGatewayService_OAuthPassthrough_AllowTimeoutHeadersWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("x-stainless-timeout", "120000")
	c.Request.Header.Set("X-Test", "keep")

	originalBody := []byte(`{"model":"gpt-5.2","stream":true,"input":[{"type":"text","text":"hi"REDACTED]REDACTED`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTED, "X-Request-Id": []string{"rid-filter-allow"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0REDACTEDREDACTEDREDACTEDREDACTED`,
			"",
			"data: [DONE]",
			"",
	REDACTED, "\n"))),
REDACTED
	upstream := &httpUpstreamRecorder{resp: respREDACTED
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			ForceCodexCLI:                        false,
			OpenAIPassthroughAllowTimeoutHeaders: true,
REDACTED
		httpUpstream: upstream,
REDACTED
	account := &Account{
		ID:             222,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"REDACTED,
		Extra:          map[string]any{"openai_passthrough": trueREDACTED,
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
REDACTED

	_, err := svc.Forward(context.Background(), c, account, originalBody)
REDACTED
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "120000", upstream.lastReq.Header.Get("x-stainless-timeout"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Test"))
REDACTED
